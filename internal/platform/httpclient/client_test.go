package httpclient

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestNewUsesDefaultTimeoutAndDirectTransport(t *testing.T) {
	client := New(0, "", nil, "telegram")

	if client.Timeout != 60*time.Second {
		t.Fatalf("unexpected timeout: %v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type: %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected direct transport without proxy")
	}
}

func TestNewConfiguresProxyWhenValid(t *testing.T) {
	client := New(30, "http://127.0.0.1:8080", nil, "openai")

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type: %T", client.Transport)
	}
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL, err := transport.Proxy(request)
	if err != nil {
		t.Fatalf("resolve proxy: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected proxy url: %#v", proxyURL)
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("unexpected timeout: %v", client.Timeout)
	}
}

func TestNewFallsBackToDirectTransportWhenProxyInvalid(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	client := New(15, "http://[::1", logger, "nai")

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type: %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("expected invalid proxy to fall back to direct transport")
	}
	if logs.String() == "" {
		t.Fatal("expected warning log for invalid proxy")
	}
}
