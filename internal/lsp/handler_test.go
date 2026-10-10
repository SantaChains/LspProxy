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

// 编译期接口断言
var _ translate.Engine = stubEngine{}
