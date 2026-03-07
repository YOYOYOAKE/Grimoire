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
		return domaindraw.Translation{}, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return domaindraw.Translation{}, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domaindraw.Translation{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, err := extractAssistantContent(respBody)
	if err != nil {
		return domaindraw.Translation{}, err
	}
	translation, err := parseTranslation(content)
	if err != nil {
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
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &payload); err == nil && len(payload.Choices) > 0 {
		if content := strings.TrimSpace(payload.Choices[0].Message.Content); content != "" {
			return content, nil
		}
	}
	return "", fmt.Errorf("unsupported llm response format")
}

func truncate(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit] + "..."
}
