package task

import (
	"testing"
	"time"
)

func TestNewTaskStartsQueued(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}

	task, err := New("task-1", "user-1", "session-1", "draw a cat", context, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new task: %v", err)
	}

	if task.Status != StatusQueued {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Timeline.CreatedAt.IsZero() || task.Timeline.UpdatedAt.IsZero() {
		t.Fatal("expected timeline to be initialized")
	}
}

func TestTaskSuccessPath(t *testing.T) {
	task := newTestTask(t)

	if err := task.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := task.SetPrompt("masterpiece, cat"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := task.MarkDrawing(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := task.MarkCompleted("data/images/user-1/task-1.jpg", time.Unix(4, 0)); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	if task.Status != StatusCompleted {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Image == "" {
		t.Fatal("expected image path to be stored")
	}
	if task.Timeline.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestTaskFailurePath(t *testing.T) {
	task := newTestTask(t)
	taskError, err := NewError("LLM_TIMEOUT", "translating", "request timeout")
	if err != nil {
		t.Fatalf("new error: %v", err)
	}

	if err := task.MarkFailed(taskError, time.Unix(2, 0)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	if task.Status != StatusFailed {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Error == nil || task.Error.Code != "LLM_TIMEOUT" {
		t.Fatalf("unexpected task error: %#v", task.Error)
	}
	if task.Timeline.FailedAt == nil {
		t.Fatal("expected failed_at to be set")
	}
}

func TestTaskRejectsZeroValueContext(t *testing.T) {
	if _, err := New("task-1", "user-1", "session-1", "draw a cat", Context{}, time.Unix(1, 0)); err == nil {
		t.Fatal("expected error")
	}
}

func TestTaskStopPath(t *testing.T) {
	task := newTestTask(t)

	if err := task.MarkStopped(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark stopped: %v", err)
	}

	if task.Status != StatusStopped {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Timeline.StoppedAt == nil {
		t.Fatal("expected stopped_at to be set")
	}
}

func TestMarkDrawingRequiresPrompt(t *testing.T) {
	task := newTestTask(t)
	if err := task.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}

	if err := task.MarkDrawing(time.Unix(3, 0)); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetSourceTaskRejectsSelfReference(t *testing.T) {
	task := newTestTask(t)

	if err := task.SetSourceTask("task-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewContextRejectsInvalidJSON(t *testing.T) {
	if _, err := NewContext("{"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMarkFailedRejectsInvalidTaskError(t *testing.T) {
	task := newTestTask(t)

	if err := task.MarkFailed(TaskError{}, time.Unix(2, 0)); err == nil {
		t.Fatal("expected error")
	}
	if task.Status != StatusQueued {
		t.Fatalf("unexpected status after failed error validation: %s", task.Status)
	}
}

func newTestTask(t *testing.T) Task {
	t.Helper()

	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}

	task, err := New("task-1", "user-1", "session-1", "draw a cat", context, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
