package openai

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
)

type translateStub struct {
	modelName  string
	baseURLRaw string
	results    []stubResult
	callCount  int
	onCall     func()
}

type stubResult struct {
	result translationResult
	err    error
}

func (s *translateStub) translate(_ context.Context, _ string, _ domaindraw.Shape) (translationResult, error) {
	if s.onCall != nil {
		s.onCall()
	}
	if s.callCount >= len(s.results) {
		return translationResult{}, errors.New("unexpected extra call")
	}
	outcome := s.results[s.callCount]
	s.callCount++
	return outcome.result, outcome.err
}

func (s *translateStub) model() string {
	return s.modelName
}

func (s *translateStub) baseURL() string {
	return s.baseURLRaw
}

func TestNewTranslateFailoverClientBuildsProvidersFromConfigs(t *testing.T) {
	client := NewTranslateFailoverClient([]config.LLM{
		{BaseURL: "https://first.example/v1", Model: "first", TimeoutSec: 5},
		{BaseURL: "https://second.example/v1", Model: "second", TimeoutSec: 7},
	}, nil)

	if len(client.clients) != 2 {
		t.Fatalf("expected two providers, got %d", len(client.clients))
	}

	first, ok := client.clients[0].(*TranslateClient)
	if !ok {
		t.Fatalf("expected first provider to be *TranslateClient, got %T", client.clients[0])
	}
	second, ok := client.clients[1].(*TranslateClient)
	if !ok {
		t.Fatalf("expected second provider to be *TranslateClient, got %T", client.clients[1])
	}

	if first.cfg.BaseURL != "https://first.example/v1" || first.cfg.Model != "first" {
		t.Fatalf("unexpected first provider config: %#v", first.cfg)
	}
	if second.cfg.BaseURL != "https://second.example/v1" || second.cfg.Model != "second" {
		t.Fatalf("unexpected second provider config: %#v", second.cfg)
	}
}

func TestFailoverClientReturnsFirstSuccessWithoutFallback(t *testing.T) {
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		results: []stubResult{
			{
				result: translationResult{
					Translation:  domaindraw.Translation{Prompt: "pos", NegativePrompt: "neg"},
					ResponseMode: llmResponseModeJSON,
				},
			},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{err: errors.New("should not be called")},
		},
	}

	client := newTranslateFailoverClient([]translateProvider{first, second}, nil)
	translation, err := client.Translate(context.Background(), "moon", domaindraw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "pos" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if first.callCount != 1 {
		t.Fatalf("expected first client call count 1, got %d", first.callCount)
	}
	if second.callCount != 0 {
		t.Fatalf("expected second client untouched, got %d", second.callCount)
	}
}

func TestFailoverClientRetriesThenFallsBack(t *testing.T) {
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		results: []stubResult{
			{err: errors.New("boom-1")},
			{err: errors.New("boom-2")},
			{err: errors.New("boom-3")},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{
				result: translationResult{
					Translation:  domaindraw.Translation{Prompt: "pos-2", NegativePrompt: "neg-2"},
					ResponseMode: llmResponseModeJSON,
				},
			},
		},
	}

	client := newTranslateFailoverClient([]translateProvider{first, second}, nil)
	translation, err := client.Translate(context.Background(), "moon", domaindraw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "pos-2" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if first.callCount != attemptsPerLLM {
		t.Fatalf("expected first client retries %d, got %d", attemptsPerLLM, first.callCount)
	}
	if second.callCount != 1 {
		t.Fatalf("expected second client called once, got %d", second.callCount)
	}
}

func TestFailoverClientReturnsAggregateErrorAfterAllProvidersFail(t *testing.T) {
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		results: []stubResult{
			{err: errors.New("boom-1")},
			{err: errors.New("boom-2")},
			{err: errors.New("boom-3")},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{err: errors.New("boom-4")},
			{err: errors.New("boom-5")},
			{err: errors.New("boom-6")},
		},
	}

	client := newTranslateFailoverClient([]translateProvider{first, second}, nil)
	_, err := client.Translate(context.Background(), "moon", domaindraw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "all llm providers failed after 6 attempts") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFailoverClientStopsWhenParentContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		onCall:     cancel,
		results: []stubResult{
			{err: context.Canceled},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{err: errors.New("should not be called")},
		},
	}

	client := newTranslateFailoverClient([]translateProvider{first, second}, nil)
	_, err := client.Translate(ctx, "moon", domaindraw.ShapeSquare)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if second.callCount != 0 {
		t.Fatalf("expected second client untouched, got %d", second.callCount)
	}
}

func TestFailoverClientLogsProviderMetadataOnSuccessAndFailure(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		results: []stubResult{
			{err: errors.New("boom")},
			{err: errors.New("boom")},
			{err: errors.New("boom")},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{
				result: translationResult{
					Translation:  domaindraw.Translation{Prompt: "pos", NegativePrompt: "neg"},
					ResponseMode: llmResponseModeJSON,
				},
			},
		},
	}

	client := newTranslateFailoverClient(
		[]translateProvider{first, second},
		slog.New(slog.NewTextHandler(logBuffer, nil)),
	)

	_, err := client.Translate(context.Background(), "moon", domaindraw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "llm request started") {
		t.Fatalf("expected request log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "llm translate attempt failed") {
		t.Fatalf("expected failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "llm translated") {
		t.Fatalf("expected success log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "base_url=https://second.example/v1") {
		t.Fatalf("expected second base_url in request log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "model=second") {
		t.Fatalf("expected model in logs, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "attempt=1") {
		t.Fatalf("expected attempt in logs, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "prompt=pos") {
		t.Fatalf("expected prompt in success log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "negative_prompt=neg") {
		t.Fatalf("expected negative prompt in success log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "characters=[]") {
		t.Fatalf("expected empty characters in success log, got %s", logOutput)
	}
	if strings.Contains(logOutput, "response_mode=") {
		t.Fatalf("did not expect response_mode in success log, got %s", logOutput)
	}
}

func TestFailoverClientLogsRawResponseFromAttemptError(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	first := &translateStub{
		modelName:  "first",
		baseURLRaw: "https://first.example/v1",
		results: []stubResult{
			{
				err: withTranslateAttemptDetails(
					errors.New("parse llm json: invalid character '}' after top-level value"),
					`{"choices":[{"message":{"content":"{\"prompt\":\"bad\"}}}"}}]}__FAILOVER_RAW_TAIL__`,
					`{"prompt":"bad"}}`,
				),
			},
			{err: errors.New("boom-2")},
			{err: errors.New("boom-3")},
		},
	}
	second := &translateStub{
		modelName:  "second",
		baseURLRaw: "https://second.example/v1",
		results: []stubResult{
			{
				result: translationResult{
					Translation: domaindraw.Translation{Prompt: "pos", NegativePrompt: "neg"},
				},
			},
		},
	}

	client := newTranslateFailoverClient(
		[]translateProvider{first, second},
		slog.New(slog.NewTextHandler(logBuffer, nil)),
	)

	_, err := client.Translate(context.Background(), "moon", domaindraw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "__FAILOVER_RAW_TAIL__") {
		t.Fatalf("expected raw response in attempt failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `assistant_content="{\"prompt\":\"bad\"}}"`) {
		t.Fatalf("expected assistant content in attempt failure log, got %s", logOutput)
	}
}
