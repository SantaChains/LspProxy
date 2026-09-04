// Package translate 提供翻译引擎的工厂函数。
package translate

import (
	"fmt"
	"log/slog"

	"github.com/SantaChains/LspProxy/internal/config"
	"github.com/SantaChains/LspProxy/internal/glossary"
)

// New 根据配置创建对应的翻译引擎，并包装三级缓存（内存 LRU + 磁盘词典）、
// 并发合并和术语词汇本。
//
// lspName 为当前代理的 LSP 可执行文件名（如 "rust-analyzer"），用于加载 LSP 专属词汇本。
// 传空字符串时仅使用全局词汇本。
//
// 引擎链路（从外到内）：
//
//	GlossaryEngine → SingleflightEngine → DictEngine（内存LRU + 磁盘词典 + FallbackEngine）
//
// FallbackEngine 内部按优先级排列多个在线引擎，前一个失败自动尝试下一个，
// 避免因单个引擎不可达导致翻译全部失败。
//
// 查询顺序：
//  1. LSP 专属词汇本（纯内存，最高优先级）
//  2. 全局词汇本（纯内存）
//  3. [SingleflightEngine] 合并并发请求（相同文本只发起一次 API 调用）
//  4. 内存 LRU 缓存
//  5. 磁盘 JSON 词典
//  6. FallbackEngine：在线翻译 API（主引擎 → 备用引擎）
func New(cfg *config.Config, lspName string, logger *slog.Logger) (Engine, error) {
	engines, err := buildEngines(cfg, logger)
	if err != nil {
		return nil, err
	}

	var base Engine
	if len(engines) == 1 {
		base = engines[0]
	} else {
		base = NewFallbackEngine(engines, logger)
	}

	// 内存缓存上限（MB → 字节）
	cacheMB := cfg.Proxy.CacheSize
	if cacheMB <= 0 {
		cacheMB = 30
	}
	memoryLimit := int64(cacheMB) * 1024 * 1024

	// 磁盘词典路径
	dictPath := cfg.Proxy.DictFile
	if dictPath == "" {
		dictPath = config.DefaultDictFile()
	}

	// 磁盘词典最大条目数（0 表示不限制）
	dictMaxEntries := cfg.Proxy.DictMaxEntries

	disk, err := NewDiskDict(dictPath, dictMaxEntries)
	if err != nil {
		// 磁盘词典初始化失败时降级为纯内存缓存，不影响代理正常运行
		base = NewCachedEngine(base, memoryLimit)
	} else {
		base = NewDictEngine(base, memoryLimit, disk)
	}

	// ── 并发合并层：确保对相同文本的并发翻译请求只发起一次 API 调用 ──
	// 位于 DictEngine/CachedEngine 之外、GlossaryEngine 之内，
	// 对所有需要网络 IO 的路径均有效（词汇本命中是纯内存操作，无需此层）。
	base = NewSingleflightEngine(base)

	// ── 术语词汇本层（最高优先级）──
	glossaryDir := cfg.Proxy.GlossaryDir
	if glossaryDir == "" {
		glossaryDir = config.DefaultGlossaryDir()
	}

	var lspNames []string
	if lspName != "" {
		lspNames = []string{lspName}
	}

	g := glossary.New(glossaryDir, lspNames, logger)
	return glossary.NewGlossaryEngine(base, g, lspName, logger), nil
}

// buildEngines 根据配置构建在线引擎列表（按优先级排列）。
//
// 规则：
//   - 用户指定的 engine 排第一
//   - 若配置了 openai.api_key，将 openai 加入列表（若未被指定为主引擎）
//   - google 始终可用，作为最终兜底
//
// 这样即使用户忘了切换 engine，只要配置了 openai，google 失败后会自动降级到 openai。
func buildEngines(cfg *config.Config, logger *slog.Logger) ([]Engine, error) {
	primary := cfg.Translate.Engine
	if primary == "" {
		primary = "google"
	}

	oaiConfigured := cfg.Translate.OpenAI.APIKey != "" &&
		cfg.Translate.OpenAI.BaseURL != "" &&
		cfg.Translate.OpenAI.Model != ""

	var engines []Engine

	switch primary {
	case "google":
		engines = append(engines, NewGoogleEngine())
		if oaiConfigured {
			engines = append(engines, buildOpenAIEngine(cfg, logger))
		}

	case "openai":
		if !oaiConfigured {
			return nil, fmt.Errorf("translate: openai 引擎需要配置 base_url、model、api_key")
		}
		engines = append(engines, buildOpenAIEngine(cfg, logger))
		engines = append(engines, NewGoogleEngine()) // google 作为兜底

	default:
		return nil, fmt.Errorf("translate: 不支持的翻译引擎 %q（可选值：google、openai）", primary)
	}

	names := make([]string, len(engines))
	for i, e := range engines {
		names[i] = e.Name()
	}
	logger.Info("翻译引擎链已构建",
		slog.String("primary", primary),
		slog.Any("fallback_chain", names),
	)

	return engines, nil
}

// buildOpenAIEngine 从配置构建 OpenAI 兼容引擎。
func buildOpenAIEngine(cfg *config.Config, logger *slog.Logger) Engine {
	oaiCfg := cfg.Translate.OpenAI
	promptFile := oaiCfg.PromptFile
	if promptFile == "" {
		promptFile = config.DefaultPromptFile()
	}
	loader := NewPromptLoader(promptFile, logger)
	return NewOpenAIEngine(oaiCfg.BaseURL, oaiCfg.APIKey, oaiCfg.Model, oaiCfg.ThinkingMode, loader)
}
