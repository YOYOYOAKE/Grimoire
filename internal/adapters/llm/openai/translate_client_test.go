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
	translation, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry, lowres" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 0 {
		t.Fatalf("expected no characters, got %#v", translation.Characters)
	}
}

func TestParseTranslationRequiresCharacters(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationSupportsLegacyFieldNames(t *testing.T) {
	translation, err := parseTranslation(`{"positivePrompt":"moonlit girl","negativePrompt":"blurry"}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 0 {
		t.Fatalf("expected empty legacy characters, got %#v", translation.Characters)
	}
}

func TestParseTranslationRejectsInvalidCharacterPosition(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl","negative_prompt":"bad hands","position":"Z9"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid position") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationRejectsEmptyNegativePrompt(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"","characters":[]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing negative_prompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationRejectsEmptyCharacterNegativePrompt(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl","negative_prompt":"","position":"C3"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "characters[0] missing negative_prompt") {
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

		return newHTTPResponse(http.StatusOK, completionWithToolCall(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
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

	if !ok || len(tools) != 1 {
		t.Fatalf("expected one tool, got %#v", requestBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool: %#v", tools[0])
	}
	toolDef, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected function tool: %#v", tool["function"])
	}
	parameters, ok := toolDef["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected parameters: %#v", toolDef["parameters"])
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected properties: %#v", parameters["properties"])
	}
	for _, field := range []string{"prompt", "negative_prompt", "characters"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected %q in tool schema, got %#v", field, properties)
		}
	}
}

func TestTranslateParsesToolCallResponse(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithToolCall(t, `{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl, long hair","negative_prompt":"bad hands","position":"C3"}]}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 1 || translation.Characters[0].Position != "C3" {
		t.Fatalf("unexpected characters: %#v", translation.Characters)
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
							"arguments": `{"prompt":"moonlit girl",`,
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
							"arguments": `"negative_prompt":"blurry","characters":[]`,
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
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateFallsBackToAssistantJSON(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry, lowres" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateLogsRequestAndTranslatedPrompts(t *testing.T) {
	testCases := []struct {
		name         string
		responseBody string
		expectParts  []string
	}{
		{
			name:         "tool",
			responseBody: completionWithToolCall(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[{"prompt":"girl, long hair","negative_prompt":"bad hands","position":"C3"}]}`),
			expectParts:  []string{"negative_prompt=\"blurry, lowres\"", "characters=", "girl, long hair", "bad hands", "C3"},
		},
		{
			name:         "plaintext",
			responseBody: completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`),
			expectParts:  []string{"negative_prompt=\"blurry, lowres\"", "characters=[]"},
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
			if !strings.Contains(logOutput, "llm request started") {
				t.Fatalf("expected request log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "base_url=https://api.openai.com/v1") {
				t.Fatalf("expected base_url in request log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "model=gpt-4o-mini") {
				t.Fatalf("expected model in request log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "attempt=1") {
				t.Fatalf("expected attempt in request log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "llm translated") {
				t.Fatalf("expected success log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "prompt=\"moonlit girl\"") {
				t.Fatalf("expected prompt in success log, got %s", logOutput)
			}
			if !strings.Contains(logOutput, "negative_prompt=\"blurry, lowres\"") {
				t.Fatalf("expected negative prompt in success log, got %s", logOutput)
			}
			for _, expected := range tc.expectParts {
				if !strings.Contains(logOutput, expected) {
					t.Fatalf("expected %q in success log, got %s", expected, logOutput)
				}
			}
			if strings.Contains(logOutput, "response_mode=") {
				t.Fatalf("did not expect response_mode in success log, got %s", logOutput)
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
			sseChunk(t, map[string]any{"content": `  "prompt":"moonlit girl",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "negative_prompt":"blurry",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "characters":[]` + "\n}"}),
			"",
			"data: [DONE]",
		}, "\n")

		return newHTTPResponse(http.StatusOK, body), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
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
						"content": `{"prompt":"fallback","negative_prompt":"fallback","characters":[]}`,
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

func newTestClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *TranslateClient {
	t.Helper()

	return &TranslateClient{
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
