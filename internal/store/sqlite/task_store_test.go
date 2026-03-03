package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"grimoire/internal/types"
)

func TestInitIdempotentAndNextTaskID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)

	if err := s.Init(ctx); err != nil {
		t.Fatalf("init first: %v", err)
	}
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init second: %v", err)
	}

	id1, err := s.NextTaskID(ctx)
	if err != nil {
		t.Fatalf("next id 1: %v", err)
	}
	id2, err := s.NextTaskID(ctx)
	if err != nil {
		t.Fatalf("next id 2: %v", err)
	}
	if id1 != "task-000001" || id2 != "task-000002" {
		t.Fatalf("unexpected ids: %s %s", id1, id2)
	}
}

func TestTaskLifecycleAndRecoverableFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	mustInit(t, s, ctx)

	idQueued, err := s.NextTaskID(ctx)
	if err != nil {
		t.Fatalf("next queued id: %v", err)
	}
	queuedTask := types.DrawTask{
		TaskID:    idQueued,
		ChatID:    1,
		UserID:    11,
		Prompt:    "queued prompt",
		Shape:     "square",
		CreatedAt: time.Now().Add(-3 * time.Minute),
	}
	if err := s.CreateTask(ctx, queuedTask); err != nil {
		t.Fatalf("create queued task: %v", err)
	}

	idPolling, err := s.NextTaskID(ctx)
	if err != nil {
		t.Fatalf("next polling id: %v", err)
	}
	pollingTask := types.DrawTask{
		TaskID:    idPolling,
		ChatID:    1,
		UserID:    11,
		Prompt:    "polling prompt",
		Shape:     "landscape",
		CreatedAt: time.Now().Add(-2 * time.Minute),
	}
	if err := s.CreateTask(ctx, pollingTask); err != nil {
		t.Fatalf("create polling task: %v", err)
	}
	if err := s.UpdateTaskStatus(ctx, idPolling, types.StatusProcessing, "polling", ""); err != nil {
		t.Fatalf("set polling status: %v", err)
	}
	if err := s.SetTaskJobID(ctx, idPolling, "job-polling"); err != nil {
		t.Fatalf("set polling job id: %v", err)
	}

	idFailed, err := s.NextTaskID(ctx)
	if err != nil {
		t.Fatalf("next failed id: %v", err)
	}
	failedTask := types.DrawTask{
		TaskID:    idFailed,
		ChatID:    1,
		UserID:    11,
		Prompt:    "failed prompt",
		Shape:     "portrait",
		CreatedAt: time.Now().Add(-1 * time.Minute),
	}
	if err := s.CreateTask(ctx, failedTask); err != nil {
		t.Fatalf("create failed task: %v", err)
	}
	if err := s.UpdateTaskStatus(ctx, idFailed, types.StatusFailed, "failed", "boom"); err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	recoverable, err := s.ListRecoverableTasks(ctx)
	if err != nil {
		t.Fatalf("list recoverable: %v", err)
	}
	if len(recoverable) != 2 {
		t.Fatalf("expected 2 recoverable tasks, got %d", len(recoverable))
	}
	if recoverable[0].TaskID != idQueued {
		t.Fatalf("expected first recoverable=%s, got %s", idQueued, recoverable[0].TaskID)
	}
	if recoverable[0].ResumeJobID != "" {
		t.Fatalf("queued task should not have resume job id, got %q", recoverable[0].ResumeJobID)
	}
	if recoverable[1].TaskID != idPolling {
		t.Fatalf("expected second recoverable=%s, got %s", idPolling, recoverable[1].TaskID)
	}
	if recoverable[1].ResumeJobID != "job-polling" {
		t.Fatalf("polling task should carry resume job id, got %q", recoverable[1].ResumeJobID)
	}

	if err := s.SaveTaskResult(ctx, idPolling, "job-polling", "/tmp/out.png", time.Now()); err != nil {
		t.Fatalf("save task result: %v", err)
	}
	stored, err := s.GetTaskByID(ctx, idPolling)
	if err != nil {
		t.Fatalf("get task by id: %v", err)
	}
	if stored.TaskID != idPolling {
		t.Fatalf("unexpected stored task id: %s", stored.TaskID)
	}
}

func TestGalleryAppendAndList(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := newTestStore(t)
	mustInit(t, s, ctx)

	base := time.Now()
	if err := s.AppendGalleryItem(ctx, 100, 200, "task-000001", "job-1", "/tmp/1.png", "cap1", base); err != nil {
		t.Fatalf("append gallery 1: %v", err)
	}
	if err := s.AppendGalleryItem(ctx, 100, 200, "task-000002", "job-2", "/tmp/2.png", "cap2", base.Add(time.Second)); err != nil {
		t.Fatalf("append gallery 2: %v", err)
	}

	items, err := s.ListGalleryItems(ctx, 100, 200)
	if err != nil {
		t.Fatalf("list gallery: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 gallery items, got %d", len(items))
	}
	if items[0].TaskID != "task-000001" || items[1].TaskID != "task-000002" {
		t.Fatalf("unexpected gallery order: %+v", items)
	}
}

func newTestStore(t *testing.T) *TaskStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "grimoire.db")
	s, err := NewTaskStore(path)
	if err != nil {
		t.Fatalf("new task store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s
}

func mustInit(t *testing.T, s *TaskStore, ctx context.Context) {
	t.Helper()
	if err := s.Init(ctx); err != nil {
		t.Fatalf("init store: %v", err)
	}
}
