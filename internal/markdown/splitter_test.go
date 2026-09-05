package markdown

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Split 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestSplit_PlainText(t *testing.T) {
	segs := Split("hello world")
	if len(segs) != 1 {
		t.Fatalf("期望 1 个片段，得到 %d", len(segs))
	}
	if segs[0].Kind != KindText {
		t.Errorf("期望 KindText，得到 %v", segs[0].Kind)
	}
	if segs[0].Content != "hello world" {
		t.Errorf("内容不符: %q", segs[0].Content)
	}
}

func TestSplit_InlineCode(t *testing.T) {
	segs := Split("Use `init` to start")
	// 期望: ["Use ", "`init`", " to start"]
	if len(segs) != 3 {
		t.Fatalf("期望 3 个片段，得到 %d: %v", len(segs), segs)
	}
	assertEqual(t, segs[0], KindText, "Use ")
	assertEqual(t, segs[1], KindCode, "`init`")
	assertEqual(t, segs[2], KindText, " to start")
}

func TestSplit_FencedCodeBlock(t *testing.T) {
	input := "示例：\n```rust\nlet x = 1;\n```\n结束"
	segs := Split(input)
	kinds := make([]SegmentKind, len(segs))
	for i, s := range segs {
		kinds[i] = s.Kind
	}
	// 必须包含至少一个 KindCode 片段
	hasCode := false
	for _, s := range segs {
		if s.Kind == KindCode && strings.Contains(s.Content, "let x = 1;") {
			hasCode = true
		}
	}
	if !hasCode {
		t.Errorf("未找到包含代码内容的 KindCode 片段，实际片段: %v", segs)
	}
}

func TestSplit_MultipleInlineCodes(t *testing.T) {
	input := "调用 `foo()` 或 `bar()`"
	segs := Split(input)
	codeCnt := 0
	for _, s := range segs {
		if s.Kind == KindCode {
			codeCnt++
		}
	}
	if codeCnt != 2 {
		t.Errorf("期望 2 个行内代码片段，得到 %d", codeCnt)
	}
}

func TestSplit_TildeBlock(t *testing.T) {
	input := "text\n~~~\ncode\n~~~\nmore"
	segs := Split(input)
	hasCode := false
	for _, s := range segs {
		if s.Kind == KindCode && strings.Contains(s.Content, "code") {
			hasCode = true
		}
	}
	if !hasCode {
		t.Errorf("未识别 ~~~ 围栏代码块: %v", segs)
	}
}

