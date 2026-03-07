package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"grimoire/internal/config"
	"grimoire/internal/domain/draw"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestParseTranslation(t *testing.T) {
	translation, err := parseTranslation(`{"positivePrompt":"moonlit girl","negativePrompt":""}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
	if translation.NegativePrompt != "" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestParseTranslationRequiresNegativePrompt(t *testing.T) {
	_, err := parseTranslation(`{"positivePrompt":"moonlit girl"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing negativePrompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslateSendsToolChoiceRequest(t *testing.T) {
	var requestBody map[string]any
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithToolCall(t, `{"positivePrompt":"moonlit girl","negativePrompt":""}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}

	if _, ok := requestBody["response_format"]; ok {
		t.Fatal("response_format should not be sent")
	}

	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool, got %#v", requestBody["tools"])
	}

	toolChoice, ok := requestBody["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool_choice: %#v", requestBody["tool_choice"])
	}
	function, ok := toolChoice["function"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool_choice.function: %#v", toolChoice["function"])
	}
	if function["name"] != translatePromptToolName {
		t.Fatalf("unexpected tool choice name: %#v", function["name"])
	}
}

func TestTranslateParsesToolCallResponse(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithToolCall(t, `{"positivePrompt":"moonlit girl","negativePrompt":"blurry"}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateParsesSSEToolCallChunks(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{
				"tool_calls": []any{
					map[string]any{
						"index": 0,
						"function": map[string]any{
							"name":      translatePromptToolName,
							"arguments": "",
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
							"arguments": `{"positivePrompt":"moonlit girl",`,
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
							"arguments": `"negativePrompt":"blurry"`,
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
							"arguments": "}",
						},
					},
				},
			}),
			"",
			"data: [DONE]",
		}, "\n")

		return newHTTPResponse(http.StatusOK, body), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateFallsBackToAssistantJSON(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"positivePrompt":"moonlit girl","negativePrompt":""}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
	if translation.NegativePrompt != "" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateLogsResponseModeOnSuccess(t *testing.T) {
	testCases := []struct {
		name         string
		responseBody string
		expectedMode string
	}{
		{
			name:         "tool",
			responseBody: completionWithToolCall(t, `{"positivePrompt":"moonlit girl","negativePrompt":""}`),
			expectedMode: llmResponseModeTool,
		},
		{
			name:         "plaintext",
			responseBody: completionWithContent(t, `{"positivePrompt":"moonlit girl","negativePrompt":""}`),
			expectedMode: llmResponseModePlaintext,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logBuffer := &bytes.Buffer{}
			client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
				return newHTTPResponse(http.StatusOK, tc.responseBody), nil
			})

			_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
			if err != nil {
				t.Fatalf("translate: %v", err)
			}

			logOutput := logBuffer.String()
			if !strings.Contains(logOutput, "llm translated") {
				t.Fatalf("expected success log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "response_mode="+tc.expectedMode) {
				t.Fatalf("expected response mode %q, got %s", tc.expectedMode, logOutput)
			}
		})
	}
}

func TestTranslateParsesSSEAssistantContentFallback(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{"content": ""}),
			"",
			sseChunk(t, map[string]any{"content": "{\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "positivePrompt":"moonlit girl",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "negativePrompt":"blurry"` + "\n}"}),
			"",
			"data: [DONE]",
		}, "\n")

		return newHTTPResponse(http.StatusOK, body), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateDoesNotFallbackWhenToolArgumentsInvalid(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		payload := map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"function": map[string]any{
									"name":      translatePromptToolName,
									"arguments": "{",
								},
							},
						},
						"content": `{"positivePrompt":"fallback","negativePrompt":"fallback"}`,
					},
				},
			},
		}
		return newHTTPResponse(http.StatusOK, mustJSON(t, payload)), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse llm json") {
		t.Fatalf("expected parse llm json error, got %v", err)
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "parse llm content failed") {
		t.Fatalf("expected parse failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `assistant_content={`) {
		t.Fatalf("expected assistant_content log, got %s", logOutput)
	}
}

func TestTranslateLogsRawResponseOnUnsupportedFormat(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, `{"unexpected":"payload"}`), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "extract llm content failed") {
		t.Fatalf("expected failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `raw_response="{\"unexpected\":\"payload\"}"`) {
		t.Fatalf("expected raw response log, got %s", logOutput)
	}
}

func newTestClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *Client {
	t.Helper()

	return &Client{
		cfg: config.Config{
			LLM: config.LLM{
				BaseURL:    "https://api.openai.com/v1",
				APIKey:     "key",
				Model:      "gpt-4o-mini",
				TimeoutSec: 10,
			},
		},
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
	}
}

func completionWithToolCall(t *testing.T, arguments string) string {
	t.Helper()

	return mustJSON(t, map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"tool_calls": []any{
						map[string]any{
							"function": map[string]any{
								"name":      translatePromptToolName,
								"arguments": arguments,
							},
						},
					},
				},
			},
		},
	})
}

func completionWithContent(t *testing.T, content string) string {
	t.Helper()

	return mustJSON(t, map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": content,
				},
			},
		},
	})
}

func sseChunk(t *testing.T, delta map[string]any) string {
	t.Helper()

	return "data: " + mustJSON(t, map[string]any{
		"id": "1",
		"choices": []any{
			map[string]any{
				"delta": delta,
			},
		},
	})
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(payload)
}
