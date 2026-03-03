package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/store"
	"grimoire/internal/types"
)

func TestProcessTaskPassesCharactersAndArtistPrefix(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{
		result: types.TranslationResult{
			PositivePrompt: "global_pos",
			NegativePrompt: "global_neg",
			Characters: []types.CharacterPrompt{
				{
					PositivePrompt: "char1_pos",
					NegativePrompt: "char1_neg",
					CenterX:        0.5,
					CenterY:        0.1,
				},
			},
		},
	}
	generator := &captureGenerator{
		submitErr: errors.New("submit stop"),
	}
	notifier := &stubNotifier{}
	taskStore := &stubTaskStore{}
	orch := NewOrchestrator(
		translator,
		generator,
		notifier,
		cfg,
		taskStore,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	orch.ProcessTask(context.Background(), types.DrawTask{
		TaskID: "task-000001",
		ChatID: 1,
		UserID: 1,
		Prompt: "test prompt",
		Shape:  "square",
	})

	if !generator.called {
		t.Fatalf("expected generator.Submit called")
	}
	if generator.req.PositivePrompt != "artist_prefix, global_pos" {
		t.Fatalf("unexpected positive prompt: %q", generator.req.PositivePrompt)
	}
	if generator.req.NegativePrompt != "global_neg" {
		t.Fatalf("unexpected negative prompt: %q", generator.req.NegativePrompt)
	}
	if len(generator.req.Characters) != 1 {
		t.Fatalf("unexpected character count: %d", len(generator.req.Characters))
	}
	ch := generator.req.Characters[0]
	if ch.PositivePrompt != "char1_pos" || ch.NegativePrompt != "char1_neg" || ch.CenterX != 0.5 || ch.CenterY != 0.1 {
		t.Fatalf("unexpected character payload: %+v", ch)
	}
}

func TestProcessTaskCompletedEditsSameMessageToPhoto(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{
		result: types.TranslationResult{
			PositivePrompt: "global_pos",
			NegativePrompt: "global_neg",
		},
	}
	generator := &completeGenerator{}
	notifier := &stubNotifier{}
	taskStore := &stubTaskStore{}
	orch := NewOrchestrator(
		translator,
		generator,
		notifier,
		cfg,
		taskStore,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	orch.ProcessTask(context.Background(), types.DrawTask{
		TaskID:          "task-000002",
		ChatID:          1,
		UserID:          1,
		StatusMessageID: 77,
		Prompt:          "test prompt",
		Shape:           "square",
	})

	if notifier.editPhotoCalls != 1 {
		t.Fatalf("expected EditPhoto once, got %d", notifier.editPhotoCalls)
	}
	if notifier.notifyPhotoCalls != 0 {
		t.Fatalf("expected NotifyPhoto not called, got %d", notifier.notifyPhotoCalls)
	}
	if taskStore.saveResultCalls != 1 {
		t.Fatalf("expected SaveTaskResult called once, got %d", taskStore.saveResultCalls)
	}
}

func TestProcessTaskResumeJobIDSkipsTranslateAndSubmit(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{}
	generator := &resumeGenerator{}
	notifier := &stubNotifier{}
	taskStore := &stubTaskStore{}

	orch := NewOrchestrator(
		translator,
		generator,
		notifier,
		cfg,
		taskStore,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	orch.ProcessTask(context.Background(), types.DrawTask{
		TaskID:          "task-000009",
		ChatID:          1,
		UserID:          1,
		StatusMessageID: 88,
		Prompt:          "should be ignored",
		Shape:           "square",
		ResumeJobID:     "resume-job",
	})

	if generator.submitCalls != 0 {
		t.Fatalf("expected no submit calls for resumed task")
	}
	if translator.calls != 0 {
		t.Fatalf("expected no translate calls for resumed task")
	}
	if generator.pollCalls == 0 {
		t.Fatalf("expected poll called")
	}
}

type stubTranslator struct {
	result types.TranslationResult
	err    error
	calls  int
}

func (s *stubTranslator) Translate(ctx context.Context, naturalText string, shape string) (types.TranslationResult, error) {
	s.calls++
	return s.result, s.err
}

type captureGenerator struct {
	called    bool
	req       types.GenerateRequest
	submitErr error
}

func (g *captureGenerator) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	g.called = true
	g.req = req
	return "", g.submitErr
}

func (g *captureGenerator) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	return types.JobResult{}, errors.New("not implemented")
}

type completeGenerator struct{}

func (g *completeGenerator) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	return "job-complete", nil
}

