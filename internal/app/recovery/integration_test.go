package recovery

import (
	"context"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

func TestRecoverWithSQLiteRepositoryRequeuesOnlyRecoverableTasks(t *testing.T) {
	ctx := context.Background()
	db := sqlitefixture.OpenDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	sqlitefixture.CreateUserAndSession(t, db, "user-1", "session-1", domainpreferences.DefaultPreference())

	for _, task := range []domaintask.Task{
		mustRecoverySQLiteTask(t, "task-queued", domaintask.StatusQueued, time.Unix(1, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-translating", domaintask.StatusTranslating, time.Unix(2, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-drawing", domaintask.StatusDrawing, time.Unix(3, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-completed", domaintask.StatusCompleted, time.Unix(4, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-failed", domaintask.StatusFailed, time.Unix(5, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-stopped", domaintask.StatusStopped, time.Unix(6, 0).UTC()),
	} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	scheduler := &recoverySchedulerStub{}
	service := NewService(taskRepo, scheduler)

	result, err := service.Recover(ctx, RecoverCommand{})
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	want := []string{"task-queued", "task-translating", "task-drawing"}
	if len(result.RequeuedTaskIDs) != len(want) {
		t.Fatalf("unexpected requeued count: %#v", result.RequeuedTaskIDs)
	}
	for index, taskID := range want {
		if result.RequeuedTaskIDs[index] != taskID {
			t.Fatalf("unexpected requeued ids: %#v", result.RequeuedTaskIDs)
		}
		if scheduler.taskIDs[index] != taskID {
			t.Fatalf("unexpected scheduled ids: %#v", scheduler.taskIDs)
		}
	}
}

func mustRecoverySQLiteTask(t *testing.T, id string, status domaintask.Status, createdAt time.Time) domaintask.Task {
	t.Helper()

	contextSnapshot, err := domaintask.NewContext(`{"version":1,"shape":"square"}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(id, "user-1", "session-1", "draw a moonlit girl", contextSnapshot, createdAt)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}

	switch status {
	case domaintask.StatusQueued:
		return task
	case domaintask.StatusTranslating:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		return task
	case domaintask.StatusDrawing:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		if err := task.SetPromptBundle(mustRecoverySQLitePromptBundle(t, "masterpiece, moonlit_girl")); err != nil {
			t.Fatalf("set prompt bundle: %v", err)
		}
		if err := task.MarkDrawing(createdAt.Add(2 * time.Second)); err != nil {
			t.Fatalf("mark drawing: %v", err)
		}
		return task
	case domaintask.StatusCompleted:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		if err := task.SetPromptBundle(mustRecoverySQLitePromptBundle(t, "masterpiece, moonlit_girl")); err != nil {
			t.Fatalf("set prompt bundle: %v", err)
		}
		if err := task.MarkDrawing(createdAt.Add(2 * time.Second)); err != nil {
			t.Fatalf("mark drawing: %v", err)
		}
		if err := task.MarkCompleted("data/images/user-1/"+id+".jpg", createdAt.Add(3*time.Second)); err != nil {
			t.Fatalf("mark completed: %v", err)
		}
		return task
	case domaintask.StatusFailed:
		taskError, err := domaintask.NewError("PROMPT_TRANSLATE_FAILED", "translating", "boom")
		if err != nil {
			t.Fatalf("new task error: %v", err)
		}
		if err := task.MarkFailed(taskError, createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
		return task
	case domaintask.StatusStopped:
		if err := task.MarkStopped(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark stopped: %v", err)
		}
		return task
	default:
		t.Fatalf("unsupported status: %s", status)
		return domaintask.Task{}
	}
}

func mustRecoverySQLitePromptBundle(t *testing.T, prompt string) domaintask.PromptBundle {
	t.Helper()
	bundle, err := domaintask.NewPromptBundle(prompt, "", nil)
	if err != nil {
		t.Fatalf("new prompt bundle: %v", err)
	}
	return bundle
}
