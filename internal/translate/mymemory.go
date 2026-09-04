package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MyMemoryEngine 使用 MyMemory 免费翻译 API 实现翻译引擎。
// 无需 API 密钥，匿名用户每日 5000 字符限额。
// 国内可达，作为 Google 之外的免费兜底引擎。
type MyMemoryEngine struct {
	client *http.Client
}

// NewMyMemoryEngine 创建 MyMemoryEngine 实例。
func NewMyMemoryEngine() *MyMemoryEngine {
	return &MyMemoryEngine{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Translate 调用 MyMemory API 翻译文本。
// langpair 格式为 "源语言|目标语言"，如 "en|zh-CN"。
// 源语言用 auto-detect，由 API 自动识别。
func (m *MyMemoryEngine) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return text, nil
	}

	// MyMemory 的 langpair 格式：源语言|目标语言，源语言用 auto
	langpair := fmt.Sprintf("auto-detect|%s", targetLang)

	reqURL := fmt.Sprintf(
		"https://api.mymemory.translated.net/get?q=%s&langpair=%s",
		url.QueryEscape(text),
		url.QueryEscape(langpair),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("mymemory: 构建请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mymemory: 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("mymemory: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
		ResponseStatus string `json:"responseStatus"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxHTTPResponseSize)).Decode(&result); err != nil {
		return "", fmt.Errorf("mymemory: 解析响应失败: %w", err)
	}

	if result.ResponseStatus != "200" && result.ResponseStatus != "" {
		return "", fmt.Errorf("mymemory: API 返回错误状态 %s", result.ResponseStatus)
	}

	translated := result.ResponseData.TranslatedText
	if translated == "" {
		return "", fmt.Errorf("mymemory: 翻译结果为空")
	}

	return translated, nil
}

// Name 返回引擎名称。
func (m *MyMemoryEngine) Name() string {
	return "MyMemory"
}
