package openai

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	requestapp "grimoire/internal/app/request"
	"grimoire/internal/config"
	"grimoire/internal/platform/httpclient"
)

//go:embed request_system_prompt.md
var requestSystemPromptFile string

var requestSystemPrompt = strings.TrimSpace(requestSystemPromptFile)

type RequestClient struct {
	cfg        config.LLM
	httpClient *http.Client
	logger     *slog.Logger
}

type requestPayload struct {
	Summary    json.RawMessage               `json:"summary"`
	Messages   []conversationMessagePayload  `json:"messages"`
	Preference conversationPreferencePayload `json:"preference"`
}

func NewRequestClient(cfg config.LLM, logger *slog.Logger) *RequestClient {
	return &RequestClient{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.TimeoutSec, cfg.Proxy, logger, "llm"),
		logger:     logger,
	}
}

func (c *RequestClient) Generate(ctx context.Context, input requestapp.GenerateInput) (string, error) {
	payloadInput, err := buildRequestPayload(input)
	if err != nil {
		return "", err
	}

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSec)*time.Second)
	defer cancel()

	userContent, err := json.Marshal(payloadInput)
	if err != nil {
		return "", fmt.Errorf("marshal request payload: %w", err)
	}

	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": requestSystemPrompt},
			{"role": "user", "content": string(userContent)},
		},
		"temperature": 0.2,
		"stream":      false,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal llm request: %w", err)
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logFailure("request generation failed", err, "", "")
		return "", fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logFailure("read request response failed", err, "", "")
		return "", fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.logFailure("request generation returned non-200", fmt.Errorf("status=%d", resp.StatusCode), string(respBody), "")
		return "", fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, err := extractRequestContent(respBody)
	if err != nil {
		c.logFailure("extract request content failed", err, string(respBody), "")
		return "", err
	}

	request, err := parseRequestOutput(content)
	if err != nil {
		c.logFailure("parse request content failed", err, string(respBody), content)
		return "", err
	}
	return request, nil
}

func buildRequestPayload(input requestapp.GenerateInput) (requestPayload, error) {
	if err := input.Preference.Validate(); err != nil {
		return requestPayload{}, err
	}

	summaryRaw := []byte(input.Summary.Content())
	if !json.Valid(summaryRaw) || !isJSONObject(summaryRaw) {
		return requestPayload{}, fmt.Errorf("request summary must be json object")
	}

	messages := make([]conversationMessagePayload, 0, len(input.Messages))
	for _, message := range input.Messages {
		if err := message.Validate(); err != nil {
			return requestPayload{}, err
		}
		messages = append(messages, conversationMessagePayload{
			Role:    string(message.Role),
			Content: message.Content,
		})
	}

	return requestPayload{
		Summary:  json.RawMessage(summaryRaw),
		Messages: messages,
		Preference: conversationPreferencePayload{
			Shape:   string(input.Preference.Shape),
			Artists: input.Preference.Artists,
		},
	}, nil
}

func parseRequestOutput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		Request string `json:"request"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", fmt.Errorf("parse request json: %w", err)
	}

	request := strings.TrimSpace(parsed.Request)
	if request == "" {
		return "", fmt.Errorf("request response missing request")
	}
	return request, nil
}

func extractRequestContent(respBody []byte) (string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", fmt.Errorf("empty llm response")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, nil
	}

	if json.Valid(respBody) {
		return string(respBody), nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseRequestSSEContent(respBody)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, nil
		}
	}

	return "", fmt.Errorf("unsupported llm response format")
}

func parseRequestSSEContent(respBody []byte) (string, error) {
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
		if isOpenAIEnvelope(payload) {
			continue
		}
		if !shouldAppendRawRequestPayload(payload) {
			continue
		}

		builder.Write(payload)
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("unsupported llm response format")
	}
	return builder.String(), nil
}

func shouldAppendRawRequestPayload(payload []byte) bool {
	if !json.Valid(payload) {
		return true
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return false
	}
	_, hasRequest := parsed["request"]
	return hasRequest
}

func (c *RequestClient) logFailure(message string, err error, rawResponse string, assistantContent string) {
	if c.logger == nil {
		return
	}

	attrs := []any{
		"model", c.cfg.Model,
		"base_url_host", baseURLHost(c.cfg.BaseURL),
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
