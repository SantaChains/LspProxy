package translate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	// failureThreshold 连续失败次数达到该值后触发熔断
	failureThreshold = 3
	// cooldownDuration 熔断冷却时长，期间跳过该引擎
	cooldownDuration = 60 * time.Second
)

// failureState 记录单个引擎的失败状态，用于熔断。
type failureState struct {
	mu         sync.Mutex
	count      int
	cooldownTo time.Time
}

// isCoolingDown 返回引擎是否处于冷却期。
func (s *failureState) isCoolingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.cooldownTo)
}

// recordFail 记录一次失败，达到阈值时设置冷却期。
func (s *failureState) recordFail() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.count >= failureThreshold {
		s.cooldownTo = time.Now().Add(cooldownDuration)
		s.count = 0
	}
}

// recordSuccess 记录一次成功，重置失败计数。
func (s *failureState) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count = 0
}

// FallbackEngine 按顺序尝试多个翻译引擎，前一个失败则尝试下一个。
//
// 支持两种模式：
//   - 并发竞速：前 concurrent 个引擎同时发起请求，首个成功立即返回，
//     适用于免费接口（Google、MyMemory），避免串行等待超时。
//   - 串行兜底：剩余引擎依次尝试，适用于 AI 接口，避免浪费配额。
//
// 内置熔断：连续失败 failureThreshold 次的引擎进入 cooldownDuration 冷却期，期间跳过。
// 用于避免每次都先试必败的引擎（如国内访问 Google）。
type FallbackEngine struct {
	engines    []Engine
	concurrent int // 前 concurrent 个引擎并发竞速，<=1 表示全部串行
	logger     *slog.Logger
	states     []*failureState
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
	states := make([]*failureState, len(engines))
	for i := range states {
		states[i] = &failureState{}
	}
	return &FallbackEngine{engines: engines, concurrent: concurrent, logger: logger, states: states}
}

// Translate 先并发尝试前 concurrent 个引擎，首个成功返回；全部失败后串行尝试剩余引擎。
// 处于冷却期的引擎会被跳过。
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
	active := 0

	for i := 0; i < n; i++ {
		if f.states[i].isCoolingDown() {
			f.logger.Debug("引擎冷却中，跳过",
				slog.String("engine", f.engines[i].Name()),
				slog.Int("index", i),
			)
			continue
		}
		active++
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			t, err := f.engines[idx].Translate(ctx, text, targetLang)
			ch <- result{index: idx, text: t, err: err}
		}(i)
	}

	if active == 0 {
		return "", fmt.Errorf("并发引擎全部处于冷却期")
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var lastErr error
	for r := range ch {
		if r.err == nil {
			f.states[r.index].recordSuccess()
			if r.index > 0 {
				f.logger.Info("翻译引擎降级成功（并发竞速）",
					slog.String("engine", f.engines[r.index].Name()),
					slog.Int("index", r.index),
				)
			}
			return r.text, nil
		}
		f.states[r.index].recordFail()
		lastErr = r.err
		f.logger.Warn("翻译引擎失败",
			slog.String("engine", f.engines[r.index].Name()),
			slog.Int("index", r.index),
			slog.String("error", r.err.Error()),
		)
	}

	return "", fmt.Errorf("并发引擎全部失败: %w", lastErr)
}

// trySequential 串行尝试剩余引擎，返回首个成功结果。
func (f *FallbackEngine) trySequential(ctx context.Context, text, targetLang string) (string, error) {
	start := f.concurrent
	if start < 0 {
		start = 0
	}
	if start >= len(f.engines) {
		start = 0
	}

	var lastErr error
	for i := start; i < len(f.engines); i++ {
		if f.states[i].isCoolingDown() {
			f.logger.Debug("引擎冷却中，跳过",
				slog.String("engine", f.engines[i].Name()),
				slog.Int("index", i),
			)
			continue
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		t, err := f.engines[i].Translate(ctx, text, targetLang)
		if err == nil {
			f.states[i].recordSuccess()
			if i > 0 {
				f.logger.Info("翻译引擎降级成功（串行）",
					slog.String("engine", f.engines[i].Name()),
					slog.Int("index", i),
				)
			}
			return t, nil
		}
		f.states[i].recordFail()
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

