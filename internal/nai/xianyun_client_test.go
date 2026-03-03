package nai

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		case r.Method == http.MethodPost && r.URL.Path == "/generate_image":
			raw, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			_ = json.Unmarshal(raw, &submitBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"job-123"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/get_result/job-123":
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

	client := NewXianyunClient(mustConfigManager(t, srv.URL), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
		if r.URL.Path != "/get_result/job-failed" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failed","error":"bad prompt"}`))
	}))
	defer srv.Close()

	client := NewXianyunClient(mustConfigManager(t, srv.URL), slog.New(slog.NewTextHandler(io.Discard, nil)))
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

	client := NewXianyunClient(mustConfigManager(t, srv.URL), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := client.Poll(t.Context(), "job-err")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestSubmitWithCharacterPayloads(t *testing.T) {
	t.Parallel()

	var submitBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/generate_image" {
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

	client := NewXianyunClient(mustConfigManager(t, srv.URL), slog.New(slog.NewTextHandler(io.Discard, nil)))
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

func mustConfigManager(t *testing.T, naiURL string) *config.Manager {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `telegram:
  bot_token: "token"
  admin_user_id: 1
llm:
  base_url: "https://example-llm.com/v1"
  api_key: "llm-key"
  model: "gpt-4o-mini"
  timeout_sec: 10
nai:
  base_url: "` + naiURL + `"
  api_key: "nai-key"
  model: "nai-model"
  poll_interval_sec: 1
generation:
  shape_default: "square"
  shape_map:
    square: "1024x1024"
    landscape: "1216x832"
    portrait: "832x1216"
  steps: 28
  scale: 5
  sampler: "k_euler"
  n_samples: 1
runtime:
  worker_concurrency: 1
  save_dir: "` + dir + `"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	mgr, err := config.NewManager(path)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return mgr
}
