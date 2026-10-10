package glossary_test

import (
	"testing"

	"github.com/SantaChains/LspProxy/internal/glossary"
)

// ─────────────────────────────────────────────
// Terms 合并查询测试（供 markdown 术语保护层使用）
//
// 注意：glossary.New 会把 builtin 内嵌词汇本增量合并进目录，
// 因此不能对映射的总条数做精确断言，只能用 builtin 不存在的
// 独特 key 验证合并与优先级行为。
// ─────────────────────────────────────────────

func TestGlossary_Terms_MergeAndPriority(t *testing.T) {
	dir := t.TempDir()

	writeToml(t, dir, "_global.toml", `[terms]
"zz global term" = "全局译文"
"zz shared term" = "全局版"
`)
	writeToml(t, dir, "rust-analyzer.toml", `[terms]
"zz shared term" = "专属版"
"zz lsp term"    = "专属译文"
`)

	g := glossary.New(dir, []string{"rust-analyzer"}, newLogger())
	defer g.Close()

	terms := g.Terms("rust-analyzer")

	// LSP 专属覆盖全局同 key 条目
	if v := terms["zz shared term"]; v != "专属版" {
		t.Errorf(`"zz shared term" 期望专属译文 "专属版"，得到 %q`, v)
	}
	// 全局条目打底
	if v := terms["zz global term"]; v != "全局译文" {
		t.Errorf(`"zz global term" 期望 "全局译文"，得到 %q`, v)
	}
	// 专属新增条目
	if v := terms["zz lsp term"]; v != "专属译文" {
		t.Errorf(`"zz lsp term" 期望 "专属译文"，得到 %q`, v)
	}
	// builtin 合并使词汇本自带术语（builtin 内容变化时不做精确条数断言）
	if len(terms) < 3 {
		t.Errorf("合并后条数不应少于用户词条数，得到 %d", len(terms))
	}
}

func TestGlossary_Terms_GlobalOnly(t *testing.T) {
	dir := t.TempDir()

	writeToml(t, dir, "_global.toml", `[terms]
"zz global term" = "全局译文"
`)

	g := glossary.New(dir, nil, newLogger())
	defer g.Close()

	// 未加载的 LSP 名称只命中全局词汇本
	terms := g.Terms("clangd")
	if v, ok := terms["zz global term"]; !ok || v != "全局译文" {
		t.Errorf("期望命中全局词条，得到 (%q, %v)", v, ok)
	}
	if _, ok := terms["zz lsp term"]; ok {
		t.Errorf("未加载的 LSP 不应命中专属词条")
	}
}

func TestGlossary_Terms_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()

	writeToml(t, dir, "_global.toml", `[terms]
"zz global term" = "全局译文"
`)

	g := glossary.New(dir, nil, newLogger())
	defer g.Close()

	terms := g.Terms("")
	terms["zz global term"] = "被篡改"

	fresh := g.Terms("")
	if fresh["zz global term"] != "全局译文" {
		t.Errorf("Terms 应返回副本，修改返回值不应影响内部状态")
	}
}
