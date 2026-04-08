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

func TestParseConversationOutputNaturalText(t *testing.T) {
	output, err := parseConversationOutput("好的，这是整理后的需求，请确认是否现在开始绘图。")
	if err != nil {
		t.Fatalf("parse conversation output: %v", err)
	}
	if output.Reply != "好的，这是整理后的需求，请确认是否现在开始绘图。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.CreateDrawingTask != nil {
		t.Fatalf("did not expect create drawing task: %#v", output.CreateDrawingTask)
	}
}

func TestConversationSystemPromptForcesToolCallOnExplicitStart(t *testing.T) {
	for _, expected := range []string{
		"如果用户这一轮已经是在明确要求“现在开始绘图”，你必须调用 `create_drawing_task` 工具。",
		"一旦你调用 `create_drawing_task` 工具，就不要再输出普通文字回复。",
		"如果你判断用户是在明确要求立即执行，你必须调用 `create_drawing_task` 工具，而不是返回普通文字回复。",
		"普通对话轮次时，你必须直接输出自然中文回复，不要输出 JSON",
		"如果确认无误，请告诉我‘现在开始绘图’；否则请继续补充。",
	} {
		if !strings.Contains(conversationSystemPrompt, expected) {
			t.Fatalf("expected prompt to contain %q", expected)
		}
	}
}

func TestConverseSendsMessageOnlyPayload(t *testing.T) {
	var requestBody map[string]any
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, "继续说说你的构图偏好。")), nil
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

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected user message: %#v", messages[1])
	}

	var userPayload map[string]any
	if err := json.Unmarshal([]byte(userMessage["content"].(string)), &userPayload); err != nil {
		t.Fatalf("unmarshal user content: %v", err)
	}
	if _, ok := userPayload["messages"].([]any); !ok {
		t.Fatalf("expected messages payload, got %#v", userPayload)
	}
	if _, ok := userPayload["summary"]; ok {
		t.Fatalf("did not expect summary payload, got %#v", userPayload["summary"])
	}
	if _, ok := userPayload["preference"]; ok {
		t.Fatalf("did not expect preference payload, got %#v", userPayload["preference"])
	}
}

func TestConverseParsesSSEContent(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{"role": "assistant"}),
			"",
			sseChunk(t, map[string]any{"content": "好的，"}),
			"",
			sseChunk(t, map[string]any{"content": "继续补充"}),
			"",
			sseChunk(t, map[string]any{"content": "一下背景细节。"}),
			"",
			"data: [DONE]",
		}, "\n")
		return newHTTPResponse(http.StatusOK, body), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.Reply != "好的，继续补充一下背景细节。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
}

func TestParseCreateDrawingTaskOutput(t *testing.T) {
	output, err := parseCreateDrawingTaskOutput(`{"request":"绘制月下城堡少女"}`)
	if err != nil {
		t.Fatalf("parse create drawing task output: %v", err)
	}
	if output.CreateDrawingTask == nil {
		t.Fatal("expected create drawing task output")
	}
	if output.CreateDrawingTask.Request != "绘制月下城堡少女" {
		t.Fatalf("unexpected request: %q", output.CreateDrawingTask.Request)
	}
	if output.Reply != "" {
		t.Fatalf("expected empty reply, got %q", output.Reply)
	}
}

func TestParseCreateDrawingTaskOutputRejectsMissingRequest(t *testing.T) {
	if _, err := parseCreateDrawingTaskOutput(`{}`); err == nil {
		t.Fatal("expected error")
	}
}

func TestConverseParsesToolCallResponse(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := mustJSON(t, map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"function": map[string]any{
									"name": createDrawingTaskToolName,
									"arguments": mustJSON(t, map[string]any{
										"request": "绘制月下城堡少女",
									}),
								},
							},
						},
					},
				},
			},
		})
		return newHTTPResponse(http.StatusOK, body), nil
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

func TestConverseParsesSSEToolCallResponse(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{
				"tool_calls": []any{
					map[string]any{
						"index": 0,
						"function": map[string]any{
							"name":      createDrawingTaskToolName,
							"arguments": "{",
						},
					},
				},
			}),
			"",
			sseChunk(t, map[string]any{
				"tool_calls": []any{
					map[string]any{
						"index": 0,
						"function": map[string]any{
							"arguments": `"request":"绘制月下城堡少女"}`,
						},
					},
				},
			}),
			"",
			"data: [DONE]",
		}, "\n")
		return newHTTPResponse(http.StatusOK, body), nil
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
		return newHTTPResponse(http.StatusOK, completionWithContent(t, "请再补充一下构图与视角。")), nil
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
	if strings.Contains(logOutput, "summary_after=") {
		t.Fatalf("did not expect summary logging, got %s", logOutput)
	}
}

func TestConverseLogsToolResponse(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	client := newTestConversationClient(t, logger, func(req *http.Request) (*http.Response, error) {
		body := mustJSON(t, map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"function": map[string]any{
									"name": createDrawingTaskToolName,
									"arguments": mustJSON(t, map[string]any{
										"request": "绘制月下城堡少女",
									}),
								},
							},
						},
					},
				},
			},
		})
		return newHTTPResponse(http.StatusOK, body), nil
	})

	if _, err := client.Converse(context.Background(), newConversationInput(t)); err != nil {
		t.Fatalf("converse: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"conversation llm response parsed",
		"response_mode=tool",
		"tool_name=create_drawing_task",
		"request=绘制月下城堡少女",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
	if strings.Contains(logOutput, "summary_after=") {
		t.Fatalf("did not expect summary logging, got %s", logOutput)
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
