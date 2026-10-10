package lsp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/SantaChains/LspProxy/internal/config"
	"github.com/SantaChains/LspProxy/internal/translate"
)

// stubEngine 返回固定前缀译文的桩翻译引擎
type stubEngine struct{}

func (stubEngine) Translate(_ context.Context, text, _ string) (string, error) {
	return "[译] " + text, nil
}

func (stubEngine) Name() string { return "stub" }

// newTestHandler 构造使用桩引擎的 Handler
func newTestHandler() *Handler {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewHandler(stubEngine{}, "zh-CN", logger, 600, config.DisplayTranslationOnly, nil)
}

// TestTranslateDiagnostics_RelatedInformation 验证诊断关联信息随主消息一并翻译
func TestTranslateDiagnostics_RelatedInformation(t *testing.T) {
	h := newTestHandler()

	params := json.RawMessage(`{
		"uri": "file:///d/demo/src/main.rs",
		"diagnostics": [{
			"range": {"start": {"line": 20, "character": 8}, "end": {"line": 20, "character": 12}},
			"severity": 1,
			"source": "rustc",
			"message": "use of possibly-uninitialized variable here",
			"relatedInformation": [
				{
					"location": {
						"uri": "file:///d/demo/src/main.rs",
						"range": {"start": {"line": 20, "character": 8}, "end": {"line": 20, "character": 12}}
					},
					"message": "binding declared here but left uninitialized"
				},
				{
					"location": {
						"uri": "file:///d/demo/src/main.rs",
						"range": {"start": {"line": 20, "character": 27}, "end": {"line": 20, "character": 27}}
					},
					"message": "consider assigning a value"
				}
			]
		}]
	}`)

	out, err := h.translateDiagnostics(context.Background(), params)
	if err != nil {
		t.Fatalf("translateDiagnostics 失败: %v", err)
	}

	var got PublishDiagnosticsParams
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	d := got.Diagnostics[0]
	if d.Message != "[译] use of possibly-uninitialized variable here" {
		t.Errorf("主消息未翻译: %q", d.Message)
	}
	if len(d.RelatedInformation) != 2 {
		t.Fatalf("关联信息数量错误: %d", len(d.RelatedInformation))
	}
	if want := "[译] binding declared here but left uninitialized"; d.RelatedInformation[0].Message != want {
		t.Errorf("关联信息[0] 未翻译: %q", d.RelatedInformation[0].Message)
	}
	if want := "[译] consider assigning a value"; d.RelatedInformation[1].Message != want {
		t.Errorf("关联信息[1] 未翻译: %q", d.RelatedInformation[1].Message)
	}

	// 结构字段必须保真
	if d.Source != "rustc" || d.Severity != 1 {
		t.Errorf("诊断标量字段丢失: source=%q severity=%d", d.Source, d.Severity)
	}
	if d.RelatedInformation[0].Location.URI != "file:///d/demo/src/main.rs" {
		t.Errorf("关联位置 URI 丢失: %q", d.RelatedInformation[0].Location.URI)
	}
	if len(d.RelatedInformation[0].Location.Range) == 0 {
		t.Error("关联位置 Range 丢失")
	}
	if len(d.Range) == 0 {
		t.Error("诊断 Range 丢失")
	}
}

// TestTranslateDiagnostics_NoRelatedInformation 验证无关联信息的诊断不受影响
func TestTranslateDiagnostics_NoRelatedInformation(t *testing.T) {
	h := newTestHandler()

	params := json.RawMessage(`{
		"uri": "file:///d/demo/src/main.rs",
		"diagnostics": [{"range": {}, "message": "this function has too many arguments"}]
	}`)

	out, err := h.translateDiagnostics(context.Background(), params)
	if err != nil {
		t.Fatalf("translateDiagnostics 失败: %v", err)
	}

	var got PublishDiagnosticsParams
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	if want := "[译] this function has too many arguments"; got.Diagnostics[0].Message != want {
		t.Errorf("主消息未翻译: %q", got.Diagnostics[0].Message)
	}
}

// TestTranslateCompletionItems_DetailUntouched 验证 detail 有意保持原样，
// 仅翻译 documentation（detail 在补全列表逐行显示，翻译延迟不可接受）
func TestTranslateCompletionItems_DetailUntouched(t *testing.T) {
	h := newTestHandler()

	items := []CompletionItem{
		{Label: "foo", Detail: "This is a description", Documentation: json.RawMessage(`"Some docs"`)},
		{Label: "bar", Detail: "fn bar(x: i32) -> i32"},
		{Label: "baz"},
	}

	translated, total := h.translateCompletionItems(context.Background(), items)
	if total != 1 {
		t.Errorf("总数错误: got %d want 1", total)
	}
	if translated != 1 {
		t.Errorf("翻译数错误: got %d want 1", translated)
	}
	if items[0].Detail != "This is a description" {
		t.Errorf("detail 应保持原样: %q", items[0].Detail)
	}
	if string(items[0].Documentation) != `"[译] Some docs"` {
		t.Errorf("documentation 未翻译: %s", items[0].Documentation)
	}
	if items[1].Detail != "fn bar(x: i32) -> i32" {
		t.Errorf("纯代码 detail 应保持原样: %q", items[1].Detail)
	}
}

// TestTranslateSignatureHelp_Parameters 验证签名参数文档翻译且 Label 保真
func TestTranslateSignatureHelp_Parameters(t *testing.T) {
	h := newTestHandler()

	result := json.RawMessage(`{
		"signatures": [{
			"label": "fn foo(x: i32, y: &str)",
			"documentation": "Docs about foo",
			"parameters": [
				{"label": [7, 13], "documentation": "the input number"},
				{"label": "y: &str"}
			]
		}]
	}`)

	out, err := h.translateSignatureHelp(context.Background(), result)
	if err != nil {
		t.Fatalf("translateSignatureHelp 失败: %v", err)
	}

	var got SignatureHelp
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	sig := got.Signatures[0]
	if sig.Label != "fn foo(x: i32, y: &str)" {
		t.Errorf("签名 Label 被改动: %q", sig.Label)
	}
	if !json.Valid(json.RawMessage(sig.Parameters[0].Label)) || string(sig.Parameters[0].Label) != "[7,13]" {
		t.Errorf("参数偏移对 Label 被改动: %s", sig.Parameters[0].Label)
	}
	if want := `"[译] the input number"`; string(sig.Parameters[0].Documentation) != want {
		t.Errorf("参数文档未翻译: %s", sig.Parameters[0].Documentation)
	}
	if sig.Parameters[1].Documentation != nil {
		t.Errorf("无文档参数不应新增 documentation: %s", sig.Parameters[1].Documentation)
	}
}

// 编译期接口断言
var _ translate.Engine = stubEngine{}
