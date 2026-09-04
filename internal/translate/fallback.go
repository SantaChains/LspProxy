package translate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// FallbackEngine 按顺序尝试多个翻译引擎，前一个失败则尝试下一个。
//
// 支持两种模式：
//   - 并发竞速：前 concurrent 个引擎同时发起请求，首个成功立即返回，
//     适用于免费接口（Google、Bing、MyMemory），避免串行等待超时。
//   - 串行兜底：剩余引擎依次尝试，适用于 AI 接口，避免浪费配额。
//
// 用于解决"配置了 API key 但 engine 字段未切换"或"某个引擎临时不可达"的场景。
type FallbackEngine struct {
	engines    []Engine
	concurrent int // 前 concurrent 个引擎并发竞速，<=1 表示全部串行
	logger     *slog.Logger
}

// NewFallbackEngine 创建降级引擎。
// concurrent 指定前多少个引擎并发竞速，建议传入免费引擎的数量。
func NewFallbackEngine(engines []Engine, concurrent int, logger *slog.Logger) *FallbackEngine {
	if concurrent < 0 {
		concurrent = 0
	}
	if concurrent > len(engines) {
		concurrent = len(engines)
	}
	return &FallbackEngine{engines: engines, concurrent: concurrent, logger: logger}
}

// Translate 先并发尝试前 concurrent 个引擎，首个成功返回；全部失败后串行尝试剩余引擎。
func (f *FallbackEngine) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if f.concurrent > 1 {
		result, err := f.tryConcurrent(ctx, text, targetLang)
		if err == nil {
			return result, nil
		}
		f.logger.Debug("并发引擎全部失败，进入串行兜底",
			slog.Int("concurrent_count", f.concurrent),
			slog.String("error", err.Error()),
		)
	}
	return f.trySequential(ctx, text, targetLang)
}

// tryConcurrent 并发尝试前 concurrent 个引擎，首个成功返回。
func (f *FallbackEngine) tryConcurrent(ctx context.Context, text, targetLang string) (string, error) {
	n := f.concurrent
	if n > len(f.engines) {
		n = len(f.engines)
	}

	type result struct {
		index int
		text  string
		err   error
	}

	ch := make(chan result, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			t, err := f.engines[idx].Translate(ctx, text, targetLang)
			ch <- result{index: idx, text: t, err: err}
		}(i)
	}

	// 等待所有 goroutine 完成后关闭 channel，避免泄漏
	go func() {
		wg.Wait()
		close(ch)
	}()

	var lastErr error
	for r := range ch {
		if r.err == nil {
			if r.index > 0 {
				f.logger.Info("翻译引擎降级成功（并发竞速）",
					slog.String("engine", f.engines[r.index].Name()),
					slog.Int("index", r.index),
				)
			}
			return r.text, nil
		}
		lastErr = r.err
		f.logger.Warn("翻译引擎失败",
			slog.String("engine", f.engines[r.index].Name()),
			slog.Int("index", r.index),
			slog.String("error", r.err.Error()),
		)
	}

	return "", fmt.Errorf("并发引擎全部失败: %w", lastErr)
}

// trySequential 串行尝试所有引擎（或仅剩余的），返回首个成功结果。
func (f *FallbackEngine) trySequential(ctx context.Context, text, targetLang string) (string, error) {
	start := f.concurrent
	if start < 0 {
		start = 0
	}
	if start >= len(f.engines) {
		start = 0 // concurrent=0 时从头开始
	}

	var lastErr error
	for i := start; i < len(f.engines); i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		t, err := f.engines[i].Translate(ctx, text, targetLang)
		if err == nil {
			if i > 0 {
				f.logger.Info("翻译引擎降级成功（串行）",
					slog.String("engine", f.engines[i].Name()),
					slog.Int("index", i),
				)
			}
			return t, nil
		}
		lastErr = err
		f.logger.Warn("翻译引擎失败，尝试下一个",
			slog.String("engine", f.engines[i].Name()),
			slog.Int("index", i),
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
	mode := "serial"
	if f.concurrent > 1 {
		mode = fmt.Sprintf("concurrent(%d)+serial", f.concurrent)
	}
	return fmt.Sprintf("Fallback[%s](%s)", mode, strings.Join(names, " → "))
}
