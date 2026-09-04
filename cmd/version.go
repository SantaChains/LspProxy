// Package cmd — version 子命令：输出构建版本信息。
package cmd

import (
	"fmt"
	"regexp"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// 版本信息（由 main 包通过 SetVersionInfo 注入）

var (
	buildVersion = "dev"
	buildCommit  = ""
	buildDate    = ""
)

// SetVersionInfo 由 main 包在启动时调用，将 ldflags 注入的版本信息传递给 cmd 包。
//
// 回退链（优先级从高到低）：
//  1. ldflags 注入值（release 构建 / make build）
//  2. debug.ReadBuildInfo() 的 vcs.revision / vcs.time（本地 go build，在 VCS 仓库内）
//  3. info.Main.Version（go install pkg@version 会填充模块版本/伪版本），
//     并从伪版本字符串中解析 commit 短哈希与构建日期
//  4. 最终回退："dev" / "unknown"
//
// 同时将 rootCmd.Version 设置为完整版本字符串，使 --version 标志生效。
func SetVersionInfo(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date

	if info, ok := debug.ReadBuildInfo(); ok {
		// ── 版本号：ldflags 为空或 "dev" 时使用模块版本 ──────────
		// go install pkg@latest 会将 info.Main.Version 设为解析后的模块版本
		// （如 v1.2.3 或伪版本 v0.0.0-20260904150232-44a41b010415）。
		// 本地 go build / go run 时该值为 "(devel)"，需忽略以保留 "dev"。
		if buildVersion == "" || buildVersion == "dev" {
			if mv := info.Main.Version; mv != "" && mv != "(devel)" {
				buildVersion = mv
			}
		}

		// ── commit / date：先取 vcs 元数据 ────────────────────────
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if buildCommit == "" && s.Value != "" {
					if len(s.Value) > 7 {
						buildCommit = s.Value[:7]
					} else {
						buildCommit = s.Value
					}
				}
			case "vcs.time":
				if buildDate == "" && len(s.Value) >= 10 {
					buildDate = s.Value[:10]
				}
			}
		}

		// ── commit / date：从模块伪版本解析（go install 的唯一来源）──
		// 伪版本格式：vX.Y.Z[-pre.N].yyyymmddhhmmss-abcdef123456
		// 末两段固定为 14 位 UTC 时间戳与 12 位 commit 短哈希。
		if buildCommit == "" || buildDate == "" {
			c, d := parsePseudoVersion(info.Main.Version)
			if buildCommit == "" && c != "" {
				buildCommit = c
			}
			if buildDate == "" && d != "" {
				buildDate = d
			}
		}
	}

	// ── 设置 rootCmd.Version，让 cobra 自动注册 --version 标志 ────
	shortCommit := buildCommit
	if shortCommit == "" {
		shortCommit = "unknown"
	}
	shortDate := buildDate
	if shortDate == "" {
		shortDate = "unknown"
	}
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, date: %s)", buildVersion, shortCommit, shortDate)
}

// pseudoVersionRE 匹配 Go 模块伪版本末尾的时间戳与 commit。
// 伪版本格式（三种）：
//   vX.0.0-yyyymmddhhmmss-abcdefabcdef
//   vX.Y.(Z+1)-0.yyyymmddhhmmss-abcdefabcdef
//   vX.Y.Z-pre.0.yyyymmddhhmmss-abcdefabcdef
// 共同特征：以 <14位时间戳>-<12位十六进制commit> 结尾。
var pseudoVersionRE = regexp.MustCompile(`(\d{14})-([0-9a-fA-F]{12})$`)

// parsePseudoVersion 从 Go 模块伪版本中解析 commit 短哈希与构建日期。
// 非伪版本（如语义化版本 v1.2.3）返回空串。
func parsePseudoVersion(v string) (commit, date string) {
	m := pseudoVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", ""
	}
	ts := m[1]
	commit = m[2]
	date = ts[:4] + "-" + ts[4:6] + "-" + ts[6:8]
	return commit, date
}

// versionCmd 输出构建版本信息。
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  "显示 LspProxy 的版本号、构建 commit 和构建日期。",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

// printVersion 格式化并输出版本信息。
func printVersion() {
	commit := buildCommit
	if commit == "" {
		commit = "unknown"
	}
	date := buildDate
	if date == "" {
		date = "unknown"
	}
	fmt.Printf("LspProxy %s\n", buildVersion)
	fmt.Printf("  commit : %s\n", commit)
	fmt.Printf("  date   : %s\n", date)
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
