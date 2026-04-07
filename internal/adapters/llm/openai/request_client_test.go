package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	requestapp "grimoire/internal/app/request"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

func TestParseRequestOutput(t *testing.T) {
	request, err := parseRequestOutput(`{"request":"一位少女站在月下城堡前，长发，安静氛围，纵向构图。"}`)
	if err != nil {
		t.Fatalf("parse request output: %v", err)
	}
	if request == "" {
		t.Fatal("expected request")
	}
}

func TestParseRequestOutputRejectsMissingRequest(t *testing.T) {
	if _, err := parseRequestOutput(`{"request":"   "}`); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildRequestPayloadRejectsNonObjectSummary(t *testing.T) {
	input := newRequestInput(t)
	input.Summary = domainsession.NewSummary(`[]`)

	if _, err := buildRequestPayload(input); err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateSendsStructuredPayload(t *testing.T) {
	var requestBody map[string]any
	client := newTestRequestClient(t, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"request":"一位少女站在月下城堡前，长发，安静氛围，纵向构图。"}`)), nil
	})

	request, err := client.Generate(context.Background(), newRequestInput(t))
	if err != nil {
		t.Fatalf("generate request: %v", err)
	}
	if request == "" {
		t.Fatal("expected request")
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
}

func TestGenerateParsesRawSSEJSONFragments(t *testing.T) {
	client := newTestRequestClient(t, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			`data: {"request":"一位少女站在月下城堡前，`,
			`data: 长发，安静氛围，纵向构图。"}`,
			"data: [DONE]",
		}, "\n")
		return newHTTPResponse(http.StatusOK, body), nil
	})

	request, err := client.Generate(context.Background(), newRequestInput(t))
	if err != nil {
		t.Fatalf("generate request: %v", err)
	}
	if request == "" {
		t.Fatal("expected request")
	}
}

func TestGenerateRejectsUnsupportedFormat(t *testing.T) {
	client := newTestRequestClient(t, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, `{"unexpected":"payload"}`), nil
	})

	if _, err := client.Generate(context.Background(), newRequestInput(t)); err == nil {
		t.Fatal("expected error")
	}
}

func newTestRequestClient(t *testing.T, transport roundTripFunc) *RequestClient {
	t.Helper()

	return &RequestClient{
		cfg: config.LLM{
			BaseURL:    "https://api.openai.com/v1",
			APIKey:     "key",
			Model:      "gpt-4o-mini",
			TimeoutSec: 10,
		},
		httpClient: &http.Client{Transport: transport},
	}
}

func newRequestInput(t *testing.T) requestapp.GenerateInput {
	t.Helper()

	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	message, err := domainsession.NewMessage("message-1", "session-1", domainsession.MessageRoleUser, "我想画一座城堡", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return requestapp.GenerateInput{
		Summary:    domainsession.NewSummary(`{"goal":"castle"}`),
		Messages:   []domainsession.Message{message},
		Preference: preference,
	}
}
