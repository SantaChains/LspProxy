package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SantaChains/LspProxy/internal/config"
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

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheRerunCmd)
	cacheCmd.AddCommand(cacheClearCmd)

	cacheListCmd.Flags().IntP("limit", "n", 50, "最多显示条数（0=全部）")
	cacheListCmd.Flags().Bool("json", false, "以 JSON 格式输出")
	cacheListCmd.Flags().String("filter", "", "按源文本或译文过滤（子串匹配）")

	cacheClearCmd.Flags().Int("older-days", 0, "仅清除最后更新早于 N 天的条目，0=清除全部")
	cacheClearCmd.Flags().Bool("yes", false, "跳过确认提示直接清除")
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
