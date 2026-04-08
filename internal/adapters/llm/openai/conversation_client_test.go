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
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

func TestParseConversationOutput(t *testing.T) {
	output, err := parseConversationOutput(`{"reply":"好的，我来继续细化。","summary":{"goal":"castle"}}`)
	if err != nil {
		t.Fatalf("parse conversation output: %v", err)
	}
	if output.Reply != "好的，我来继续细化。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.Summary.Content() != `{"goal":"castle"}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
	}
}

func TestParseConversationOutputRejectsInvalidSummary(t *testing.T) {
	if _, err := parseConversationOutput(`{"reply":"hi","summary":"not-json"}`); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseConversationOutputRejectsNonObjectSummary(t *testing.T) {
	if _, err := parseConversationOutput(`{"reply":"hi","summary":[]}`); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildConversationPayloadRejectsNonObjectSummary(t *testing.T) {
	input := newConversationInput(t)
	input.Summary = domainsession.NewSummary(`[]`)

	if _, err := buildConversationPayload(input); err == nil {
		t.Fatal("expected error")
	}
}

func TestConverseSendsStructuredPayload(t *testing.T) {
	var requestBody map[string]any
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"继续说说你的构图偏好。","summary":{"goal":"castle","mood":"quiet"}}`)), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.Reply != "继续说说你的构图偏好。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.Summary.Content() != `{"goal":"castle","mood":"quiet"}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
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
	if _, ok := userPayload["summary"].(map[string]any); !ok {
		t.Fatalf("expected json summary payload, got %#v", userPayload["summary"])
	}
	preference, ok := userPayload["preference"].(map[string]any)
	if !ok || preference["shape"] != string(domaindraw.ShapePortrait) {
		t.Fatalf("unexpected preference payload: %#v", userPayload["preference"])
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("unexpected tools payload: %#v", requestBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool payload: %#v", tools[0])
	}
	function, ok := tool["function"].(map[string]any)
	if !ok || function["name"] != createDrawingTaskToolName {
		t.Fatalf("unexpected tool function payload: %#v", tool["function"])
	}
}

func TestConverseParsesSSEContent(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{"role": "assistant"}),
			"",
			sseChunk(t, map[string]any{"content": ""}),
			"",
			sseChunk(t, map[string]any{"content": "{\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "reply":"好的，继续。",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "summary":{"goal":"castle"}` + "\n}"}),
			"",
			"data: [DONE]",
		}, "\n")
		return newHTTPResponse(http.StatusOK, body), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.Reply != "好的，继续。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.Summary.Content() != `{"goal":"castle"}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
	}
}

func TestConverseParsesRawSSEJSONFragments(t *testing.T) {
	client := newTestConversationClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			`data: {"reply":"好的，继续。",`,
			`data: "summary":{"goal":"castle"}}`,
			"data: [DONE]",
		}, "\n")
		return newHTTPResponse(http.StatusOK, body), nil
	})

	output, err := client.Converse(context.Background(), newConversationInput(t))
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if output.Reply != "好的，继续。" {
		t.Fatalf("unexpected reply: %q", output.Reply)
	}
	if output.Summary.Content() != `{"goal":"castle"}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
	}
}

func TestParseCreateDrawingTaskOutput(t *testing.T) {
	output, err := parseCreateDrawingTaskOutput(`{"request":"绘制月下城堡少女","summary":{"goal":"castle","ready":true}}`)
	if err != nil {
		t.Fatalf("parse create drawing task output: %v", err)
	}
	if output.CreateDrawingTask == nil {
		t.Fatal("expected create drawing task output")
	}
	if output.CreateDrawingTask.Request != "绘制月下城堡少女" {
		t.Fatalf("unexpected request: %q", output.CreateDrawingTask.Request)
	}
	if output.Summary.Content() != `{"goal":"castle","ready":true}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
	}
	if output.Reply != "" {
		t.Fatalf("expected empty reply, got %q", output.Reply)
	}
}

func TestParseCreateDrawingTaskOutputRejectsMissingRequest(t *testing.T) {
	if _, err := parseCreateDrawingTaskOutput(`{"summary":{"goal":"castle"}}`); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseCreateDrawingTaskOutputRejectsMissingSummary(t *testing.T) {
	if _, err := parseCreateDrawingTaskOutput(`{"request":"绘制月下城堡少女"}`); err == nil {
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
										"summary": map[string]any{
											"goal": "castle",
											"ready": true,
										},
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
	if output.Summary.Content() != `{"goal":"castle","ready":true}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
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
							"arguments": `"request":"绘制月下城堡少女",`,
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
							"arguments": `"summary":{"goal":"castle","ready":true}}`,
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
	if output.Summary.Content() != `{"goal":"castle","ready":true}` {
		t.Fatalf("unexpected summary: %q", output.Summary.Content())
	}
}

func TestConverseRejectsUnsupportedFormat(t *testing.T) {
	client := newTestConversationClient(t, slog.Default(), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, `{"unexpected":"payload"}`), nil
	})

	if _, err := client.Converse(context.Background(), newConversationInput(t)); err == nil {
		t.Fatal("expected error")
	}
}

func TestConverseLogsJSONResponse(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	client := newTestConversationClient(t, logger, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"reply":"开始绘图吧。","summary":{"goal":"castle","ready":true}}`)), nil
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
		"reply=开始绘图吧。",
		"raw_response=",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
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
										"summary": map[string]any{
											"goal": "castle",
										},
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
		"summary_after=\"{\\\"goal\\\":\\\"castle\\\"}\"",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
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

	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	message, err := domainsession.NewMessage("message-1", "session-1", domainsession.MessageRoleUser, "我想画一座城堡", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return conversationapp.ConversationInput{
		SessionID:  "session-1",
		Summary:    domainsession.NewSummary(`{"goal":"castle"}`),
		Messages:   []domainsession.Message{message},
		Preference: preference,
	}
}
