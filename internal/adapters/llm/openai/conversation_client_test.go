package openai

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	conversationapp "grimoire/internal/app/conversation"
	"grimoire/internal/config"
	domainsession "grimoire/internal/domain/session"
)

func TestParseConversationOutputReplyJSON(t *testing.T) {
	output, err := parseConversationOutput(`{"reply":"好的，请再补充一下视角。","request":""}`)
	if err != nil {
		t.Fatalf("parse conversation output: %v", err)
	}
	if output.Reply != "好的，请再补充一下视角。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.CreateDrawingTask != nil {
		t.Fatalf("did not expect create drawing task: %#v", output.CreateDrawingTask)
	}
}

func TestParseConversationOutputRequestJSON(t *testing.T) {
	output, err := parseConversationOutput(`{"reply":"","request":"绘制月下城堡少女"}`)
	if err != nil {
		t.Fatalf("parse conversation output: %v", err)
	}
	if output.CreateDrawingTask == nil {
		t.Fatal("expected create drawing task")
	}
	if output.CreateDrawingTask.Request != "绘制月下城堡少女" {
		t.Fatalf("unexpected request: %q", output.CreateDrawingTask.Request)
	}
	if output.Reply != "" {
		t.Fatalf("expected empty reply, got %q", output.Reply)
	}
}

func TestParseConversationOutputRejectsInvalidShapes(t *testing.T) {
	for _, raw := range []string{
		`{"reply":"","request":""}`,
		`{"reply":"继续补充","request":"绘制月下城堡少女"}`,
		`not-json`,
	} {
		if _, err := parseConversationOutput(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestConversationSystemPromptRequiresJSONOutput(t *testing.T) {
	for _, expected := range []string{
		"你的输出必须始终是一个合法的 json 对象",
		`"reply": "自然中文回复，或空字符串"`,
		`"request": "最终用于绘图的中文 request，或空字符串"`,
		"`reply` 和 `request` 必须二选一",
	} {
		if !strings.Contains(conversationSystemPrompt, expected) {
			t.Fatalf("expected prompt to contain %q", expected)
		}
	}
}

func TestConverseSendsJSONOutputRequest(t *testing.T) {
	var requestBody map[string]any
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"继续说说你的构图偏好。","request":""}`)), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.Reply != "继续说说你的构图偏好。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.CreateDrawingTask != nil {
		t.Fatalf("did not expect create drawing task: %#v", output.CreateDrawingTask)
	}

	if _, ok := requestBody["tools"]; ok {
		t.Fatalf("did not expect tools in request body: %#v", requestBody["tools"])
	}
	if _, ok := requestBody["tool_choice"]; ok {
		t.Fatalf("did not expect tool_choice in request body: %#v", requestBody["tool_choice"])
	}
	if _, ok := requestBody["reasoning_effort"]; ok {
		t.Fatalf("did not expect reasoning_effort without explicit config: %#v", requestBody["reasoning_effort"])
	}
	responseFormat, ok := requestBody["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected response_format: %#v", requestBody["response_format"])
	}
	if responseFormat["type"] != "json_object" {
		t.Fatalf("unexpected response_format.type: %#v", responseFormat["type"])
	}
}

func TestConverseSendsReasoningEffortWhenConfigured(t *testing.T) {
	var requestBody map[string]any
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"继续说说你的构图偏好。","request":""}`)), nil
	})
	client.cfg.ReasoningEffort = " custom-effort "

	if _, err := client.Converse(context.Background(), newConversationInput(t)); err != nil {
		t.Fatalf("converse: %v", err)
	}
	if requestBody["reasoning_effort"] != "custom-effort" {
		t.Fatalf("unexpected reasoning_effort: %#v", requestBody["reasoning_effort"])
	}
}

func TestConverseParsesRequestJSONResponse(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"","request":"绘制月下城堡少女"}`)), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.CreateDrawingTask == nil {
		t.Fatal("expected create drawing task output")
	}
	if output.CreateDrawingTask.Request != "绘制月下城堡少女" {
		t.Fatalf("unexpected request: %q", output.CreateDrawingTask.Request)
	}
}

func TestConverseLogsJSONResponse(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	client := newTestConversationClient(t, logger, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"请再补充一下构图与视角。","request":""}`)), nil
	})

	if _, err := client.Converse(context.Background(), newConversationInput(t)); err != nil {
		t.Fatalf("converse: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"conversation llm request started",
		"session_id=session-1",
		"user_payload=",
		"conversation llm response parsed",
		"response_mode=json",
		"reply=请再补充一下构图与视角。",
		"raw_response=",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
	if strings.Contains(logOutput, "tool_name=") {
		t.Fatalf("did not expect tool_name in logs, got %s", logOutput)
	}
}

func TestConverseLogsRequestJSONResponse(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	client := newTestConversationClient(t, logger, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"","request":"绘制月下城堡少女"}`)), nil
	})

	if _, err := client.Converse(context.Background(), newConversationInput(t)); err != nil {
		t.Fatalf("converse: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"conversation llm response parsed",
		"response_mode=json",
		"request=绘制月下城堡少女",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
	if strings.Contains(logOutput, "tool_name=") {
		t.Fatalf("did not expect tool_name in logs, got %s", logOutput)
	}
}

func newTestConversationClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *ConversationClient {
	t.Helper()

	return &ConversationClient{
		cfg: config.LLM{
			BaseURL:    "https://api.openai.com/v1",
			APIKey:     "key",
			Model:      "gpt-4o-mini",
			TimeoutSec: 10,
		},
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
	}
}

func newConversationInput(t *testing.T) conversationapp.ConversationInput {
	t.Helper()

	message, err := domainsession.NewMessage("message-1", "session-1", domainsession.MessageRoleUser, "我想画一座城堡", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return conversationapp.ConversationInput{
		SessionID: "session-1",
		Messages:  []domainsession.Message{message},
	}
}
