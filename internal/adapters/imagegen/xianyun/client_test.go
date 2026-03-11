package xianyun

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/domain/draw"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestResolveDimensions(t *testing.T) {
	width, height, err := resolveDimensions(draw.ShapeLandscape)
	if err != nil {
		t.Fatalf("resolve dimensions: %v", err)
	}
	if width != 1216 || height != 832 {
		t.Fatalf("unexpected dimensions: %dx%d", width, height)
	}
}

func TestResolveDimensionsSupportsSmallAndLargeShapes(t *testing.T) {
	testCases := []struct {
		shape  draw.Shape
		width  int
		height int
	}{
		{shape: draw.ShapeSmallSquare, width: 640, height: 640},
		{shape: draw.ShapeSmallLandscape, width: 768, height: 512},
		{shape: draw.ShapeSmallPortrait, width: 512, height: 768},
		{shape: draw.ShapeLargeSquare, width: 1472, height: 1472},
		{shape: draw.ShapeLargeLandscape, width: 1536, height: 1024},
		{shape: draw.ShapeLargePortrait, width: 1014, height: 1536},
	}

	for _, tc := range testCases {
		width, height, err := resolveDimensions(tc.shape)
		if err != nil {
			t.Fatalf("resolve dimensions for %s: %v", tc.shape, err)
		}
		if width != tc.width || height != tc.height {
			t.Fatalf("unexpected dimensions for %s: %dx%d", tc.shape, width, height)
		}
	}
}

func TestSubmitLogsRequestMetadata(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, `{"job_id":"job-1"}`), nil
	})

	jobID, err := client.Submit(context.Background(), draw.GenerateRequest{
		Prompt:         "pos",
		NegativePrompt: "neg",
		Shape:          draw.ShapeSquare,
		Artists:        "artist:foo",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if jobID != "job-1" {
		t.Fatalf("unexpected job id: %s", jobID)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"nai request started",
		"base_url=https://image.idlecloud.cc/api",
		"model=nai-diffusion-4-5-full",
		"attempt=1",
		"shape=square",
		"artists=artist:foo",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in log output, got %s", expected, logOutput)
		}
	}
}

func TestSubmitRetriesUntilSuccess(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	requests := 0
	waitCalls := 0
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		requests++
		if requests < 3 {
			return newHTTPResponse(http.StatusBadGateway, `{"error":"busy"}`), nil
		}
		return newHTTPResponse(http.StatusOK, `{"job_id":"job-3"}`), nil
	})
	client.wait = func(_ context.Context, _ time.Duration) error {
		waitCalls++
		return nil
	}

	jobID, err := client.Submit(context.Background(), draw.GenerateRequest{
		Prompt:         "pos",
		NegativePrompt: "neg",
		Shape:          draw.ShapePortrait,
		Artists:        "artist:bar",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if jobID != "job-3" {
		t.Fatalf("unexpected job id: %s", jobID)
	}
	if requests != 3 {
		t.Fatalf("expected 3 requests, got %d", requests)
	}
	if waitCalls != 2 {
		t.Fatalf("expected 2 waits, got %d", waitCalls)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"attempt=1",
		"attempt=2",
		"attempt=3",
		"nai submit attempt failed",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in log output, got %s", expected, logOutput)
		}
	}
}

func TestSubmitReturnsErrorAfterThreeFailures(t *testing.T) {
	requests := 0
	waitCalls := 0
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		requests++
		return newHTTPResponse(http.StatusBadGateway, `{"error":"busy"}`), nil
	})
	client.wait = func(_ context.Context, _ time.Duration) error {
		waitCalls++
		return nil
	}

	_, err := client.Submit(context.Background(), draw.GenerateRequest{
		Prompt:         "pos",
		NegativePrompt: "neg",
		Shape:          draw.ShapeSquare,
		Artists:        "artist:foo",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if requests != 3 {
		t.Fatalf("expected 3 requests, got %d", requests)
	}
	if waitCalls != 2 {
		t.Fatalf("expected 2 waits, got %d", waitCalls)
	}
	if !strings.Contains(err.Error(), "submit nai status=502") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *Client {
	t.Helper()

	return &Client{
		cfg: config.Config{
			NAI: config.NAI{
				BaseURL:    "https://image.idlecloud.cc/api",
				APIKey:     "key",
				Model:      "nai-diffusion-4-5-full",
				TimeoutSec: 10,
			},
		},
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
		wait: func(_ context.Context, _ time.Duration) error {
			return nil
		},
	}
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
