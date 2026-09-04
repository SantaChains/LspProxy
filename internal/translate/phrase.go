package translate

import (
	"context"
	"strings"
	"sync"
)

// PhraseEngine 内置高频短语词典，作为引擎链路的最外层预检查。
//
// LSP 文档中存在大量固定高频短语（如 "Returns"、"Parameters"、"Examples"），
// 每次都走完整引擎链路（词汇本 → 缓存 → 在线 API）是浪费。
// PhraseEngine 在调用底层引擎前先查表，命中则零延迟返回。
//
// 匹配策略：
//   - 精确匹配（trim 空格后）
//   - 首字母大小写不敏感匹配（"Returns" 与 "returns" 视为同一短语）
//   - 仅匹配完整文本，不做子串匹配（避免误翻译长文本中的片段）
type PhraseEngine struct {
	inner Engine
	mu    sync.RWMutex
	dict  map[string]string // key: 小写短语, value: 译文
}

// NewPhraseEngine 创建内置短语词典引擎，包装底层引擎。
func NewPhraseEngine(inner Engine) *PhraseEngine {
	return &PhraseEngine{
		inner: inner,
		dict:  buildPhraseDict(),
	}
}

// Translate 先查内置短语词典，命中则直接返回；否则调用底层引擎。
func (p *PhraseEngine) Translate(ctx context.Context, text, targetLang string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text, nil
	}

	// 仅对短文本查词典（长文本不可能是单个短语）
	if len(trimmed) <= 80 {
		p.mu.RLock()
		translated, ok := p.dict[strings.ToLower(trimmed)]
		p.mu.RUnlock()
		if ok {
			return translated, nil
		}
	}

	return p.inner.Translate(ctx, text, targetLang)
}

// Name 返回引擎名称。
func (p *PhraseEngine) Name() string {
	return "PhraseDict(" + p.inner.Name() + ")"
}

// buildPhraseDict 构建内置高频短语词典。
// 覆盖 rust-analyzer 和 clangd 文档中最常见的固定短语。
// key 统一小写，查询时将输入转小写后匹配。
func buildPhraseDict() map[string]string {
	return map[string]string{
		// ── 章节标题 ──
		"returns":         "返回值",
		"return value":    "返回值",
		"return values":   "返回值",
		"parameters":      "参数",
		"parameter":       "参数",
		"params":          "参数",
		"arguments":       "参数",
		"argument":        "参数",
		"args":            "参数",
		"examples":        "示例",
		"example":         "示例",
		"remarks":         "备注",
		"remark":          "备注",
		"notes":           "备注",
		"note":            "备注",
		"see also":        "另请参阅",
		"see":             "另请参阅",
		"description":     "描述",
		"type information": "类型信息",
		"invariants":      "不变量",
		"invariant":       "不变量",
		"panics":          "恐慌",
		"panic":           "恐慌",
		"errors":          "错误",
		"error":           "错误",
		"safety":          "安全性",
		"aborts":          "中止",
		"abort":           "中止",
		"side effects":    "副作用",
		"side effect":     "副作用",
		"thread safety":   "线程安全",
		"since":           "自",
		"deprecated":      "已弃用",
		"warning":         "警告",
		"warnings":        "警告",
		"caveat":          "注意事项",
		"caveats":         "注意事项",
		"todo":            "待办",
		"todos":           "待办",
		"overview":        "概述",
		"summary":         "摘要",
		"usage":           "用法",
		"syntax":          "语法",
		"semantics":       "语义",
		"behavior":        "行为",
		"behaviour":       "行为",
		"details":         "详情",
		"detail":          "详情",
		"background":      "背景",
		"motivation":      "动机",
		"rationale":       "原理",

		// ── 常见句子开头 ──
		"this function":  "此函数",
		"this method":    "此方法",
		"this struct":    "此结构体",
		"this trait":     "此 trait",
		"this type":      "此类型",
		"this enum":      "此枚举",
		"this module":    "此模块",
		"this crate":     "此 crate",
		"this package":   "此包",
		"this class":     "此类",
		"this object":    "此对象",
		"the function":   "该函数",
		"the method":     "该方法",
		"the struct":     "该结构体",
		"the type":       "该类型",
		"the class":      "该类",
		"returns a":      "返回一个",
		"returns an":     "返回一个",
		"returns the":    "返回",
		"returns true if": "如果...返回 true",
		"returns false if": "如果...返回 false",
		"gets the":       "获取",
		"sets the":       "设置",
		"creates a":      "创建一个",
		"creates a new":  "创建一个新的",
		"creates an":     "创建一个",
		"creates a new instance of": "创建一个新的",
		"may panic if":   "如果...可能 panic",
		"returns an error if": "如果...返回错误",
		"returns none if": "如果...返回 None",
		"returns some if": "如果...返回 Some",
		"returns ok if":  "如果...返回 Ok",
		"returns err if": "如果...返回 Err",

		// ── 布尔/空值常量（保留原文） ──
		"true":  "true",
		"false": "false",
		"nil":   "nil",
		"null":  "null",
		"none":  "None",
		"some":  "Some",
		"ok":    "Ok",
		"err":   "Err",

		// ── 数量词 ──
		"no":      "无",
		"none.":   "无。",
		"nothing": "无",
		"empty":   "空",
		"void":    "void",
		"unit":    "unit",
	}
}
