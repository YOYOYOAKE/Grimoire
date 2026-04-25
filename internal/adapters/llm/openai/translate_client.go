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

type TranslateClient struct {
	cfg        config.LLM
	httpClient *http.Client
	logger     *slog.Logger
}

type TranslateFailoverClient struct {
	clients []translateProvider
	logger  *slog.Logger
}

type translateProvider interface {
	translate(ctx context.Context, prompt string, shape domaindraw.Shape) (translationResult, error)
	model() string
	baseURL() string
}

type translationResult struct {
	Translation  domaindraw.Translation
	ResponseMode string
}

type translateAttemptError struct {
	err              error
	rawResponse      string
	assistantContent string
}

func (e *translateAttemptError) Error() string {
	return e.err.Error()
}

func (e *translateAttemptError) Unwrap() error {
	return e.err
}

const attemptsPerLLM = 3

const llmResponseModeJSON = "json"

//go:embed translate_system_prompt.md
var translateSystemPromptFile string

var translateSystemPrompt = strings.TrimSpace(translateSystemPromptFile)

func NewTranslateClient(cfg config.LLM, logger *slog.Logger) *TranslateClient {
	return &TranslateClient{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.TimeoutSec, cfg.Proxy, logger, "llm"),
		logger:     logger,
	}
}

func NewTranslateFailoverClient(cfgs []config.LLM, logger *slog.Logger) *TranslateFailoverClient {
	clients := make([]translateProvider, 0, len(cfgs))
	for _, cfg := range cfgs {
		clients = append(clients, NewTranslateClient(cfg, logger))
	}
	return newTranslateFailoverClient(clients, logger)
}

func newTranslateFailoverClient(clients []translateProvider, logger *slog.Logger) *TranslateFailoverClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &TranslateFailoverClient{
		clients: clients,
		logger:  logger,
	}
}

func (c *TranslateClient) Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error) {
	logTranslateRequest(c.logger, c.cfg.BaseURL, c.cfg.Model, 1, shape)
	result, err := c.translate(ctx, prompt, shape)
	if err != nil {
		return domaindraw.Translation{}, err
	}
	logTranslateSuccess(c.logger, result.Translation)
	return result.Translation, nil
}

func (c *TranslateClient) translate(ctx context.Context, prompt string, shape domaindraw.Shape) (translationResult, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSec)*time.Second)
	defer cancel()

	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": translateSystemPrompt},
			{"role": "user", "content": fmt.Sprintf("request=%s", strings.TrimSpace(prompt))},
		},
		"temperature": 0.2,
		"stream":      false,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	addReasoningEffort(body, c.cfg.ReasoningEffort)

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
		statusErr := fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
		c.logFailure("llm returned non-200", shape, statusErr, string(respBody), "")
		return translationResult{}, withTranslateAttemptDetails(statusErr, string(respBody), "")
	}

	content, responseMode, err := extractAssistantContent(respBody)
	if err != nil {
		c.logFailure("extract llm json content failed", shape, err, string(respBody), "")
		return translationResult{}, withTranslateAttemptDetails(err, string(respBody), "")
	}
	translation, err := parseTranslation(content)
	if err != nil {
		c.logFailure("parse llm json content failed", shape, err, string(respBody), content)
		return translationResult{}, withTranslateAttemptDetails(err, string(respBody), content)
	}
	return translationResult{
		Translation:  translation,
		ResponseMode: responseMode,
	}, nil
}

func (c *TranslateClient) model() string {
	return c.cfg.Model
}

func (c *TranslateClient) baseURL() string {
	return c.cfg.BaseURL
}

func (c *TranslateFailoverClient) Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error) {
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

	promptRaw, usedLegacyPrompt, ok := translationField(parsed, "prompt", "positivePrompt")
	if !ok {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing prompt")
	}
	negativeRaw, usedLegacyNegative, ok := translationField(parsed, "negative_prompt", "negativePrompt")
	if !ok {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing negative_prompt")
	}

	prompt, err := parseTranslationStringField(promptRaw, "prompt")
	if err != nil {
		return domaindraw.Translation{}, err
	}
	negativePrompt, err := parseTranslationStringField(negativeRaw, "negative_prompt")
	if err != nil {
		return domaindraw.Translation{}, err
	}

	charactersRaw, ok := parsed["characters"]
	characters := []domaindraw.CharacterPrompt{}
	if ok {
		characters, err = parseCharacters(charactersRaw)
		if err != nil {
			return domaindraw.Translation{}, err
		}
	} else if !usedLegacyPrompt && !usedLegacyNegative {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing characters")
	}

	translation := domaindraw.Translation{
		Prompt:         strings.TrimSpace(prompt),
		NegativePrompt: strings.TrimSpace(negativePrompt),
		Characters:     characters,
	}
	if translation.Prompt == "" {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing prompt")
	}
	if translation.NegativePrompt == "" {
		return domaindraw.Translation{}, fmt.Errorf("llm response missing negative_prompt")
	}
	return translation, nil
}

