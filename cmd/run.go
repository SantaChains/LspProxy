// Package cmd 包含 LspProxy 的所有 CLI 命令定义。
// 本文件注册根命令的运行逻辑（直接代理模式与 TUI 模式）。
package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SantaChains/LspProxy/internal/config"
	"github.com/SantaChains/LspProxy/internal/proxy"
	"github.com/SantaChains/LspProxy/internal/translate"
	"github.com/SantaChains/LspProxy/tui"
)

// exitCoder 是可以提供退出码的错误接口，由 proxy.lspExitError 实现。
type exitCoder interface {
	error
	ExitCode() int
}

// tuiFlag 若为 true，则启动 TUI 管理界面而非直接运行代理
var tuiFlag bool

func init() {
	// 将运行逻辑绑定到根命令
	rootCmd.RunE = runProxy

	// --tui：启动 TUI 管理界面
	rootCmd.Flags().BoolVar(
		&tuiFlag,
		"tui",
		false,
		"启动 TUI 管理界面（交互式配置与日志查看）",
	)

	// -e / --engine：覆盖配置文件中的翻译引擎设置
	rootCmd.Flags().StringP(
		"engine",
		"e",
		"",
		"翻译引擎（google|openai），覆盖配置文件中的设置",
	)

	// 允许在 -- 之后传入 LSP 命令参数（如 rust-analyzer、clangd 等）
	// ArbitraryArgs 使 cobra 不对位置参数数量做限制
	rootCmd.Args = cobra.ArbitraryArgs
}

// runProxy 是根命令的主执行函数。
//
// 两种运行模式：
//  1. --tui 模式：启动 Bubble Tea TUI 管理界面
//  2. 代理模式：args 中包含 LSP 命令，例如 ["rust-analyzer"] 或 ["clangd", "--background-index"]
func runProxy(cmd *cobra.Command, args []string) error {
	// ── 1. 确定配置文件路径 ─────────────────────────────────────────────
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = config.DefaultPath()
	}

	// ── 2. 加载配置 ──────────────────────────────────────────────────────
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// ── 3. 命令行标志覆盖配置文件 ────────────────────────────────────────
	if engineFlag, _ := cmd.Flags().GetString("engine"); engineFlag != "" {
		cfg.Translate.Engine = engineFlag
	}

	// ── 4. TUI 模式 ───────────────────────────────────────────────────────
	if tuiFlag {
		return tui.Run(cfg, cfgPath)
	}

	// ── 5. 代理模式：校验参数 ─────────────────────────────────────────────
	if len(args) == 0 {
		return fmt.Errorf(
			"代理模式需要指定 LSP 命令，例如：\n  LspProxy -- rust-analyzer\n  LspProxy -- clangd --background-index\n\n" +
				"若要启动管理界面，请使用 --tui 标志",
		)
	}

	lspCommand := args[0]
	lspArgs := args[1:]

	// 词汇本文件名使用规范化名称：小写、去目录与扩展名。
	// 编辑器扩展的 server.path 可能传绝对路径（如 D:\bin\rust-analyzer.exe）。
	lspName := strings.ToLower(strings.TrimSuffix(filepath.Base(lspCommand), filepath.Ext(lspCommand)))

	// ── 5.5 探测直通：--version/--help 不走代理生命周期 ──────────────────
	// 编辑器以 "<server> --version" 探测可用性时，stdin 管道保持打开并等待
	// 进程退出；代理模式会在子进程退出后继续等待客户端 stdin 关闭而挂起，
	// 导致探测永不返回、服务器永不启动。此处直接透传真实 LSP。
	// version/help 为子命令形式（如 gopls 预检执行 "gopls version -json"）。
	// 小写 -v 有意不在列：部分 LSP（如 pylsp -v 冗长日志）将其作为会话参数，
	// 须走代理模式保留翻译；无扩展以 -v 探测（真实 rust-analyzer -v 会启动服务器）。
	if len(lspArgs) > 0 {
		switch lspArgs[0] {
		case "--version", "-V", "-version", "--help", "-h", "version", "help":
			if err := proxy.RunPassthrough(lspCommand, lspArgs); err != nil {
				// 透传真实 LSP 的退出码，探测失败时编辑器据此判定二进制不可用
				var ec exitCoder
				if errors.As(err, &ec) {
					os.Exit(ec.ExitCode())
				}
				return err
			}
			return nil
		}
	}

	// ── 6. 初始化文件日志（严禁写入 stdout，否则污染 LSP 协议）────────────
	logger, logFile, err := buildFileLogger(cfg)
	if err != nil {
		// 日志初始化失败时降级为 stderr（不影响 LSP 协议）
		fmt.Fprintf(os.Stderr, "警告：初始化日志文件失败，降级到 stderr：%v\n", err)
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: parseLogLevel(cfg.Log.Level),
		}))
	} else {
		defer logFile.Close()
	}

	// ── 7. 创建翻译引擎 ───────────────────────────────────────────────────
	engine, gl, err := translate.New(cfg, lspName, logger)
	if err != nil {
		return fmt.Errorf("初始化翻译引擎失败: %w", err)
	}

	// 若引擎实现了 io.Closer（如 DictEngine），在退出时关闭以确保资源释放和数据持久化
	if closer, ok := engine.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	logger.Info("翻译引擎已初始化",
		slog.String("engine", engine.Name()),
		slog.String("targetLang", cfg.Proxy.TargetLang),
		slog.String("lspName", lspName),
	)

	// ── 8. 创建代理实例 ───────────────────────────────────────────────────
	// termFn 每次翻译时即时取词，词汇本热重载后无需任何失效处理
	termFn := func() map[string]string { return gl.Terms(lspName) }
	p := proxy.New(cfg, engine, logger, termFn)

	// ── 9. 监听系统信号，实现优雅退出 ────────────────────────────────────
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── 10. 运行代理（阻塞直到 LSP 进程退出或收到终止信号）───────────────
	if err := p.Run(ctx, lspCommand, lspArgs); err != nil {
		// LSP 子进程异常退出：以相同退出码退出，让编辑器进程管理器感知崩溃
		var ec exitCoder
		if errors.As(err, &ec) {
			os.Exit(ec.ExitCode())
		}
		return err
	}
	return nil
}

// buildFileLogger 根据配置创建写入文件的 slog.Logger。
// 返回 logger 实例、打开的文件句柄（调用方需负责 Close）以及错误信息。
//
// 日志文件路径取自 cfg.Log.File，若目录不存在会自动创建。
func buildFileLogger(cfg *config.Config) (*slog.Logger, *os.File, error) {
	logPath := cfg.Log.File

	// 确保日志目录存在
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("创建日志目录失败 [%s]: %w", logDir, err)
	}

	// 以追加方式打开日志文件（不存在时自动创建）
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("打开日志文件失败 [%s]: %w", logPath, err)
	}

	level := parseLogLevel(cfg.Log.Level)

	// 使用 JSON 格式便于后续解析，写入文件
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug, // 仅在 debug 级别时附加源码位置
	})

	return slog.New(handler), f, nil
}

// parseLogLevel 将配置字符串转换为 slog.Level。
// 支持 "debug"、"info"（默认）、"warn"、"error"，不区分大小写。
func parseLogLevel(level string) slog.Level {
	level = strings.ToLower(level)
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		// 未知值或空字符串均默认为 info
		return slog.LevelInfo
	}
}
