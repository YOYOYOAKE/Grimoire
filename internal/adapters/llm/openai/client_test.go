package openai

import (
	"bytes"
	"context"
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
	translation, err := parseTranslation(`{"positivePrompt":"moonlit girl","negativePrompt":"blurry"}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
}

func TestTranslateParsesSSEChunks(t *testing.T) {
	client := &Client{
		cfg: config.Config{
			LLM: config.LLM{
				BaseURL:    "https://api.openai.com/v1",
				APIKey:     "key",
				Model:      "gpt-4o-mini",
				TimeoutSec: 10,
			},
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := strings.Join([]string{
					`data: {"id":"1","choices":[{"delta":{"role":"assistant","content":""}}]}`,
					``,
					`data: {"id":"1","choices":[{"delta":{"content":"{\n"}}]}`,
					``,
					`data: {"id":"1","choices":[{"delta":{"content":"  \"positivePrompt\":\"moonlit girl\",\n"}}]}`,
					``,
					`data: {"id":"1","choices":[{"delta":{"content":"  \"negativePrompt\":\"blurry\"\n}"}}]}`,
					``,
					`data: [DONE]`,
				}, "\n")
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	}

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

func TestTranslateLogsRawResponseOnUnsupportedFormat(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := &Client{
		cfg: config.Config{
			LLM: config.LLM{
				BaseURL:    "https://api.openai.com/v1",
				APIKey:     "key",
				Model:      "gpt-4o-mini",
				TimeoutSec: 10,
			},
		},
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"unexpected":"payload"}`)),
				}, nil
			}),
		},
		logger: slog.New(slog.NewTextHandler(logBuffer, nil)),
	}

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
