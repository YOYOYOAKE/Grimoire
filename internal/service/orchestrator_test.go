package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
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

func TestCancelTaskBeforeProcessSkipsExecution(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{
		result: types.TranslationResult{
			PositivePrompt: "global_pos",
			NegativePrompt: "global_neg",
		},
	}
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

	if !orch.CancelTask("task-000888") {
		t.Fatalf("expected cancel request accepted")
	}
	orch.ProcessTask(context.Background(), types.DrawTask{
		TaskID:          "task-000888",
		ChatID:          1,
		UserID:          1,
		StatusMessageID: 66,
		Prompt:          "should not run",
		Shape:           "square",
	})

	if translator.calls != 0 {
		t.Fatalf("expected translator not called")
	}
	if generator.pollCalls != 0 || generator.submitCalls != 0 {
		t.Fatalf("expected generator not called")
	}
	if taskStore.lastStatus != "cancelled" {
		t.Fatalf("expected cancelled status persisted, got %q", taskStore.lastStatus)
	}
}

func TestCancelTaskDuringPollWaitMarksCancelled(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{}
	generator := &cancelDuringPollWaitGenerator{firstPollDone: make(chan struct{})}
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

	done := make(chan struct{})
	go func() {
		orch.ProcessTask(context.Background(), types.DrawTask{
			TaskID:          "task-000777",
			ChatID:          1,
			UserID:          1,
			StatusMessageID: 99,
			Shape:           "square",
			ResumeJobID:     "job-cancel",
		})
		close(done)
	}()

	select {
	case <-generator.firstPollDone:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting first poll")
	}

	if !orch.CancelTask("task-000777") {
		t.Fatal("expected cancel request accepted")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting task exit")
	}

	if taskStore.lastStatus != "cancelled" {
		t.Fatalf("expected cancelled status persisted, got %q", taskStore.lastStatus)
	}
	if countTextsContaining(notifier.allTexts(), "状态: cancelled") == 0 {
		t.Fatalf("expected cancelled status notification, got=%v", notifier.allTexts())
	}
}

func TestProcessingOver3MinutesWarnsOnceAndContinues(t *testing.T) {
	t.Parallel()

	cfg := mustConfigManager(t)
	translator := &stubTranslator{}
	generator := &processingThenFailGenerator{processingPolls: 4}
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
	orch.pollIntervalOverride = 10 * time.Millisecond
	orch.processingWarningAfter = 25 * time.Millisecond

	orch.ProcessTask(context.Background(), types.DrawTask{
		TaskID:          "task-000778",
		ChatID:          1,
		UserID:          1,
		StatusMessageID: 100,
		Shape:           "square",
		ResumeJobID:     "job-processing",
	})

	texts := notifier.allTexts()
	warnCount := countTextsContaining(texts, "任务可能失败，系统继续轮询")
	if warnCount != 1 {
		t.Fatalf("expected warning once, got=%d texts=%v", warnCount, texts)
	}
	if taskStore.lastStatus != types.StatusFailed {
		t.Fatalf("expected task ended by generator failure, got %q", taskStore.lastStatus)
	}
	if generator.pollCalls.Load() < 5 {
		t.Fatalf("expected polling continued after warning, poll_calls=%d", generator.pollCalls.Load())
	}
}

func countTextsContaining(texts []string, needle string) int {
	count := 0
	for _, text := range texts {
		if strings.Contains(text, needle) {
			count++
		}
	}
	return count
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

type cancelDuringPollWaitGenerator struct {
	firstPollDone chan struct{}
}

func (g *cancelDuringPollWaitGenerator) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	return "unexpected", nil
}

func (g *cancelDuringPollWaitGenerator) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	select {
	case <-g.firstPollDone:
	default:
		close(g.firstPollDone)
	}
	return types.JobResult{
		Status:        types.StatusQueued,
		QueuePosition: 1,
	}, nil
}

type processingThenFailGenerator struct {
	processingPolls int32
	pollCalls       atomic.Int32
}

func (g *processingThenFailGenerator) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	return "unexpected", nil
}

func (g *processingThenFailGenerator) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	call := g.pollCalls.Add(1)
	if call <= g.processingPolls {
		return types.JobResult{
			Status:        types.StatusProcessing,
			QueuePosition: 1,
		}, nil
	}
	return types.JobResult{
		Status: types.StatusFailed,
		Error:  "forced failure",
	}, nil
}

type stubNotifier struct {
	editPhotoCalls   int
	notifyPhotoCalls int
	notifyTexts      []string
	editTexts        []string
}

func (s *stubNotifier) NotifyText(ctx context.Context, chatID int64, text string) (int64, error) {
	s.notifyTexts = append(s.notifyTexts, text)
	return 1, nil
}

func (s *stubNotifier) EditText(ctx context.Context, chatID int64, messageID int64, text string) error {
	s.editTexts = append(s.editTexts, text)
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

func (s *stubNotifier) allTexts() []string {
	out := make([]string, 0, len(s.notifyTexts)+len(s.editTexts))
	out = append(out, s.notifyTexts...)
	out = append(out, s.editTexts...)
	return out
}

type stubTaskStore struct {
	updateStatusCalls int
	setJobIDCalls     int
	saveResultCalls   int
	lastStatus        string
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
	s.lastStatus = status
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
	dbPath := t.TempDir() + "/grimoire.db"
	mgr, err := config.NewManager(dbPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Close()
	})
	if err := mgr.SetByPath("generation.artist", "artist_prefix"); err != nil {
		t.Fatalf("set generation.artist: %v", err)
	}
	return mgr
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "grimoire-service-test-*")
	if err != nil {
		panic(err)
	}

	if err := os.MkdirAll(dir+"/configs", 0o755); err != nil {
		panic(err)
	}
	configYAML := `
telegram:
  bot_token: "token"
  admin_user_id: 1
  proxy: ""
  timeout_sec: 60
llm:
  timeout_sec: 180
  openai_custom:
    enable: true
    base_url: "https://example-llm.com/v1"
    api_key: "llm-key"
    model: "gpt-4o-mini"
    proxy: ""
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
nai:
  base_url: "https://image.idlecloud.cc/api"
  api_key: "nai-key"
  model: "nai-model"
  timeout_sec: 180
  proxy: ""
`
	if err := os.WriteFile(dir+"/configs/config.yaml", []byte(strings.TrimSpace(configYAML)+"\n"), 0o600); err != nil {
		panic(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.Chdir(wd)
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