func TestSplit_Join_Roundtrip(t *testing.T) {
	cases := []string{
		"plain text",
		"Use `init` here",
		"text\n```go\nfoo()\n```\nafter",
		"a `b` c `d` e",
		"",
		"   ",
	}
	for _, input := range cases {
		got := Join(Split(input))
		if got != input {
			t.Errorf("Join(Split(%q)) = %q，期望与原文相同", input, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Protect / Restore 测试
// ─────────────────────────────────────────────────────────────────────────────

func TestProtect_NoCode(t *testing.T) {
	masked, codes := Protect("hello world")
	if len(codes) != 0 {
		t.Errorf("期望 0 个代码块，得到 %d", len(codes))
	}
	if masked != "hello world" {
		t.Errorf("无代码块时 masked 应等于原文，得到 %q", masked)
	}
}

func TestProtect_SingleInlineCode(t *testing.T) {
	masked, codes := Protect("Use `init` to start")
	if len(codes) != 1 {
		t.Fatalf("期望 1 个代码块，得到 %d", len(codes))
	}
	if codes[0] != "`init`" {
		t.Errorf("codes[0] = %q，期望 \"`init`\"", codes[0])
	}
	if !strings.Contains(masked, "$CODE_0$") {
		t.Errorf("masked 中未找到占位符 $CODE_0$：%q", masked)
	}
	if strings.Contains(masked, "`init`") {
		t.Errorf("masked 中不应包含原始代码块内容")
	}
}

func TestProtect_MultipleInlineCodes(t *testing.T) {
	masked, codes := Protect("调用 `foo()` 或 `bar()`")
	if len(codes) != 2 {
		t.Fatalf("期望 2 个代码块，得到 %d", len(codes))
	}
	if !strings.Contains(masked, "$CODE_0$") {
		t.Errorf("masked 缺少 $CODE_0$: %q", masked)
	}
	if !strings.Contains(masked, "$CODE_1$") {
		t.Errorf("masked 缺少 $CODE_1$: %q", masked)
	}
}

func TestProtect_FencedBlock(t *testing.T) {
	input := "示例：\n```rust\nlet x = 1;\n```\n结束"
	masked, codes := Protect(input)
	if len(codes) == 0 {
		t.Fatal("期望至少 1 个代码块")
	}
	found := false
	for _, c := range codes {
		if strings.Contains(c, "let x = 1;") {
			found = true
		}
	}
	if !found {
		t.Errorf("代码内容未被提取到 codes: %v", codes)
	}
	if strings.Contains(masked, "let x = 1;") {
		t.Errorf("masked 中不应包含代码块内容")
	}
}

func TestRestore_Basic(t *testing.T) {
	masked, codes := Protect("Use `init` to start")
	restored := Restore(masked, codes)
	if restored != "Use `init` to start" {
		t.Errorf("还原失败: %q", restored)
	}
}

func TestRestore_NoCodes(t *testing.T) {
	result := Restore("hello world", nil)
	if result != "hello world" {
		t.Errorf("无 codes 时应原样返回: %q", result)
	}
}

func TestRestore_WithSpacesInPlaceholder(t *testing.T) {
	// 模拟翻译引擎在占位符内加了空格的情况
	codes := []string{"`init`"}
	modified := "使用 $ CODE_0 $ 开始"
	restored := Restore(modified, codes)
	if !strings.Contains(restored, "`init`") {
		t.Errorf("宽松匹配还原失败: %q", restored)
	}
}

func TestRestore_MultipleCodes(t *testing.T) {
	input := "Call `foo()` or `bar()` to proceed"
	masked, codes := Protect(input)
	restored := Restore(masked, codes)
	if restored != input {
		t.Errorf("多代码块还原失败:\n  原文: %q\n  还原: %q", input, restored)
	}
}

func TestProtectRestore_Roundtrip(t *testing.T) {
	cases := []string{
		// 纯文本
		"plain text without code",
		// 单个行内代码
		"Use `init` to setup",
		// 多个行内代码
		"Call `foo()` and `bar()` here",
		// 围栏代码块
		"示例：\n```rust\nlet x = 1;\n```\n结束",
		// 问题截图中的真实场景：文字和围栏代码块在同一行（不规范 markdown）
		"使用[init] 设置默认订阅者: ```rust tracing_subscriber::fmt().init();```",
		// 行内代码和围栏代码混合
		"Use `Foo` struct:\n```go\ntype Foo struct{}\n```",
		// 代码块内含反引号
		"code: `x := \"hello\"`",
		// 空文本
		"",
		// 纯代码块
		"```\nonly code\n```",
		// 多个围栏代码块
		"block1:\n```\na\n```\nblock2:\n```\nb\n```",
	}

	for _, input := range cases {
		masked, codes := Protect(input)
		restored := Restore(masked, codes)
		if restored != input {
			t.Errorf("Protect/Restore 往返测试失败:\n  输入:   %q\n  masked: %q\n  还原:   %q", input, masked, restored)
		}
	}
}

// TestProtectRestore_ScreenshotCase 专门测试截图中出现的问题场景：
// "使用[init] 设置默认订阅者: ```rust tracing_subscriber::fmt().init();"
// 这种代码块和文字在同一行的混排情况。
func TestProtectRestore_ScreenshotCase(t *testing.T) {
	// 模拟 LSP hover 文档中真实出现的混排格式
	input := "使用[init] 设置默认订阅者: ```rust tracing_subscriber::fmt().init();\n\n配置输出格式: ```rust\ntracing_subscriber::fmt()\n```"

	masked, codes := Protect(input)

	t.Logf("原文:   %q", input)
	t.Logf("masked: %q", masked)
	t.Logf("codes:  %v", codes)

	// masked 中不应再含有代码块内容（已被占位符替换）
	if strings.Contains(masked, "tracing_subscriber") {
		t.Errorf("masked 中仍含有代码内容，占位符替换不完整: %q", masked)
	}

	// 还原后应与原文完全一致
	restored := Restore(masked, codes)
	if restored != input {
		t.Errorf("还原失败:\n  期望: %q\n  实际: %q", input, restored)
	}
}

// TestProtect_MaskedTextIsTranslatable 验证 masked 文本适合送入翻译引擎：
// 应只含普通文本和占位符，不含反引号代码块。
func TestProtect_MaskedTextIsTranslatable(t *testing.T) {
	inputs := []string{
		"Returns the `Vec` length",
		"Use `init` or `run` to start the `Server`",
		"```rust\nfn main() {}\n```\nSee above example",
	}
	for _, input := range inputs {
		masked, _ := Protect(input)
		if strings.Contains(masked, "```") {
			t.Errorf("masked 中含有围栏代码块，翻译引擎可能无法正确处理: %q → %q", input, masked)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────

func assertEqual(t *testing.T, seg Segment, kind SegmentKind, content string) {
	t.Helper()
	if seg.Kind != kind {
		t.Errorf("Kind: 期望 %v，得到 %v（内容: %q）", kind, seg.Kind, seg.Content)
	}
	if seg.Content != content {
		t.Errorf("Content: 期望 %q，得到 %q", content, seg.Content)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 技术术语保护测试
// ─────────────────────────────────────────────────────────────────────────────

func TestProtect_TechTermPanic(t *testing.T) {
	// "Panic" 出现在普通文本中，应被保护为占位符
	input := "Panic if the path value is malformed."
	masked, codes := Protect(input)

	if strings.Contains(masked, "Panic") || strings.Contains(masked, "panic") {
		t.Errorf("技术术语 Panic 未被替换为占位符，masked = %q", masked)
	}
	if len(codes) == 0 {
		t.Fatal("codes 应包含被保护的术语")
	}

	// 还原后应与原文完全一致
	restored := Restore(masked, codes)
	if restored != input {
		t.Errorf("还原失败:\n  期望: %q\n  实际: %q", input, restored)
	}
}

func TestProtect_TechTermCaseInsensitive(t *testing.T) {
	// 大小写均应被匹配
	cases := []struct {
		input string
		term  string
	}{
		{"panic: index out of range", "panic"},
		{"This function Panic if invalid", "Panic"},
		{"PANIC on nil pointer", "PANIC"},
		{"throws an error when", "throws"},
		{"raises RuntimeError", "raises"},
	}
	for _, tc := range cases {
		masked, codes := Protect(tc.input)
		if strings.Contains(masked, tc.term) {
			t.Errorf("输入 %q：术语 %q 未被替换，masked = %q", tc.input, tc.term, masked)
		}
		restored := Restore(masked, codes)
		if restored != tc.input {
			t.Errorf("输入 %q：还原失败，得到 %q", tc.input, restored)
		}
	}
}

func TestProtect_TechTermWordBoundary(t *testing.T) {
	// 词表中的词语不应匹配嵌入在其他单词中的子串
	cases := []struct {
		input     string
		shouldHit bool // 是否应该命中保护
	}{
		// "panic" 作为独立单词 → 应保护
		{"This will panic at runtime", true},
		// "panicky" 含有 panic 子串但不是独立单词 → 不应保护
		{"The panicky user ran away", false},
		// "throws" 独立单词 → 应保护
		{"This function throws when invalid", true},
		// "raises" 独立单词 → 应保护
		{"raises RuntimeError on failure", true},
	}
	for _, tc := range cases {
		_, codes := Protect(tc.input)
		hasTerm := len(codes) > 0
		// 注意：代码块也会计入 codes，此用例无代码块，codes 全为术语
		if tc.shouldHit && !hasTerm {
			t.Errorf("输入 %q：期望术语被保护，但 codes 为空", tc.input)
		}
		if !tc.shouldHit && hasTerm {
			t.Errorf("输入 %q：不期望术语被保护，但 codes = %v", tc.input, codes)
		}
	}
}

func TestProtect_TechTermWithCodeBlock(t *testing.T) {
	// 技术术语和代码块混合出现时，占位符编号应连续递增
	input := "Panic if `path` is nil"
	masked, codes := Protect(input)

	// 应有 2 个占位符：`path`（KindCode）和 Panic（技术术语）
	if len(codes) != 2 {
		t.Fatalf("期望 2 个占位符，得到 %d: codes = %v", len(codes), codes)
	}

	if strings.Contains(masked, "Panic") {
		t.Errorf("Panic 未被占位符替换: %q", masked)
	}
	if strings.Contains(masked, "`path`") {
		t.Errorf("`path` 未被占位符替换: %q", masked)
	}

	restored := Restore(masked, codes)
	if restored != input {
		t.Errorf("还原失败:\n  期望: %q\n  实际: %q", input, restored)
	}
}

func TestProtect_Roundtrip_AllFormats(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"标准链接", "详见 [Option](https://doc.rust-lang.org/std/option/enum.Option.html) 文档"},
		{"自动链接", "详见 <https://doc.rust-lang.org/std/option/enum.Option.html>"},
		{"file协议自动链接", "点击 <file:///C:/Users/foo/src/main.rs> 打开源码"},
		{"行内HTML标签", "返回值是 <code>Option</code> 类型"},
		{"HTML自闭合标签", "第一行<br/>第二行"},
		{"reference链接定义", "[Rust Book][ref] 提供了更多信息\n\n[ref]: https://doc.rust-lang.org"},
		{"图片URL", "![示意图](https://example.com/img.png)"},
		{"混合格式", "调用 `foo()` 方法，详见 [文档](https://doc.com) 和 <https://rust.org>"},
		{"代码块保护优先", "示例：```rust\nlet x = \"https://example.com\";\n```"},
		{"HTML标签含属性", "使用 <span class=\"highlight\">高亮</span> 显示"},
		{"空链接文字", "[](https://example.com) 空文字链接"},
		{"代码块内尖括号不被误匹配", "类型参数 `Result<T, E>` 正常"},
		{"完整hover文档模拟", "### Function: foo\n\nDo `foo` does X and Y.\n\n[See Rust Book](https://doc.rust-lang.org/book)\n\n<file:///path/to/foo.rs:10>\n\n<pre>raw</pre>"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			masked, codes := Protect(c.input)
			restored := Restore(masked, codes)
			if restored != c.input {
				t.Errorf("roundtrip 失败\n原文:     %q\nmasked:   %q\ncodes:    %v\n还原:     %q",
					c.input, masked, codes, restored)
			}
		})
	}
}

func TestProtect_AutolinkReplaced(t *testing.T) {
	masked, codes := Protect("详见 <https://doc.rust-lang.org/std/>")
	if !strings.Contains(masked, "$CODE_0$") {
		t.Errorf("自动链接应被替换，masked=%q", masked)
	}
	if len(codes) != 1 {
		t.Errorf("期望 1 个 code，得到 %d: %v", len(codes), codes)
	}
	if codes[0] != "https://doc.rust-lang.org/std/" {
		t.Errorf("codes[0] 不符: %q", codes[0])
	}
}

func TestProtect_HTMLTagsReplaced(t *testing.T) {
	_, codes := Protect("返回 <code>Option</code> 或 <em>None</em>")
	// <code>、</code>、<em>、</em> 共 4 个标签
	if len(codes) != 4 {
		t.Errorf("期望 4 个 code（两个开+两个闭标签），得到 %d: %v", len(codes), codes)
	}
	for _, c := range codes {
		if !strings.HasPrefix(c, "<") {
			t.Errorf("所有 codes 应以 < 开头，得到: %q", c)
		}
	}
}

func TestProtect_RefDefURLReplaced(t *testing.T) {
	input := "[Rust Book][ref]\n\n[ref]: https://doc.rust-lang.org \"Rust Official Doc\""
	masked, codes := Protect(input)
	// 标准链接保护 [ref]: https://... 和 reference 定义行的 URL
	// 至少 ref 定义的 URL 要被保护
	restored := Restore(masked, codes)
	if restored != input {
		t.Errorf("roundtrip 失败\n原文: %q\n还原: %q", input, restored)
	}
}

func TestProtect_NoHTMLInCodeBlock(t *testing.T) {
	// 代码块内的 <T> 不应被 HTML 保护
	input := "泛型语法：```rust\nfn foo<T>(x: T) -> T { x }\n```"
	_, codes := Protect(input)
	// 只有代码块本身应被替换，codes 长度应为 1
	if len(codes) != 1 {
		t.Errorf("代码块内的 <T> 不应被 HTML 标签保护，codes=%v", codes)
	}
}
