package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintask "grimoire/internal/domain/task"
)

func TestTaskRepositoryCreateAndGet(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	task := newTaskFixture(t, "task-1", "user-1", "session-1", "draw a castle", time.Unix(1, 0))
	if err := repository.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := repository.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected status: %s", got.Status)
	}
	if got.Context.Raw() != `{"version":1,"shape":"square"}` {
		t.Fatalf("unexpected context: %s", got.Context.Raw())
	}
}

func TestTaskRepositoryUpdatePersistsMutableFields(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	task := newTaskFixture(t, "task-1", "user-1", "session-1", "draw a castle", time.Unix(1, 0))
	if err := repository.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	task.SetProgressMessageID("progress-1")
	if err := task.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := task.SetPrompt("masterpiece, castle"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := task.MarkDrawing(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := task.MarkCompleted("data/images/user-1/task-1.jpg", time.Unix(4, 0)); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	task.SetResultMessageID("result-1")

	if err := repository.Update(context.Background(), task); err != nil {
		t.Fatalf("update task: %v", err)
	}

	got, err := repository.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get updated task: %v", err)
	}
	if got.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected status: %s", got.Status)
	}
	if got.Prompt != "masterpiece, castle" {
		t.Fatalf("unexpected prompt: %q", got.Prompt)
	}
	if got.Image != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected image: %q", got.Image)
	}
	if got.ProgressMessageID != "progress-1" || got.ResultMessageID != "result-1" {
		t.Fatalf("unexpected message ids: progress=%q result=%q", got.ProgressMessageID, got.ResultMessageID)
	}
	if got.Timeline.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestTaskRepositoryUpdatePersistsFailureError(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	task := newTaskFixture(t, "task-1", "user-1", "session-1", "draw a castle", time.Unix(1, 0))
	if err := repository.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	taskError, err := domaintask.NewError("NAI_TIMEOUT", "drawing", "request timeout after 60s")
	if err != nil {
		t.Fatalf("new task error: %v", err)
	}
	if err := task.MarkFailed(taskError, time.Unix(2, 0)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	if err := repository.Update(context.Background(), task); err != nil {
		t.Fatalf("update failed task: %v", err)
	}

	got, err := repository.Get(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get failed task: %v", err)
	}
	if got.Status != domaintask.StatusFailed {
		t.Fatalf("unexpected status: %s", got.Status)
	}
	if got.Error == nil || got.Error.Code != "NAI_TIMEOUT" {
		t.Fatalf("unexpected error payload: %#v", got.Error)
	}
}

func TestTaskRepositoryUpdateRejectsImmutableFieldChange(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")
	task := newTaskFixture(t, "task-1", "user-1", "session-1", "draw a castle", time.Unix(1, 0))

	if err := repository.Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	task.Request = "draw a forest"
	if err := repository.Update(context.Background(), task); err == nil {
		t.Fatal("expected error")
	}
}

func TestTaskRepositoryListRecoverable(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	queued := newTaskFixture(t, "task-queued", "user-1", "session-1", "queued", time.Unix(1, 0))
	translating := newTaskFixture(t, "task-translating", "user-1", "session-1", "translating", time.Unix(2, 0))
	if err := translating.MarkTranslating(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	drawing := newTaskFixture(t, "task-drawing", "user-1", "session-1", "drawing", time.Unix(4, 0))
	if err := drawing.MarkTranslating(time.Unix(5, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := drawing.SetPrompt("prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := drawing.MarkDrawing(time.Unix(6, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	completed := newTaskFixture(t, "task-completed", "user-1", "session-1", "completed", time.Unix(7, 0))
	if err := completed.MarkTranslating(time.Unix(8, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := completed.SetPrompt("prompt"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := completed.MarkDrawing(time.Unix(9, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := completed.MarkCompleted("data/images/user-1/task-completed.jpg", time.Unix(10, 0)); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	for _, task := range []domaintask.Task{drawing, queued, completed, translating} {
		if err := repository.Create(context.Background(), task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	got, err := repository.ListRecoverable(context.Background())
	if err != nil {
		t.Fatalf("list recoverable: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("unexpected recoverable count: %d", len(got))
	}
	if got[0].ID != "task-queued" || got[1].ID != "task-translating" || got[2].ID != "task-drawing" {
		t.Fatalf("unexpected recoverable order: %#v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestTaskRepositoryListBySourceTask(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	root := newTaskFixture(t, "task-root", "user-1", "session-1", "root", time.Unix(1, 0))
	if err := repository.Create(context.Background(), root); err != nil {
		t.Fatalf("create root task: %v", err)
	}

	childOne := newTaskFixture(t, "task-child-1", "user-1", "session-1", "retry-1", time.Unix(2, 0))
	if err := childOne.SetSourceTask("task-root"); err != nil {
		t.Fatalf("set source task: %v", err)
	}
	childTwo := newTaskFixture(t, "task-child-2", "user-1", "session-1", "retry-2", time.Unix(3, 0))
	if err := childTwo.SetSourceTask("task-root"); err != nil {
		t.Fatalf("set source task: %v", err)
	}
	other := newTaskFixture(t, "task-other", "user-1", "session-1", "other", time.Unix(4, 0))

	for _, task := range []domaintask.Task{childTwo, other, childOne} {
		if err := repository.Create(context.Background(), task); err != nil {
			t.Fatalf("create child task %s: %v", task.ID, err)
		}
	}

	got, err := repository.ListBySourceTask(context.Background(), "task-root")
	if err != nil {
		t.Fatalf("list by source task: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected child task count: %d", len(got))
	}
	if got[0].ID != "task-child-1" || got[1].ID != "task-child-2" {
		t.Fatalf("unexpected child task order: %#v", []string{got[0].ID, got[1].ID})
	}
}

func TestTaskRepositoryCreateRejectsInvalidTask(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)

	err := repository.Create(context.Background(), domaintask.Task{
		ID: "task-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTaskRepositoryUsesTransactionConnection(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewTaskRepository(db)
	runner := NewTxRunner(db)
	createTestSessionRecord(t, db, "session-1", "user-1")
	expectedErr := errors.New("rollback")

	err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := repository.Create(ctx, newTaskFixture(t, "task-1", "user-1", "session-1", "draw a castle", time.Unix(1, 0))); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if got := countRows(t, db, "tasks"); got != 0 {
		t.Fatalf("expected rollback to leave task count at 0, got %d", got)
	}
}

func newTaskFixture(t *testing.T, id string, userID string, sessionID string, request string, createdAt time.Time) domaintask.Task {
	t.Helper()

	context, err := domaintask.NewContext(`{"version":1,"shape":"square"}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(id, userID, sessionID, request, context, createdAt)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
