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

	"grimoire/internal/app/conversation"
	"grimoire/internal/config"
	"grimoire/internal/platform/httpclient"
)

//go:embed conversation_system_prompt.md
var conversationSystemPromptFile string

var conversationSystemPrompt = strings.TrimSpace(conversationSystemPromptFile)

type ConversationClient struct {
	cfg        config.LLM
	httpClient *http.Client
	logger     *slog.Logger
}

type conversationRequestPayload struct {
	Messages []conversationMessagePayload `json:"messages"`
}

type conversationMessagePayload struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const conversationResponseModeJSON = "json"

func NewConversationClient(cfg config.LLM, logger *slog.Logger) *ConversationClient {
	return &ConversationClient{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.TimeoutSec, cfg.Proxy, logger, "llm"),
		logger:     logger,
	}
}

func (c *ConversationClient) Converse(ctx context.Context, input conversation.ConversationInput) (conversation.ConversationOutput, error) {
	requestPayload, err := buildConversationPayload(input)
	if err != nil {
		return conversation.ConversationOutput{}, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.TimeoutSec)*time.Second)
	defer cancel()

	userContent, err := json.Marshal(requestPayload)
	if err != nil {
		return conversation.ConversationOutput{}, fmt.Errorf("marshal conversation payload: %w", err)
	}
	c.logRequest(input, string(userContent))

	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": conversationSystemPrompt},
			{"role": "user", "content": string(userContent)},
		},
		"temperature": 0.2,
		"stream":      false,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return conversation.ConversationOutput{}, fmt.Errorf("marshal llm request: %w", err)
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return conversation.ConversationOutput{}, fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logFailure("conversation request failed", err, "", "")
		return conversation.ConversationOutput{}, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logFailure("read conversation response failed", err, "", "")
		return conversation.ConversationOutput{}, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		c.logFailure("conversation returned non-200", fmt.Errorf("status=%d", resp.StatusCode), string(respBody), "")
		return conversation.ConversationOutput{}, fmt.Errorf("llm status=%d body=%s", resp.StatusCode, truncate(string(respBody), 400))
	}

	content, responseMode, err := extractConversationContent(respBody)
	if err != nil {
		c.logFailure("extract conversation json content failed", err, string(respBody), "")
		return conversation.ConversationOutput{}, err
	}

	output, err := parseConversationOutput(content)
	if err != nil {
		c.logFailure("parse conversation json content failed", err, string(respBody), content)
		return conversation.ConversationOutput{}, err
	}
	c.logSuccess(input, string(userContent), string(respBody), responseMode, output)
	return output, nil
}

func buildConversationPayload(input conversation.ConversationInput) (conversationRequestPayload, error) {
	messages := make([]conversationMessagePayload, 0, len(input.Messages))
	for _, message := range input.Messages {
		if err := message.Validate(); err != nil {
			return conversationRequestPayload{}, err
		}
		messages = append(messages, conversationMessagePayload{
			Role:    string(message.Role),
			Content: message.Content,
		})
	}

	return conversationRequestPayload{
		Messages: messages,
	}, nil
}

func parseConversationOutput(raw string) (conversation.ConversationOutput, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		Reply   string `json:"reply"`
		Request string `json:"request"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return conversation.ConversationOutput{}, fmt.Errorf("parse conversation json: %w", err)
	}

	reply := strings.TrimSpace(parsed.Reply)
	request := strings.TrimSpace(parsed.Request)
	switch {
	case reply == "" && request == "":
		return conversation.ConversationOutput{}, fmt.Errorf("conversation response missing reply or request")
	case reply != "" && request != "":
		return conversation.ConversationOutput{}, fmt.Errorf("conversation response reply and request are mutually exclusive")
	case reply != "":
		return conversation.ConversationOutput{Reply: reply}, nil
	default:
		return conversation.ConversationOutput{
			CreateDrawingTask: &conversation.CreateDrawingTask{Request: request},
		}, nil
	}
}

func extractConversationContent(respBody []byte) (string, string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", "", fmt.Errorf("empty llm response")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, conversationResponseModeJSON, nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseConversationSSEContent(respBody)
		if err != nil {
			return "", "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, conversationResponseModeJSON, nil
		}
	}

	return "", "", fmt.Errorf("unsupported llm response format")
}

func parseConversationSSEContent(respBody []byte) (string, error) {
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

func (c *ConversationClient) logFailure(message string, err error, rawResponse string, assistantContent string) {
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

func (c *ConversationClient) logRequest(input conversation.ConversationInput, userPayload string) {
	if c.logger == nil {
		return
	}

	c.logger.Info(
		"conversation llm request started",
		"model", c.cfg.Model,
		"base_url_host", baseURLHost(c.cfg.BaseURL),
		"session_id", strings.TrimSpace(input.SessionID),
		"recent_message_count", len(input.Messages),
		"messages", input.Messages,
		"user_payload", userPayload,
	)
}

func (c *ConversationClient) logSuccess(
	input conversation.ConversationInput,
	userPayload string,
	rawResponse string,
	responseMode string,
	output conversation.ConversationOutput,
) {
	if c.logger == nil {
		return
	}

	attrs := []any{
		"model", c.cfg.Model,
		"base_url_host", baseURLHost(c.cfg.BaseURL),
		"session_id", strings.TrimSpace(input.SessionID),
		"recent_message_count", len(input.Messages),
		"messages", input.Messages,
		"user_payload", userPayload,
		"raw_response", rawResponse,
		"response_mode", responseMode,
		"reply", output.Reply,
	}
	if output.CreateDrawingTask != nil {
		attrs = append(attrs, "request", output.CreateDrawingTask.Request)
	}

	c.logger.Info("conversation llm response parsed", attrs...)
}