func translationField(parsed map[string]json.RawMessage, preferred string, legacy string) (json.RawMessage, bool, bool) {
	if raw, ok := parsed[preferred]; ok {
		return raw, false, true
	}
	if raw, ok := parsed[legacy]; ok {
		return raw, true, true
	}
	return nil, false, false
}

func parseTranslationStringField(raw json.RawMessage, field string) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("llm response %s must be string: %w", field, err)
	}
	return value, nil
}

func parseCharacters(raw json.RawMessage) ([]domaindraw.CharacterPrompt, error) {
	var parsed []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("llm response characters must be array: %w", err)
	}

	characters := make([]domaindraw.CharacterPrompt, 0, len(parsed))
	for i, item := range parsed {
		promptRaw, ok := item["prompt"]
		if !ok {
			return nil, fmt.Errorf("llm response characters[%d] missing prompt", i)
		}
		negativeRaw, ok := item["negative_prompt"]
		if !ok {
			return nil, fmt.Errorf("llm response characters[%d] missing negative_prompt", i)
		}
		positionRaw, ok := item["position"]
		if !ok {
			return nil, fmt.Errorf("llm response characters[%d] missing position", i)
		}

		prompt, err := parseTranslationStringField(promptRaw, fmt.Sprintf("characters[%d].prompt", i))
		if err != nil {
			return nil, err
		}
		negativePrompt, err := parseTranslationStringField(negativeRaw, fmt.Sprintf("characters[%d].negative_prompt", i))
		if err != nil {
			return nil, err
		}
		position, err := parseTranslationStringField(positionRaw, fmt.Sprintf("characters[%d].position", i))
		if err != nil {
			return nil, err
		}
		position, err = normalizeCharacterPosition(position)
		if err != nil {
			return nil, fmt.Errorf("llm response characters[%d] invalid position: %w", i, err)
		}

		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			return nil, fmt.Errorf("llm response characters[%d] missing prompt", i)
		}
		negativePrompt = strings.TrimSpace(negativePrompt)
		if negativePrompt == "" {
			return nil, fmt.Errorf("llm response characters[%d] missing negative_prompt", i)
		}

		characters = append(characters, domaindraw.CharacterPrompt{
			Prompt:         prompt,
			NegativePrompt: negativePrompt,
			Position:       position,
		})
	}

	return characters, nil
}

func normalizeCharacterPosition(raw string) (string, error) {
	position := strings.ToUpper(strings.TrimSpace(raw))
	if len(position) != 2 {
		return "", fmt.Errorf("position must be one of A1-E5")
	}

	column := position[0]
	row := position[1]
	if column < 'A' || column > 'E' || row < '1' || row > '5' {
		return "", fmt.Errorf("position must be one of A1-E5")
	}
	return position, nil
}

func extractAssistantContent(respBody []byte) (string, string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", "", fmt.Errorf("empty llm response")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, llmResponseModeJSON, nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseSSEContent(respBody)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, llmResponseModeJSON, nil
		}
	}
	return "", "", fmt.Errorf("unsupported llm response format")
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

func (c *TranslateClient) logFailure(message string, shape domaindraw.Shape, err error, rawResponse string, assistantContent string) {
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
		attrs = append(attrs, "raw_response", rawResponse)
	}
	if strings.TrimSpace(assistantContent) != "" {
		attrs = append(attrs, "assistant_content", assistantContent)
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
		"prompt", translation.Prompt,
		"negative_prompt", translation.NegativePrompt,
		"characters", formatCharactersForLog(translation.Characters),
	)
}

func formatCharactersForLog(characters []domaindraw.CharacterPrompt) string {
	if len(characters) == 0 {
		return "[]"
	}

	logCharacters := make([]map[string]string, 0, len(characters))
	for _, character := range characters {
		logCharacters = append(logCharacters, map[string]string{
			"prompt":          character.Prompt,
			"negative_prompt": character.NegativePrompt,
			"position":        character.Position,
		})
	}

	data, err := json.Marshal(logCharacters)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func logTranslateAttemptFailure(logger *slog.Logger, shape domaindraw.Shape, llmIndex int, model string, baseURL string, attempt int, err error) {
	if logger == nil {
		return
	}

	attrs := []any{
		"shape", shape,
		"llm_index", llmIndex,
		"model", model,
		"base_url_host", baseURLHost(baseURL),
		"attempt", attempt,
		"error", err,
	}
	var translateErr *translateAttemptError
	if errors.As(err, &translateErr) {
		if strings.TrimSpace(translateErr.rawResponse) != "" {
			attrs = append(attrs, "raw_response", translateErr.rawResponse)
		}
		if strings.TrimSpace(translateErr.assistantContent) != "" {
			attrs = append(attrs, "assistant_content", translateErr.assistantContent)
		}
	}
	logger.Warn("llm translate attempt failed", attrs...)
}

func withTranslateAttemptDetails(err error, rawResponse string, assistantContent string) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(rawResponse) == "" && strings.TrimSpace(assistantContent) == "" {
		return err
	}
	return &translateAttemptError{
		err:              err,
		rawResponse:      rawResponse,
		assistantContent: assistantContent,
	}
}
