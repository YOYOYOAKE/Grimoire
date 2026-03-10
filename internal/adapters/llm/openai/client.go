package openai

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	"grimoire/internal/platform/httpclient"
)

type Client struct {
	cfg        config.LLM
	httpClient *http.Client
	logger     *slog.Logger
}

type FailoverClient struct {
	clients []translateClient
	logger  *slog.Logger
}

type translateClient interface {
	translate(ctx context.Context, prompt string, shape domaindraw.Shape) (translationResult, error)
	model() string
	baseURL() string
}

type translationResult struct {
	Translation  domaindraw.Translation
	ResponseMode string
}

const translatePromptToolName = "translate_prompt"
const attemptsPerLLM = 3

const (
	llmResponseModeTool      = "tool"
	llmResponseModePlaintext = "plaintext"
)

//go:embed system_prompt.md
var systemPromptFile string

var systemPrompt = strings.TrimSpace(systemPromptFile)

func NewClient(cfg config.LLM, logger *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.TimeoutSec, cfg.Proxy, logger, "llm"),
		logger:     logger,
	}
}

func NewFailoverClient(cfgs []config.LLM, logger *slog.Logger) *FailoverClient {
	clients := make([]translateClient, 0, len(cfgs))
	for _, cfg := range cfgs {
		clients = append(clients, NewClient(cfg, logger))
	}
	return newFailoverClient(clients, logger)
}

func newFailoverClient(clients []translateClient, logger *slog.Logger) *FailoverClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &FailoverClient{
		clients: clients,
		logger:  logger,
	}
}

func (c *Client) Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error) {
	logTranslateRequest(c.logger, c.cfg.BaseURL, c.cfg.Model, 1, shape)
	result, err := c.translate(ctx, prompt, shape)
	if err != nil {
		return domaindraw.Translation{}, err
	}
	logTranslateSuccess(c.logger, result.Translation)
	return result.Translation, nil
}

func (c *Client) translate(ctx context.Context, prompt string, shape domaindraw.Shape) (translationResult, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSec)*time.Second)
	defer cancel()

	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("shape=%s\nrequest=%s", shape, strings.TrimSpace(prompt))},
		},
		"temperature": 0.2,
		"stream":      false,
		"tools":       []any{translatePromptTool()},
		"tool_choice": map[string]any{
			"type": "function",
			"function": map[string]string{
				"name": translatePromptToolName,
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return translationResult{}, fmt.Errorf("marshal llm request: %w", err)
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return translationResult{}, fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logFailure("llm request failed", shape, err, "", "")
		return translationResult{}, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logFailure("read llm response failed", shape, err, "", "")
		return translationResult{}, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.logFailure("llm returned non-200", shape, fmt.Errorf("status=%d", resp.StatusCode), string(respBody), "")
		return translationResult{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, responseMode, err := extractAssistantContent(respBody)
	if err != nil {
		c.logFailure("extract llm content failed", shape, err, string(respBody), "")
		return translationResult{}, err
	}
	translation, err := parseTranslation(content)
	if err != nil {
		c.logFailure("parse llm content failed", shape, err, string(respBody), content)
		return translationResult{}, err
	}
	return translationResult{
		Translation:  translation,
		ResponseMode: responseMode,
	}, nil
}

func (c *Client) model() string {
	return c.cfg.Model
}

func (c *Client) baseURL() string {
	return c.cfg.BaseURL
}

func (c *FailoverClient) Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error) {
	if len(c.clients) == 0 {
		return domaindraw.Translation{}, fmt.Errorf("no llm providers configured")
	}

	totalAttempts := 0
	for llmIndex, client := range c.clients {
		model := client.model()
		baseURL := client.baseURL()
		for attempt := 1; attempt <= attemptsPerLLM; attempt++ {
			if err := ctx.Err(); err != nil {
				return domaindraw.Translation{}, err
			}

			totalAttempts++
			logTranslateRequest(c.logger, baseURL, model, attempt, shape)
			result, err := client.translate(ctx, prompt, shape)
			if err == nil {
				logTranslateSuccess(c.logger, result.Translation)
				return result.Translation, nil
			}

			logTranslateAttemptFailure(c.logger, shape, llmIndex, model, baseURL, attempt, err)

			if parentErr := ctx.Err(); parentErr != nil {
				return domaindraw.Translation{}, parentErr
			}
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return domaindraw.Translation{}, ctx.Err()
			}
		}
	}

	return domaindraw.Translation{}, fmt.Errorf("all llm providers failed after %d attempts", totalAttempts)
}

func translatePromptTool() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        translatePromptToolName,
			"description": "Return the positivePrompt and negativePrompt fields for image generation.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"positivePrompt": map[string]string{
						"type":        "string",
						"description": "NovelAI-friendly English positive prompt tags.",
					},
					"negativePrompt": map[string]string{
						"type":        "string",
						"description": "NovelAI-friendly English negative prompt tags. Use an empty string if none.",
					},
				},
				"required":             []string{"positivePrompt", "negativePrompt"},
				"additionalProperties": false,
			},
		},
	}
}

