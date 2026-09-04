package translate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// FallbackEngine 按顺序尝试多个翻译引擎，前一个失败则尝试下一个。
//
// 用于解决"配置了 API key 但 engine 字段未切换"或"某个引擎临时不可达"的场景。
// 例如用户配置了 openai.api_key 但 engine 仍为 google，google 超时后自动降级到 openai。
type FallbackEngine struct {
	engines []Engine
	logger  *slog.Logger
}

// NewFallbackEngine 创建降级引擎。engines 按优先级排列，第一个成功的结果会被返回。
func NewFallbackEngine(engines []Engine, logger *slog.Logger) *FallbackEngine {
	return &FallbackEngine{engines: engines, logger: logger}
}

// Translate 依次尝试每个引擎，返回第一个成功的结果。全部失败则返回最后一个错误。
func (f *FallbackEngine) Translate(ctx context.Context, text, targetLang string) (string, error) {
	var lastErr error
	for i, e := range f.engines {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		result, err := e.Translate(ctx, text, targetLang)
		if err == nil {
			if i > 0 {
				f.logger.Info("翻译引擎降级成功",
					slog.String("engine", e.Name()),
					slog.Int("fallback_index", i),
				)
			}
			return result, nil
		}
		lastErr = err
		f.logger.Warn("翻译引擎失败，尝试下一个",
			slog.String("engine", e.Name()),
			slog.Int("fallback_index", i),
			slog.String("error", err.Error()),
		)
	}
	return "", fmt.Errorf("所有翻译引擎均失败: %w", lastErr)
}

// Name 返回所有引擎名称的链式表示。
func (f *FallbackEngine) Name() string {
	names := make([]string, len(f.engines))
	for i, e := range f.engines {
		names[i] = e.Name()
	}
	return "Fallback(" + strings.Join(names, " → ") + ")"
}
