package nai

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"grimoire/internal/config"
	"grimoire/internal/types"
)

func TestSubmitAndPollCompleted(t *testing.T) {
	t.Parallel()

	var pollCount atomic.Int32
	var submitBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/generate_image":
			raw, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			_ = json.Unmarshal(raw, &submitBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-123"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/get_result/job-123":
			count := pollCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if count == 1 {
				_, _ = w.Write([]byte(`{"status":"queued","queue_position":2}`))
				return
			}
			img := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
			_, _ = w.Write([]byte(`{"status":"completed","queue_position":0,"image_base64":"` + img + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := NewXianyunClient(mustConfigManager(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	routeClientToServer(t, client, srv)
	jobID, err := client.Submit(t.Context(), types.GenerateRequest{
		PositivePrompt: "1girl",
		NegativePrompt: "lowres",
		Shape:          "square",
	})
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}
	if jobID != "job-123" {
		t.Fatalf("unexpected job id: %s", jobID)
	}
	if got, ok := submitBody["use_coords"].(bool); !ok || got {
		t.Fatalf("expected use_coords=false, got %#v", submitBody["use_coords"])
	}
	if _, ok := submitBody["characterPrompts"]; ok {
		t.Fatalf("characterPrompts should be omitted when no characters")
	}
	if _, ok := submitBody["v4_prompt_char_captions"]; ok {
		t.Fatalf("v4_prompt_char_captions should be omitted when no characters")
	}
	if _, ok := submitBody["v4_negative_prompt_char_captions"]; ok {
		t.Fatalf("v4_negative_prompt_char_captions should be omitted when no characters")
	}

	first, err := client.Poll(t.Context(), jobID)
	if err != nil {
		t.Fatalf("Poll #1 error: %v", err)
	}
	if first.Status != "queued" || first.QueuePosition != 2 {
		t.Fatalf("unexpected first poll result: %+v", first)
	}

	second, err := client.Poll(t.Context(), jobID)
	if err != nil {
		t.Fatalf("Poll #2 error: %v", err)
	}
	if second.Status != "completed" || second.ImageBase64 == "" {
		t.Fatalf("unexpected second poll result: %+v", second)
	}
}

func TestPollFailedStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get_result/job-failed" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","error":"bad prompt"}`))
	}))
	defer srv.Close()

	client := NewXianyunClient(mustConfigManager(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	routeClientToServer(t, client, srv)
	out, err := client.Poll(t.Context(), "job-failed")
	if err != nil {
		t.Fatalf("Poll error: %v", err)
	}
	if out.Status != "failed" || out.Error != "bad prompt" {
		t.Fatalf("unexpected poll result: %+v", out)
	}
}

func TestPollHTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	client := NewXianyunClient(mustConfigManager(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	routeClientToServer(t, client, srv)
	_, err := client.Poll(t.Context(), "job-err")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSubmitWithCharacterPayloads(t *testing.T) {
	t.Parallel()

	var submitBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/generate_image" {
			raw, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			_ = json.Unmarshal(raw, &submitBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-char"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := NewXianyunClient(mustConfigManager(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	routeClientToServer(t, client, srv)
	_, err := client.Submit(t.Context(), types.GenerateRequest{
		PositivePrompt: "global",
		NegativePrompt: "global_uc",
		Shape:          "square",
		Characters: []types.CharacterPrompt{
			{
				PositivePrompt: "char1",
				NegativePrompt: "char1_uc",
				CenterX:        0.1,
				CenterY:        0.7,
			},
			{
				PositivePrompt: "char2",
				NegativePrompt: "char2_uc",
				CenterX:        0.9,
				CenterY:        0.3,
			},
		},
	})
	if err != nil {
		t.Fatalf("Submit error: %v", err)
	}

	charPrompts, ok := submitBody["characterPrompts"].([]any)
	if !ok || len(charPrompts) != 2 {
		t.Fatalf("unexpected characterPrompts: %#v", submitBody["characterPrompts"])
	}
	v4Pos, ok := submitBody["v4_prompt_char_captions"].([]any)
	if !ok || len(v4Pos) != 2 {
		t.Fatalf("unexpected v4_prompt_char_captions: %#v", submitBody["v4_prompt_char_captions"])
	}
	v4Neg, ok := submitBody["v4_negative_prompt_char_captions"].([]any)
	if !ok || len(v4Neg) != 2 {
		t.Fatalf("unexpected v4_negative_prompt_char_captions: %#v", submitBody["v4_negative_prompt_char_captions"])
	}

	firstChar, _ := charPrompts[0].(map[string]any)
	if firstChar["prompt"] != "char1" || firstChar["uc"] != "char1_uc" {
		t.Fatalf("unexpected first character prompt: %#v", firstChar)
	}
	center, _ := firstChar["center"].(map[string]any)
	if center["x"] != 0.1 || center["y"] != 0.7 {
		t.Fatalf("unexpected center: %#v", center)
	}
}

func mustConfigManager(t *testing.T) *config.Manager {
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
	if err := mgr.SetByPath("nai.api_key", "nai-key"); err != nil {
		t.Fatalf("set nai.api_key: %v", err)
	}
	if err := mgr.SetByPath("nai.model", "nai-model"); err != nil {
		t.Fatalf("set nai.model: %v", err)
	}
	return mgr
}

func routeClientToServer(t *testing.T, client *XianyunClient, srv *httptest.Server) {
	t.Helper()
	targetURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	baseRT := srv.Client().Transport
	client.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = targetURL.Scheme
			clone.URL.Host = targetURL.Host
			return baseRT.RoundTrip(clone)
		}),
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

var naiTestEnvOnce sync.Once

func ensureTestTelegramEnv() {
	naiTestEnvOnce.Do(func() {
		_ = os.Setenv(config.EnvTelegramBotToken, "token")
		_ = os.Setenv(config.EnvTelegramAdminUserID, "1")
		_ = os.Setenv(config.EnvTelegramProxyURL, "")
	})
}