func (g *completeGenerator) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	return types.JobResult{
		Status:      types.StatusCompleted,
		ImageBase64: base64.StdEncoding.EncodeToString([]byte("png-bytes")),
	}, nil
}

type resumeGenerator struct {
	submitCalls int
	pollCalls   int
}

func (g *resumeGenerator) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	g.submitCalls++
	return "unexpected", nil
}

func (g *resumeGenerator) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	g.pollCalls++
	return types.JobResult{
		Status:      types.StatusCompleted,
		ImageBase64: base64.StdEncoding.EncodeToString([]byte("png-bytes")),
	}, nil
}

type stubNotifier struct {
	editPhotoCalls   int
	notifyPhotoCalls int
}

func (s *stubNotifier) NotifyText(ctx context.Context, chatID int64, text string) (int64, error) {
	return 1, nil
}

func (s *stubNotifier) EditText(ctx context.Context, chatID int64, messageID int64, text string) error {
	return nil
}

func (s *stubNotifier) EditPhoto(ctx context.Context, chatID int64, messageID int64, filePath string, caption string) error {
	s.editPhotoCalls++
	return nil
}

func (s *stubNotifier) NotifyPhoto(ctx context.Context, chatID int64, filePath string, caption string) error {
	s.notifyPhotoCalls++
	return nil
}

type stubTaskStore struct {
	updateStatusCalls int
	setJobIDCalls     int
	saveResultCalls   int
}

func (s *stubTaskStore) Init(ctx context.Context) error { return nil }
func (s *stubTaskStore) NextTaskID(ctx context.Context) (string, error) {
	return "task-000001", nil
}
func (s *stubTaskStore) CreateInboundMessage(ctx context.Context, chatID, userID, messageID int64, text string, createdAt time.Time) error {
	return nil
}
func (s *stubTaskStore) CreateTask(ctx context.Context, task types.DrawTask) error { return nil }
func (s *stubTaskStore) UpdateTaskStatus(ctx context.Context, taskID string, status string, stage string, errMsg string) error {
	s.updateStatusCalls++
	return nil
}
func (s *stubTaskStore) SetTaskJobID(ctx context.Context, taskID string, jobID string) error {
	s.setJobIDCalls++
	return nil
}
func (s *stubTaskStore) SaveTaskResult(ctx context.Context, taskID string, jobID string, filePath string, completedAt time.Time) error {
	s.saveResultCalls++
	return nil
}
func (s *stubTaskStore) GetTaskByID(ctx context.Context, taskID string) (types.DrawTask, error) {
	return types.DrawTask{}, store.ErrNotFound
}
func (s *stubTaskStore) ListRecoverableTasks(ctx context.Context) ([]types.DrawTask, error) {
	return nil, nil
}
func (s *stubTaskStore) AppendGalleryItem(ctx context.Context, chatID, messageID int64, taskID, jobID, filePath, caption string, createdAt time.Time) error {
	return nil
}
func (s *stubTaskStore) ListGalleryItems(ctx context.Context, chatID, messageID int64) ([]store.GalleryItem, error) {
	return nil, nil
}

func mustConfigManager(t *testing.T) *config.Manager {
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
  base_url: "https://example.com/api"
  api_key: "nai-key"
  model: "nai-model"
  poll_interval_sec: 1
generation:
  shape_default: "square"
  artist: "artist_prefix"
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
  sqlite_path: "` + filepath.Join(dir, "grimoire.db") + `"
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
