package markdown

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// ProtectWithTerms：第七层词汇本术语保护
// ─────────────────────────────────────────────────────────────────────────────

// restoreTerms 是测试便捷封装：保护 → 还原，返回最终文本。
// 术语占位符的还原内容即词汇本译文，因此最终文本应包含译文。
func restoreTerms(text string, terms map[string]string) string {
	masked, codes := ProtectWithTerms(text, terms)
	return Restore(masked, codes)
}

func TestProtectWithTerms_SubstringReplace(t *testing.T) {
	terms := map[string]string{"borrow checker": "借用检查器"}
	got := restoreTerms("The borrow checker prevents data races at compile time.", terms)
	want := "The 借用检查器 prevents data races at compile time."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_CaseInsensitive(t *testing.T) {
	terms := map[string]string{"Rust": "Rust 语言"}
	got := restoreTerms("RUST is fast. Also rust and Rust.", terms)
	want := "Rust 语言 is fast. Also Rust 语言 and Rust 语言."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_WordBoundary(t *testing.T) {
	terms := map[string]string{"lifetime": "生命周期"}
	// lifetimes 不应被 lifetime 的前缀命中
	got := restoreTerms("The lifetime of a value; many lifetimes exist.", terms)
	want := "The 生命周期 of a value; many lifetimes exist."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_LongestTermWins(t *testing.T) {
	terms := map[string]string{
		"lifetime":         "生命周期",
		"lifetime elision": "生命周期省略",
	}
	got := restoreTerms("This uses lifetime elision rules.", terms)
	want := "This uses 生命周期省略 rules."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_SkipsInlineCode(t *testing.T) {
	terms := map[string]string{"lifetime": "生命周期"}
	got := restoreTerms("Pass a `lifetime` parameter here.", terms)
	// 行内代码由第一层保护，术语层不得触碰
	want := "Pass a `lifetime` parameter here."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_SkipsLinkURL(t *testing.T) {
	terms := map[string]string{"lang": "语言"}
	in := "See [the book](https://doc.rust-lang.org/book/) for details."
	got := restoreTerms(in, terms)
	if got != in {
		t.Errorf("URL 中的 lang 被误替换：got %q, want %q", got, in)
	}
}

func TestProtectWithTerms_ShortTermsIgnored(t *testing.T) {
	terms := map[string]string{"a": "甲", "R": "R 语言"}
	got := restoreTerms("a and R are ignored.", terms)
	want := "a and R are ignored."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProtectWithTerms_PureTermText(t *testing.T) {
	// 整段恰好只有一个词条：掩码后无可翻译词语，
	// handler 会直接 Restore 输出译文（不调用引擎）
	terms := map[string]string{"borrow checker": "借用检查器"}
	masked, codes := ProtectWithTerms("borrow checker", terms)
	if masked != "$CODE_0$" {
		t.Fatalf("期望掩码文本为纯占位符，得到 %q", masked)
	}
	got := Restore(masked, codes)
	if got != "借用检查器" {
		t.Errorf("got %q, want %q", got, "借用检查器")
	}
}

func TestProtectWithTerms_EmptyTermsEqualsProtect(t *testing.T) {
	text := "Returns the number of elements."
	baseMasked, baseCodes := Protect(text)
	emptyMasked, emptyCodes := ProtectWithTerms(text, nil)
	if baseMasked != emptyMasked || len(baseCodes) != len(emptyCodes) {
		t.Errorf("空术语表应与 Protect 完全一致")
	}
}

func TestProtectWithTerms_MultipleOccurrences(t *testing.T) {
	terms := map[string]string{"trait bound": "trait 约束"}
	masked, codes := ProtectWithTerms("A trait bound is a constraint. Each trait bound narrows candidates.", terms)
	n := strings.Count(masked, "$CODE_")
	if n != 2 {
		t.Errorf("期望 2 个术语占位符，得到 %d（%q）", n, masked)
	}
	got := Restore(masked, codes)
	want := "A trait 约束 is a constraint. Each trait 约束 narrows candidates."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
