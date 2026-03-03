package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/types"
)

type OpenAIClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

type parseOutputError struct {
	cause error
}

func (e *parseOutputError) Error() string {
	if e == nil || e.cause == nil {
		return "parse output error"
	}
	return e.cause.Error()
}

func NewOpenAIClient(cfg *config.Manager, logger *slog.Logger) *OpenAIClient {
	return &OpenAIClient{
		cfg:        cfg,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

func (c *OpenAIClient) Translate(ctx context.Context, naturalText string, shape string) (types.TranslationResult, error) {
	cfg := c.cfg.Snapshot()
	requestTimeout := time.Duration(cfg.LLM.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	systemPrompt := `
You translate Chinese natural language image requests into NovelAI-friendly English tag prompts.

Output rules:
1) Output JSON only, no extra text.
2) Schema:
{
  "positivePrompt":"...",
  "negativePrompt":"...",
  "characterPrompts":[
    {
      "charPositivePrompt":"...",
      "charUnconcentPrompt":"...",
      "centers":{"x":1,"y":"A"}
    }
  ]
}
3) positivePrompt and negativePrompt are required.
4) characterPrompts can be empty.
5) If characterPrompts is not empty, each item must include non-empty:
   - charPositivePrompt
   - charUnconcentPrompt
   - centers.x
   - centers.y
6) centers.x must be integer 1..5 where 1=top and 5=bottom.
7) centers.y must be letter A..E where A=left and E=right.
`
	userPrompt := fmt.Sprintf("shape=%s\nrequest=%s", shape, naturalText)

	result, err := c.translateAttempt(ctx, cfg, shape, naturalText, systemPrompt, userPrompt, 1)
	if err == nil {
		return result, nil
	}

	var parseErr *parseOutputError
	if !errors.As(err, &parseErr) {
		return types.TranslationResult{}, err
	}

	c.logger.Warn("llm parse failed, retry once",
		"shape", shape,
		"error", parseErr.cause,
	)
	retryUserPrompt := userPrompt + "\n\nYour previous output was invalid. Return valid JSON only and follow the schema strictly."

	result, retryErr := c.translateAttempt(ctx, cfg, shape, naturalText, systemPrompt, retryUserPrompt, 2)
	if retryErr == nil {
		c.logger.Info("llm parse retry succeeded", "shape", shape, "character_count", len(result.Characters))
		return result, nil
	}

	if errors.As(retryErr, &parseErr) {
		return types.TranslationResult{}, parseErr.cause
	}
	return types.TranslationResult{}, retryErr
}

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

	url := strings.TrimRight(cfg.LLM.BaseURL, "/") + "/chat/completions"
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

func parsePromptJSON(raw string) (types.TranslationResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		PositivePrompt   string `json:"positivePrompt"`
		NegativePrompt   string `json:"negativePrompt"`
		CharacterPrompts []struct {
			CharPositivePrompt  string `json:"charPositivePrompt"`
			CharUnconcentPrompt string `json:"charUnconcentPrompt"`
			Centers             struct {
				X any `json:"x"`
				Y any `json:"y"`
			} `json:"centers"`
		} `json:"characterPrompts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return types.TranslationResult{}, err
	}

	result := types.TranslationResult{
		PositivePrompt: strings.TrimSpace(parsed.PositivePrompt),
		NegativePrompt: strings.TrimSpace(parsed.NegativePrompt),
	}
	if result.PositivePrompt == "" {
		return types.TranslationResult{}, fmt.Errorf("missing positivePrompt")
	}
	if result.NegativePrompt == "" {
		return types.TranslationResult{}, fmt.Errorf("missing negativePrompt")
	}

	for idx, cp := range parsed.CharacterPrompts {
		pos := strings.TrimSpace(cp.CharPositivePrompt)
		neg := strings.TrimSpace(cp.CharUnconcentPrompt)
		if pos == "" {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].charPositivePrompt is required", idx)
		}

		row, err := parseGridRow(cp.Centers.X)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.x invalid: %w", idx, err)
		}
		col, err := parseGridCol(cp.Centers.Y)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.y invalid: %w", idx, err)
		}

		centerX, err := mapGridIndexToCoord(row)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.x invalid: %w", idx, err)
		}
		centerY, err := mapGridIndexToCoord(col)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.y invalid: %w", idx, err)
		}

		result.Characters = append(result.Characters, types.CharacterPrompt{
			PositivePrompt: pos,
			NegativePrompt: neg,
			CenterX:        centerX,
			CenterY:        centerY,
		})
	}

	return result, nil
}

func parseGridRow(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("must be integer")
		}
		return int(n), nil
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, fmt.Errorf("empty value")
		}
		parsed, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("must be integer string")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func parseGridCol(v any) (int, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("must be letter A-E")
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 1 {
		return 0, fmt.Errorf("must be single letter A-E")
	}
	r := s[0]
	if r < 'A' || r > 'E' {
		return 0, fmt.Errorf("must be in A-E")
	}
	return int(r-'A') + 1, nil
}

func mapGridIndexToCoord(idx int) (float64, error) {
	switch idx {
	case 1:
		return 0.1, nil
	case 2:
		return 0.3, nil
	case 3:
		return 0.5, nil
	case 4:
		return 0.7, nil
	case 5:
		return 0.9, nil
	default:
		return 0, fmt.Errorf("index out of range 1..5")
	}
}

func extractAssistantContent(respBody []byte) (string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", fmt.Errorf("空响应")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, nil
	}

	if _, err := parsePromptJSON(string(respBody)); err == nil {
		return string(respBody), nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseSSEContent(respBody)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, nil
		}
	}

	return "", fmt.Errorf("不支持的响应格式")
}

func parseSSEContent(respBody []byte) (string, error) {
	lines := bytes.Split(respBody, []byte("\n"))
	var builder strings.Builder
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		if content, ok := parseOpenAICompletionPayload(payload); ok {
			builder.WriteString(content)
			continue
		}

		if _, err := parsePromptJSON(string(payload)); err == nil {
			builder.Write(payload)
			continue
		}
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("SSE 数据中未找到可解析内容")
	}
	return builder.String(), nil
}

func parseOpenAICompletionPayload(payload []byte) (string, bool) {
	var out struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Delta struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
			Text json.RawMessage `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", false
	}
	if len(out.Choices) == 0 {
		return "", false
	}

	if content, ok := decodeContentField(out.Choices[0].Message.Content); ok {
		return content, true
	}
	if content, ok := decodeContentField(out.Choices[0].Delta.Content); ok {
		return content, true
	}
	if content, ok := decodeContentField(out.Choices[0].Text); ok {
		return content, true
	}
	return "", false
}

func decodeContentField(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), true
	}

	var asArray []map[string]any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		var parts []string
		for _, item := range asArray {
			if t, ok := item["text"].(string); ok && strings.TrimSpace(t) != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ""), true
		}
	}

	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if t, ok := asObject["text"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t), true
		}
	}

	return "", false
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
