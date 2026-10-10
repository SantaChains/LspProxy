package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SantaChains/LspProxy/internal/config"
	"github.com/SantaChains/LspProxy/internal/glossary"
	"github.com/SantaChains/LspProxy/internal/translate"
	"github.com/spf13/cobra"
)

// cacheCmd 缓存管理子命令组
var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "管理磁盘翻译缓存",
	Long: `管理 translate.dict_file 磁盘缓存：列出条目、删除单条触发重新翻译、或全部清除。
注意：代理运行时无法清除缓存，需先停止 LSP 进程。`,
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有缓存条目",
	RunE:  runCacheList,
}

var cacheRerunCmd = &cobra.Command{
	Use:   "rerun <index>",
	Short: "删除指定序号条目，下次遇到同一文本时自动重新翻译",
	RunE:  runCacheRerun,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "清除全部或指定天数前的缓存",
	RunE:  runCacheClear,
}

var cachePromoteCmd = &cobra.Command{
	Use:   "promote <序号>",
	Short: "将缓存条目晋升为词汇本术语，段落内子串匹配永久生效",
	Long: `把 cache list 中的一条译文固化到术语词汇本 TOML。

词汇本词条由翻译管线的术语保护层在段落内做词边界子串匹配，
不依赖整段精确命中，且支持热重载（代理运行中立即生效）。
目标词汇本用 --lsp <name>（LSP 专属）或 --global（全局）指定。

仅适合纯文本短语条目：含 $CODE_N$ 代码占位符的条目会被拒绝。`,
	RunE: runCachePromote,
}

var cacheExportCmd = &cobra.Command{
	Use:   "export",
	Short: "将缓存导出为 Markdown 文档",
	Long: `把磁盘词典的全部条目导出为 Markdown 表格（按更新时间倒序），
用于离线检索或归档。默认输出到标准输出，--out 写入文件。`,
	RunE: runCacheExport,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheRerunCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cachePromoteCmd)
	cacheCmd.AddCommand(cacheExportCmd)

	cacheListCmd.Flags().IntP("limit", "n", 50, "最多显示条数（0=全部）")
	cacheListCmd.Flags().Bool("json", false, "以 JSON 格式输出")
	cacheListCmd.Flags().String("filter", "", "按源文本或译文过滤（子串匹配）")

	cacheClearCmd.Flags().Int("older-days", 0, "仅清除最后更新早于 N 天的条目，0=清除全部")
	cacheClearCmd.Flags().Bool("yes", false, "跳过确认提示直接清除")

	cachePromoteCmd.Flags().String("lsp", "", "目标 LSP 专属词汇本名称（如 rust-analyzer）")
	cachePromoteCmd.Flags().BoolP("global", "g", false, "写入全局词汇本 _global.toml")

	cacheExportCmd.Flags().String("out", "", "输出文件路径（默认输出到标准输出）")
	cacheExportCmd.Flags().String("filter", "", "按源文本或译文过滤（子串匹配）")
	cacheExportCmd.Flags().IntP("limit", "n", 0, "最多导出条数（0=全部）")
}

func runCacheList(cmd *cobra.Command, args []string) error {
	dict, err := openDict(cmd)
	if err != nil {
		return err
	}
	defer dict.Close()

	entries := dict.List()
	if len(entries) == 0 {
		fmt.Println("缓存为空")
		return nil
	}

	limit, _ := cmd.Flags().GetInt("limit")
	useJSON, _ := cmd.Flags().GetBool("json")
	filter, _ := cmd.Flags().GetString("filter")

	if filter != "" {
		lower := strings.ToLower(filter)
		var filtered []translate.CacheEntry
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Source), lower) ||
				strings.Contains(strings.ToLower(e.Translation), lower) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if useJSON {
		return printJSON(entries, limit)
	}
	return printText(entries, limit)
}

func printJSON(entries []translate.CacheEntry, limit int) error {
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	out, _ := json.MarshalIndent(entries, "", "  ")
	fmt.Println(string(out))
	return nil
}

func printText(entries []translate.CacheEntry, limit int) error {
	shown := len(entries)
	if limit > 0 && limit < shown {
		shown = limit
	}

	for i := 0; i < shown; i++ {
		e := entries[i]
		sourceShort := truncateStr(e.Source, 70)
		transShort := truncateStr(e.Translation, 55)
		age := time.Since(e.UpdatedAt).Round(time.Second)
		fmt.Printf("[%4d] %s\n      → %s  (%s)\n", i+1, sourceShort, transShort, age)
	}
	fmt.Printf("\n共 %d 条，显示 %d 条", len(entries), shown)
	if limit > 0 && shown == limit {
		fmt.Print("（更多请用 --limit 0 或 --filter 精确查找）")
	}
	fmt.Println()
	return nil
}

func runCacheRerun(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法: LspProxy cache rerun <序号>，序号来自 list 命令的 [N] 列")
	}

	dict, err := openDict(cmd)
	if err != nil {
		return err
	}
	defer dict.Close()

	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("无效序号 %q，请用 cache list 查看序号", args[0])
	}

	entries := dict.List()
	if idx < 1 || idx > len(entries) {
		return fmt.Errorf("序号 %d 超出范围（共 %d 条）", idx, len(entries))
	}

	target := entries[idx-1]
	if dict.Delete(target.Key) {
		fmt.Printf("已删除 [%d]: %s\n", idx, truncateStr(target.Source, 80))
		fmt.Printf("      原译文: %s\n", truncateStr(target.Translation, 80))
		fmt.Println("下次遇到同一文本时自动用当前引擎重新翻译")
	} else {
		return fmt.Errorf("删除失败，条目可能已被驱逐")
	}
	return nil
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	dict, err := openDict(cmd)
	if err != nil {
		return err
	}
	defer dict.Close()

	olderDays, _ := cmd.Flags().GetInt("older-days")
	yes, _ := cmd.Flags().GetBool("yes")

	var count int
	if olderDays <= 0 {
		if !yes {
			fmt.Printf("确认清除全部 %d 条缓存？(y/N): ", dict.Len())
			var input string
			fmt.Scanln(&input)
			if !strings.EqualFold(strings.TrimSpace(input), "y") {
				fmt.Println("已取消")
				return nil
			}
		}
		count, err = dict.ClearAll()
	} else {
		count, err = dict.ClearOlderThan(olderDays)
	}
	if err != nil {
		return fmt.Errorf("清除失败: %w", err)
	}
	fmt.Printf("已清除 %d 条缓存\n", count)
	return nil
}

