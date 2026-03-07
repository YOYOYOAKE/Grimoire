package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	"grimoire/internal/platform/httpclient"
)

type Client struct {
	cfg        config.Config
	httpClient *http.Client
	logger     *slog.Logger
}

func NewClient(cfg config.Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.LLM.TimeoutSec, cfg.LLM.Proxy, logger, "llm"),
		logger:     logger,
	}
}

func (c *Client) Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.LLM.TimeoutSec)*time.Second)
	defer cancel()

	body := map[string]any{
		"model": c.cfg.LLM.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("shape=%s\nrequest=%s", shape, strings.TrimSpace(prompt))},
		},
		"temperature": 0.2,
		"stream":      false,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return domaindraw.Translation{}, fmt.Errorf("marshal llm request: %w", err)
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(c.cfg.LLM.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return domaindraw.Translation{}, fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.LLM.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logFailure("llm request failed", shape, err, "", "")
		return domaindraw.Translation{}, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logFailure("read llm response failed", shape, err, "", "")
		return domaindraw.Translation{}, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.logFailure("llm returned non-200", shape, fmt.Errorf("status=%d", resp.StatusCode), string(respBody), "")
		return domaindraw.Translation{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, err := extractAssistantContent(respBody)
	if err != nil {
		c.logFailure("extract llm content failed", shape, err, string(respBody), "")
		return domaindraw.Translation{}, err
	}
	translation, err := parseTranslation(content)
	if err != nil {
		c.logFailure("parse llm content failed", shape, err, string(respBody), content)
		return domaindraw.Translation{}, err
	}
	if c.logger != nil {
		c.logger.Info("llm translated", "shape", shape, "positive_prompt", translation.PositivePrompt)
	}
	return translation, nil
}

const systemPrompt = `
You translate Chinese natural language image requests into NovelAI-friendly English tag prompts.

Output JSON only:
{
  "positivePrompt":"...",
  "negativePrompt":"..."
}
`

func parseTranslation(raw string) (domaindraw.Translation, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		PositivePrompt string `json:"positivePrompt"`
		NegativePrompt string `json:"negativePrompt"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return domaindraw.Translation{}, fmt.Errorf("parse llm json: %w", err)
	}
	translation := domaindraw.Translation{
		PositivePrompt: strings.TrimSpace(parsed.PositivePrompt),
		NegativePrompt: strings.TrimSpace(parsed.NegativePrompt),
	}
	if translation.PositivePrompt == "" {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing positivePrompt")
	}
	if translation.NegativePrompt == "" {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing negativePrompt")
	}
	return translation, nil
}

func extractAssistantContent(respBody []byte) (string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", fmt.Errorf("empty llm response")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, nil
	}

	if _, err := parseTranslation(string(respBody)); err == nil {
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
	return "", fmt.Errorf("unsupported llm response format")
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

		if _, err := parseTranslation(string(payload)); err == nil {
			builder.Write(payload)
		}
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("unsupported llm response format")
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
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return "", false
		}
		return asString, true
	}

	var asArray []map[string]any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		var parts []string
		for _, item := range asArray {
			if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ""), true
		}
	}

	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if text, ok := asObject["text"].(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text, true
			}
		}
	}

	return "", false
}

func (c *Client) logFailure(message string, shape domaindraw.Shape, err error, rawResponse string, assistantContent string) {
	if c.logger == nil {
		return
	}

	attrs := []any{
		"shape", shape,
		"error", err,
	}
	if strings.TrimSpace(rawResponse) != "" {
		attrs = append(attrs, "raw_response", truncate(rawResponse, 2000))
	}
	if strings.TrimSpace(assistantContent) != "" {
		attrs = append(attrs, "assistant_content", truncate(assistantContent, 2000))
	}
	c.logger.Error(message, attrs...)
}

func truncate(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit] + "..."
}
