// 词汇本管理子命令：导出为 Markdown wiki。
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/SantaChains/LspProxy/internal/config"
	"github.com/SantaChains/LspProxy/internal/glossary"
	"github.com/spf13/cobra"
)

// glossaryCmd 词汇本管理子命令组
var glossaryCmd = &cobra.Command{
	Use:   "glossary",
	Short: "管理术语词汇本",
	Long: `查看与导出词汇本目录（默认 ~/.local/share/lsp-proxy/glossary/）下的术语词汇本。
编辑词条请使用 TUI（LspProxy --tui）或直接修改 TOML 文件，支持热重载。`,
}

var glossaryExportCmd = &cobra.Command{
	Use:   "export",
	Short: "将词汇本导出为 Markdown wiki",
	Long: `把所有词汇本 TOML 合成一份 Markdown wiki：目录 + 分节表格，可直接放入
GitHub wiki 或 Obsidian。默认输出到标准输出，--out 写入文件。`,
	RunE: runGlossaryExport,
}

func init() {
	rootCmd.AddCommand(glossaryCmd)
	glossaryCmd.AddCommand(glossaryExportCmd)

	glossaryExportCmd.Flags().String("out", "", "输出文件路径（默认输出到标准输出）")
}

func runGlossaryExport(cmd *cobra.Command, args []string) error {
	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	dir := cfg.Proxy.GlossaryDir
	if dir == "" {
		dir = config.DefaultGlossaryDir()
	}

	// 列出全部 TOML 文件，_global 置顶、其余按文件名排序
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("读取词汇本目录失败 [%s]: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".toml") {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "_global.toml") != (names[j] == "_global.toml") {
			return names[i] == "_global.toml"
		}
		return names[i] < names[j]
	})

	type section struct {
		title  string
		anchor string
		terms  []glossary.TermEntry
	}
	var sections []section
	total := 0
	for _, name := range names {
		terms, err := glossary.LoadTermsFromFile(filepath.Join(dir, name))
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告：跳过无法解析的词汇本 %s：%v\n", name, err)
			continue
		}
		if len(terms) == 0 {
			continue
		}
		title := strings.TrimSuffix(name, ".toml")
		if title == "_global" {
			title = "全局词汇本"
		}
		sections = append(sections, section{title: title, terms: terms})
		total += len(terms)
	}

	var sb strings.Builder
	sb.WriteString("# LspProxy 术语词汇本\n\n")
	fmt.Fprintf(&sb, "共 %d 个词汇本、%d 条术语，导出于 %s。\n", len(sections), total, time.Now().Format("2006-01-02 15:04"))

	// 目录
	if len(sections) > 0 {
		sb.WriteString("\n## 目录\n\n")
		for _, s := range sections {
			fmt.Fprintf(&sb, "- [%s](#%s)（%d 条）\n", s.title, anchorSlug(s.title), len(s.terms))
		}
	}

	// 分节表格
	for _, s := range sections {
		fmt.Fprintf(&sb, "\n## %s\n\n", s.title)
		sb.WriteString("| 原文 | 译文 |\n|------|------|\n")
		for _, t := range s.terms {
			fmt.Fprintf(&sb, "| %s | %s |\n", mdCell(t.Key), mdCell(t.Value))
		}
	}

	return writeOrPrint(cmd, "glossary export", sb.String())
}

// anchorSlug 生成 GitHub 风格的标题锚点：小写、空格转连字符、去除非单词字符。
func anchorSlug(title string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', unicode.Is(unicode.Han, r):
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteByte('-')
		}
	}
	return sb.String()
}