// openDict 加载配置并打开磁盘词典
func openDict(cmd *cobra.Command) (*translate.DiskDict, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}

	dictPath := cfg.Proxy.DictFile
	if dictPath == "" {
		dictPath = config.DefaultDictFile()
	}
	maxEntries := cfg.Proxy.DictMaxEntries
	if maxEntries <= 0 {
		maxEntries = 100000
	}

	dict, err := translate.NewDiskDict(dictPath, maxEntries)
	if err != nil {
		return nil, fmt.Errorf("打开缓存文件失败 [%s]: %w", dictPath, err)
	}
	return dict, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// runCachePromote 将缓存条目写入词汇本 TOML，实现"常用译文人工固化"闭环。
func runCachePromote(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法: LspProxy cache promote <序号>，序号来自 cache list 的 [N] 列")
	}

	lspFlag, _ := cmd.Flags().GetString("lsp")
	globalFlag, _ := cmd.Flags().GetBool("global")
	if !globalFlag && lspFlag == "" {
		return fmt.Errorf("请指定目标词汇本：--lsp <name>（LSP 专属）或 --global（全局）")
	}

	dict, err := openDict(cmd)
	if err != nil {
		return err
	}
	defer dict.Close()

	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("无效序号 %q，请用 cache list 查看序号", args[0])
	}

	entries := dict.List()
	if idx < 1 || idx > len(entries) {
		return fmt.Errorf("序号 %d 超出范围（共 %d 条）", idx, len(entries))
	}

	target := entries[idx-1]
	if strings.Contains(target.Source, "$CODE_") {
		return fmt.Errorf(
			"条目 [%d] 含代码占位符，不适合作为术语：\n  %s\n请选择纯文本短语条目",
			idx, truncateStr(target.Source, 80),
		)
	}

	// 确定目标词汇本文件
	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	glossaryDir := cfg.Proxy.GlossaryDir
	if glossaryDir == "" {
		glossaryDir = config.DefaultGlossaryDir()
	}
	var fileName string
	if globalFlag {
		fileName = "_global.toml"
	} else {
		fileName = lspFlag + ".toml"
	}
	filePath := filepath.Join(glossaryDir, fileName)

	if err := os.MkdirAll(glossaryDir, 0o755); err != nil {
		return fmt.Errorf("创建词汇本目录失败 [%s]: %w", glossaryDir, err)
	}

	// 追加或覆盖词条（保留文件注释头，SaveTerms 内部处理）
	terms, err := glossary.LoadTermsFromFile(filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err // TOML 解析失败等真错误不应静默重置文件
		}
		terms = nil // 文件不存在时从零开始
	}

	action := "新增"
	updated := false
	for i := range terms {
		if terms[i].Key == target.Source {
			terms[i].Value = target.Translation
			updated = true
			action = "覆盖"
			break
		}
	}
	if !updated {
		terms = append(terms, glossary.TermEntry{Key: target.Source, Value: target.Translation})
	}

	if err := glossary.SaveTermsToFile(filePath, terms); err != nil {
		return err
	}

	fmt.Printf("已%s %s: %q = %q\n", action, fileName, target.Source, target.Translation)
	fmt.Println("词汇本支持热重载，代理运行中立即生效")
	return nil
}

// runCacheExport 将磁盘词典导出为 Markdown 表格。
func runCacheExport(cmd *cobra.Command, args []string) error {
	dict, err := openDict(cmd)
	if err != nil {
		return err
	}
	defer dict.Close()

	entries := dict.List()

	if filter, _ := cmd.Flags().GetString("filter"); filter != "" {
		lower := strings.ToLower(filter)
		var filtered []translate.CacheEntry
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Source), lower) ||
				strings.Contains(strings.ToLower(e.Translation), lower) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	var sb strings.Builder
	sb.WriteString("# LspProxy 翻译缓存\n\n")
	fmt.Fprintf(&sb, "共 %d 条，导出于 %s。\n\n", len(entries), time.Now().Format("2006-01-02 15:04"))
	sb.WriteString("| # | 原文 | 译文 | 更新时间 |\n|---|------|------|----------|\n")
	for i, e := range entries {
		fmt.Fprintf(&sb, "| %d | %s | %s | %s |\n",
			i+1, mdCell(e.Source), mdCell(e.Translation), e.UpdatedAt.Format("2006-01-02 15:04"))
	}

	return writeOrPrint(cmd, "cache export", sb.String())
}

// mdCell 将文本转义为 Markdown 表格单元格内容（管道符与换行）。
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// writeOrPrint 将内容写入 --out 文件或输出到标准输出。
func writeOrPrint(cmd *cobra.Command, name, content string) error {
	if out, _ := cmd.Flags().GetString("out"); out != "" {
		if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
			return fmt.Errorf("%s 写入文件失败 [%s]: %w", name, out, err)
		}
		fmt.Printf("已导出 %d 字节到 %s\n", len(content), out)
		return nil
	}
	fmt.Print(content)
	return nil
}
