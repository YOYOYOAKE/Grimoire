package llm

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"grimoire/internal/config"
)

func TestParsePromptJSONValidWithCharacters(t *testing.T) {
	t.Parallel()

	result, err := parsePromptJSON(`{
		"positivePrompt":"masterpiece, best quality",
		"negativePrompt":"lowres, blurry",
		"characterPrompts":[
			{"charPositivePrompt":"char1","charUnconcentPrompt":"char1_uc","centers":{"x":1,"y":"A"}},
			{"charPositivePrompt":"char2","charUnconcentPrompt":"char2_uc","centers":{"x":"3","y":"c"}},
			{"charPositivePrompt":"char3","charUnconcentPrompt":"char3_uc","centers":{"x":5,"y":"E"}}
		]
	}`)
	if err != nil {
		t.Fatalf("parsePromptJSON error: %v", err)
	}
	if result.PositivePrompt != "masterpiece, best quality" {
		t.Fatalf("unexpected positivePrompt: %q", result.PositivePrompt)
	}
	if result.NegativePrompt != "lowres, blurry" {
		t.Fatalf("unexpected negativePrompt: %q", result.NegativePrompt)
	}
	if len(result.Characters) != 3 {
		t.Fatalf("unexpected character count: %d", len(result.Characters))
	}

	assertCenter := func(idx int, x float64, y float64) {
		t.Helper()
		ch := result.Characters[idx]
		if ch.CenterX != x || ch.CenterY != y {
			t.Fatalf("unexpected center[%d]: got=(%.1f, %.1f) want=(%.1f, %.1f)", idx, ch.CenterX, ch.CenterY, x, y)
		}
	}
	assertCenter(0, 0.1, 0.1)
	assertCenter(1, 0.5, 0.5)
	assertCenter(2, 0.9, 0.9)
}

func TestParsePromptJSONInvalidCoordinate(t *testing.T) {
	t.Parallel()

	_, err := parsePromptJSON(`{
		"positivePrompt":"ok",
		"negativePrompt":"ok",
		"characterPrompts":[
			{"charPositivePrompt":"char1","charUnconcentPrompt":"char1_uc","centers":{"x":6,"y":"A"}}
		]
	}`)
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestParsePromptJSONAllowsEmptyCharUnconcentPrompt(t *testing.T) {
	t.Parallel()

	result, err := parsePromptJSON(`{
		"positivePrompt":"ok",
		"negativePrompt":"ok",
		"characterPrompts":[
			{"charPositivePrompt":"char1","charUnconcentPrompt":"","centers":{"x":3,"y":"C"}}
		]
	}`)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(result.Characters) != 1 {
		t.Fatalf("unexpected character count: %d", len(result.Characters))
	}
	if result.Characters[0].NegativePrompt != "" {
		t.Fatalf("expected empty negative prompt, got %q", result.Characters[0].NegativePrompt)
	}
}

func TestParsePromptJSONRejectLegacySchema(t *testing.T) {
	t.Parallel()

	_, err := parsePromptJSON(`{"positive_prompt":"x","negative_prompt":"y"}`)
	if err == nil {
		t.Fatalf("expected legacy schema rejection")
	}
	if !strings.Contains(err.Error(), "positivePrompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslateWithMockServer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Fatalf("missing bearer auth, got %q", got)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if stream, ok := reqBody["stream"].(bool); !ok || stream {
			t.Fatalf("expected stream=false, got %v", reqBody["stream"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"positivePrompt\":\"global\",\"negativePrompt\":\"global_uc\",\"characterPrompts\":[{\"charPositivePrompt\":\"char1\",\"charUnconcentPrompt\":\"char1_uc\",\"centers\":{\"x\":3,\"y\":\"C\"}}]}"}}]}`))
	}))
	defer srv.Close()

	cfgManager := mustConfigManager(t, srv.URL)
	client := NewOpenAIClient(cfgManager, slog.New(slog.NewTextHandler(io.Discard, nil)))

	result, err := client.Translate(t.Context(), "可爱的女孩", "square")
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	if result.PositivePrompt != "global" || result.NegativePrompt != "global_uc" {
		t.Fatalf("unexpected prompts: %+v", result)
	}
	if len(result.Characters) != 1 {
		t.Fatalf("unexpected character count: %d", len(result.Characters))
	}
	if result.Characters[0].CenterX != 0.5 || result.Characters[0].CenterY != 0.5 {
		t.Fatalf("unexpected center: %+v", result.Characters[0])
	}
}

func TestTranslateRetryOnParseFailure(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"positivePrompt\":\"missing negative\"}"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"positivePrompt\":\"ok\",\"negativePrompt\":\"ok_uc\",\"characterPrompts\":[]}"}}]}`))
	}))
	defer srv.Close()

	cfgManager := mustConfigManager(t, srv.URL)
	client := NewOpenAIClient(cfgManager, slog.New(slog.NewTextHandler(io.Discard, nil)))

	result, err := client.Translate(t.Context(), "test", "square")
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount.Load())
	}
	if result.PositivePrompt != "ok" || result.NegativePrompt != "ok_uc" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTranslateNetworkErrorNoRetry(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	cfgManager := mustConfigManager(t, srv.URL)
	client := NewOpenAIClient(cfgManager, slog.New(slog.NewTextHandler(io.Discard, nil)))

	_, err := client.Translate(t.Context(), "test", "square")
	if err == nil {
		t.Fatalf("expected error")
	}
	if requestCount.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount.Load())
	}
}

func TestTranslateSSEStyleResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"positivePrompt\\\":\\\"1girl\\\",\\\"negativePrompt\\\":\\\"lowres\\\",\\\"characterPrompts\\\":[]}\"}}]}\n"))
		_, _ = w.Write([]byte("data: [DONE]\n"))
	}))
	defer srv.Close()

	cfgManager := mustConfigManager(t, srv.URL)
	client := NewOpenAIClient(cfgManager, slog.New(slog.NewTextHandler(io.Discard, nil)))

	result, err := client.Translate(t.Context(), "test", "square")
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	if result.PositivePrompt != "1girl" || result.NegativePrompt != "lowres" {
		t.Fatalf("unexpected prompts: %+v", result)
	}
}

func mustConfigManager(t *testing.T, llmURL string) *config.Manager {
	t.Helper()
	ensureTestTelegramEnv()

	dbPath := filepath.Join(t.TempDir(), "grimoire.db")
	mgr, err := config.NewManager(dbPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Close()
	})
	if err := mgr.SetByPath("llm.base_url", llmURL); err != nil {
		t.Fatalf("set llm.base_url: %v", err)
	}
	if err := mgr.SetByPath("llm.api_key", "llm-key"); err != nil {
		t.Fatalf("set llm.api_key: %v", err)
	}
	if err := mgr.SetByPath("llm.model", "gpt-4o-mini"); err != nil {
		t.Fatalf("set llm.model: %v", err)
	}
	return mgr
}

var llmTestEnvOnce sync.Once

func ensureTestTelegramEnv() {
	llmTestEnvOnce.Do(func() {
		_ = os.Setenv(config.EnvTelegramBotToken, "token")
		_ = os.Setenv(config.EnvTelegramAdminUserID, "1")
		_ = os.Setenv(config.EnvTelegramProxyURL, "")
	})
}