func parseTranslation(raw string) (domaindraw.Translation, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return domaindraw.Translation{}, fmt.Errorf("parse llm json: %w", err)
	}

	positiveRaw, ok := parsed["positivePrompt"]
	if !ok {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing positivePrompt")
	}
	negativeRaw, ok := parsed["negativePrompt"]
	if !ok {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing negativePrompt")
	}

	var positivePrompt string
	if err := json.Unmarshal(positiveRaw, &positivePrompt); err != nil {
		return domaindraw.Translation{}, fmt.Errorf("llm response positivePrompt must be string: %w", err)
	}
	var negativePrompt string
	if err := json.Unmarshal(negativeRaw, &negativePrompt); err != nil {
		return domaindraw.Translation{}, fmt.Errorf("llm response negativePrompt must be string: %w", err)
	}

	translation := domaindraw.Translation{
		PositivePrompt: strings.TrimSpace(positivePrompt),
		NegativePrompt: strings.TrimSpace(negativePrompt),
	}
	if translation.PositivePrompt == "" {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing positivePrompt")
	}
	return translation, nil
}

func extractAssistantContent(respBody []byte) (string, string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", "", fmt.Errorf("empty llm response")
	}

	if arguments, found, err := parseToolCallArguments(respBody); err != nil {
		return "", "", err
	} else if found {
		return arguments, llmResponseModeTool, nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		if arguments, found, err := parseSSEToolCallArguments(respBody); err != nil {
			return "", "", err
		} else if found {
			return arguments, llmResponseModeTool, nil
		}
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, llmResponseModePlaintext, nil
	}

	if _, err := parseTranslation(string(respBody)); err == nil {
		return string(respBody), llmResponseModePlaintext, nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseSSEContent(respBody)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, llmResponseModePlaintext, nil
		}
	}
	return "", "", fmt.Errorf("unsupported llm response format")
}

func parseToolCallArguments(payload []byte) (string, bool, error) {
	var out struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", false, nil
	}
	if len(out.Choices) == 0 {
		return "", false, nil
	}

	for _, toolCall := range out.Choices[0].Message.ToolCalls {
		if strings.TrimSpace(toolCall.Function.Name) != translatePromptToolName {
			continue
		}
		arguments, err := decodeToolArgumentString(toolCall.Function.Arguments)
		if err != nil {
			return "", true, err
		}
		return arguments, true, nil
	}
	return "", false, nil
}

func parseSSEToolCallArguments(respBody []byte) (string, bool, error) {
	type collectedToolCall struct {
		name      string
		arguments strings.Builder
	}

	lines := bytes.Split(respBody, []byte("\n"))
	collected := make(map[int]*collectedToolCall)
	order := make([]int, 0)

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		var out struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Index    int `json:"index"`
						Function struct {
							Name      string          `json:"name"`
							Arguments json.RawMessage `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &out); err != nil {
			continue
		}
		if len(out.Choices) == 0 {
			continue
		}

		for _, toolCall := range out.Choices[0].Delta.ToolCalls {
			call, ok := collected[toolCall.Index]
			if !ok {
				call = &collectedToolCall{}
				collected[toolCall.Index] = call
				order = append(order, toolCall.Index)
			}
			if name := strings.TrimSpace(toolCall.Function.Name); name != "" {
				call.name = name
			}
			arguments, err := decodeToolArgumentString(toolCall.Function.Arguments)
			if err != nil {
				return "", true, err
			}
			call.arguments.WriteString(arguments)
		}
	}

	for _, index := range order {
		call := collected[index]
		if call == nil || call.name != translatePromptToolName {
			continue
		}
		return call.arguments.String(), true, nil
	}
	return "", false, nil
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

func decodeToolArgumentString(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", nil
	}

	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("tool arguments must be string: %w", err)
	}
	return out, nil
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

func truncate(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit] + "..."
}

func baseURLHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return strings.TrimSpace(raw)
	}
	return parsed.Host
}

func logTranslateRequest(logger *slog.Logger, baseURL string, model string, attempt int, shape domaindraw.Shape) {
	if logger == nil {
		return
	}

	logger.Info(
		"llm request started",
		"base_url", strings.TrimSpace(baseURL),
		"model", model,
		"attempt", attempt,
		"shape", shape,
	)
}

func logTranslateSuccess(logger *slog.Logger, translation domaindraw.Translation) {
	if logger == nil {
		return
	}

	logger.Info(
		"llm translated",
		"positive_prompt", translation.PositivePrompt,
		"negative_prompt", translation.NegativePrompt,
	)
}

func logTranslateAttemptFailure(logger *slog.Logger, shape domaindraw.Shape, llmIndex int, model string, baseURL string, attempt int, err error) {
	if logger == nil {
		return
	}

	logger.Warn(
		"llm translate attempt failed",
		"shape", shape,
		"llm_index", llmIndex,
		"model", model,
		"base_url_host", baseURLHost(baseURL),
		"attempt", attempt,
		"error", err,
	)
}
