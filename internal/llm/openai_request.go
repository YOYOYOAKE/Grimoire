package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/types"
)

func (c *OpenAIClient) translateAttempt(
	ctx context.Context,
	cfg config.Config,
	shape string,
	naturalText string,
	systemPrompt string,
	userPrompt string,
	attempt int,
) (types.TranslationResult, error) {
	start := time.Now()
	content, err := c.requestLLMContent(ctx, cfg, shape, naturalText, systemPrompt, userPrompt, attempt, start)
	if err != nil {
		return types.TranslationResult{}, err
	}

	c.logger.Info("llm assistant raw content",
		"shape", shape,
		"attempt", attempt,
		"content", content,
		"content_len", len(content),
	)
	result, err := parsePromptJSON(content)
	if err != nil {
		c.logger.Error("llm output json parse failed",
			"shape", shape,
			"attempt", attempt,
			"content", content,
			"content_len", len(content),
			"error", err,
		)
		return types.TranslationResult{}, &parseOutputError{cause: fmt.Errorf("解析 LLM 输出 JSON 失败: %w", err)}
	}

	c.logger.Info("llm translated",
		"shape", shape,
		"attempt", attempt,
		"positive_prompt", result.PositivePrompt,
		"negative_prompt", result.NegativePrompt,
		"character_count", len(result.Characters),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return result, nil
}

func (c *OpenAIClient) requestLLMContent(
	ctx context.Context,
	cfg config.Config,
	shape string,
	naturalText string,
	systemPrompt string,
	userPrompt string,
	attempt int,
	start time.Time,
) (string, error) {
	body := map[string]any{
		"model": cfg.LLM.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.2,
		"stream":      false,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("构建 LLM 请求失败: %w", err)
	}

	url := buildLLMChatCompletionsURL(cfg)
	c.logger.Info("llm http request start",
		"url", url,
		"model", cfg.LLM.Model,
		"shape", shape,
		"attempt", attempt,
		"timeout_sec", cfg.LLM.TimeoutSec,
		"prompt_len", len(naturalText),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("创建 LLM 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.LLM.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("llm http request failed",
			"model", cfg.LLM.Model,
			"shape", shape,
			"attempt", attempt,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()
	c.logger.Info("llm http response received",
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"attempt", attempt,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("llm response read failed",
			"status", resp.StatusCode,
			"attempt", attempt,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", fmt.Errorf("读取 LLM 响应失败: %w", err)
	}
	c.logger.Info("llm response body read",
		"status", resp.StatusCode,
		"attempt", attempt,
		"bytes", len(respBody),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM 返回非 200: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, err := extractAssistantContent(respBody)
	if err != nil {
		return "", &parseOutputError{cause: fmt.Errorf("解析 LLM 响应失败: %w, body=%s", err, truncate(string(respBody), 400))}
	}
	return content, nil
}

func buildLLMChatCompletionsURL(cfg config.Config) string {
	base := strings.TrimSpace(cfg.LLM.BaseURL)
	if cfg.LLM.Provider == config.ProviderOpenAICustom {
		base = ensureOpenAICustomV1BaseURL(base)
	} else {
		base = strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + "/chat/completions"
}

func ensureOpenAICustomV1BaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/v1"
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path
	default:
		parsed.Path = path + "/v1"
	}
	return parsed.String()
}
