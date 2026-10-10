package markdown_test

import (
	"strings"
	"testing"

	"github.com/SantaChains/LspProxy/internal/markdown"
)

// TestProtect_LinkTitlePreserved 验证标准链接的 title 部分保留不被丢弃
func TestProtect_LinkTitlePreserved(t *testing.T) {
	text := `[docs](https://example.com "the docs")`
	masked, codes := markdown.Protect(text)

	if !strings.Contains(masked, `"the docs"`) {
		t.Errorf("title 被丢弃: %q", masked)
	}
	if strings.Contains(masked, "https://") {
		t.Errorf("URL 未保护: %q", masked)
	}

	// 模拟翻译改动链接文字后还原
	translated := strings.Replace(masked, "[docs]", "[文档]", 1)
	restored := markdown.Restore(translated, codes)
	if want := `[文档](https://example.com "the docs")`; restored != want {
		t.Errorf("还原失败: got %q want %q", restored, want)
	}
}

// TestProtect_BareURL 验证裸 URL（无语法包裹）被保护并还原
func TestProtect_BareURL(t *testing.T) {
	text := `more details at https://doc.rust-lang.org/std/option then continue`
	masked, codes := markdown.Protect(text)

	if strings.Contains(masked, "doc.rust-lang.org") {
		t.Errorf("裸 URL 未保护: %q", masked)
	}

	restored := markdown.Restore(masked, codes)
	if !strings.Contains(restored, "https://doc.rust-lang.org/std/option") {
		t.Errorf("裸 URL 还原失败: %q", restored)
	}
}

// TestProtect_BareURLTrailingPunctuation 验证裸 URL 尾部标点不被吞入占位符
func TestProtect_BareURLTrailingPunctuation(t *testing.T) {
	text := `see https://example.com/docs, and then visit https://example.com.`
	masked, codes := markdown.Protect(text)

	if !strings.Contains(masked, ", and then visit") {
		t.Errorf("逗号被吞入 URL: %q", masked)
	}

	restored := markdown.Restore(masked, codes)
	if want := "see https://example.com/docs, and then visit https://example.com."; restored != want {
		t.Errorf("还原失败: got %q want %q", restored, want)
	}
}

// TestProtect_InlineCodeURLNotBareURL 验证行内代码中的 URL 由代码层保护，语义不变
func TestProtect_InlineCodeURLNotDoubleProtected(t *testing.T) {
	text := "use `https://example.com/api` as endpoint"
	masked, codes := markdown.Protect(text)

	if strings.Contains(masked, "https://") {
		t.Errorf("行内代码 URL 未保护: %q", masked)
	}

	restored := markdown.Restore(masked, codes)
	if restored != text {
		t.Errorf("往返不等价: got %q want %q", restored, text)
	}
}
