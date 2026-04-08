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

func TestRestoreCompletedTask(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	timeline := mustNewTimeline(t, time.Unix(1, 0))
	if err := timeline.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := timeline.MarkDrawing(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := timeline.MarkCompleted(time.Unix(4, 0)); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	task, err := Restore(
		"task-1",
		"user-1",
		"session-1",
		"task-0",
		"draw a cat",
		"masterpiece, cat",
		"data/images/user-1/task-1.jpg",
		StatusCompleted,
		nil,
		timeline,
		context,
		"100",
		"200",
	)
	if err != nil {
		t.Fatalf("restore task: %v", err)
	}

	if task.Status != StatusCompleted {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.SourceTaskID != "task-0" {
		t.Fatalf("unexpected source task: %s", task.SourceTaskID)
	}
	if task.Image == "" || task.Prompt == "" {
		t.Fatal("expected restored prompt and image")
	}
}

func TestRestoreFailedTaskRequiresError(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	timeline := mustNewTimeline(t, time.Unix(1, 0))
	if err := timeline.MarkFailed(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	if _, err := Restore(
		"task-1",
		"user-1",
		"session-1",
		"",
		"draw a cat",
		"",
		"",
		StatusFailed,
		nil,
		timeline,
		context,
		"",
		"",
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestoreQueuedTaskRejectsTransitionTimeline(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	timeline := mustNewTimeline(t, time.Unix(1, 0))
	if err := timeline.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}

	if _, err := Restore(
		"task-1",
		"user-1",
		"session-1",
		"",
		"draw a cat",
		"",
		"",
		StatusQueued,
		nil,
		timeline,
		context,
		"",
		"",
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestoreFailedTaskRejectsDrawingWithoutTranslating(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	timeline := mustNewTimeline(t, time.Unix(1, 0))
	if err := timeline.MarkDrawing(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := timeline.MarkFailed(time.Unix(4, 0)); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	taskError, err := NewError("DRAW_FAILED", "drawing", "draw failed")
	if err != nil {
		t.Fatalf("new error: %v", err)
	}

	if _, err := Restore(
		"task-1",
		"user-1",
		"session-1",
		"",
		"draw a cat",
		"masterpiece, cat",
		"",
		StatusFailed,
		&taskError,
		timeline,
		context,
		"",
		"",
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestRestoreStoppedTaskRejectsMissingPromptAfterDrawing(t *testing.T) {
	context, err := NewContext(`{"version":1}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	timeline := mustNewTimeline(t, time.Unix(1, 0))
	if err := timeline.MarkTranslating(time.Unix(2, 0)); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := timeline.MarkDrawing(time.Unix(3, 0)); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	if err := timeline.MarkStopped(time.Unix(4, 0)); err != nil {
		t.Fatalf("mark stopped: %v", err)
	}

	if _, err := Restore(
		"task-1",
		"user-1",
		"session-1",
		"",
		"draw a cat",
		"",
		"",
		StatusStopped,
		nil,
		timeline,
		context,
		"",
		"",
	); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetMessageIDsTrimWhitespace(t *testing.T) {
	task := newTestTask(t)

	task.SetProgressMessageID(" progress-1 ")
	task.SetResultMessageID(" result-1 ")

	if task.ProgressMessageID != "progress-1" {
		t.Fatalf("unexpected progress message id: %q", task.ProgressMessageID)
	}
	if task.ResultMessageID != "result-1" {
		t.Fatalf("unexpected result message id: %q", task.ResultMessageID)
	}
}

func TestContextRawReturnsNormalizedJSON(t *testing.T) {
	context, err := NewContext(` {"version":1} `)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}

	if context.Raw() != `{"version":1}` {
		t.Fatalf("unexpected raw context: %q", context.Raw())
	}
}

func TestValidateTimelineForStatusRejectsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name     string
		status   Status
		timeline Timeline
	}{
		{
			name:     "translating requires translating timestamp",
			status:   StatusTranslating,
			timeline: mustNewTimeline(t, time.Unix(1, 0)),
		},
		{
			name:   "completed rejects failed timestamp",
			status: StatusCompleted,
			timeline: Timeline{
				CreatedAt:            time.Unix(1, 0),
				UpdatedAt:            time.Unix(4, 0),
				TranslatingStartedAt: timePtr(time.Unix(2, 0)),
				DrawingStartedAt:     timePtr(time.Unix(3, 0)),
				CompletedAt:          timePtr(time.Unix(4, 0)),
				FailedAt:             timePtr(time.Unix(5, 0)),
			},
		},
		{
			name:   "stopped rejects completed timestamp",
			status: StatusStopped,
			timeline: Timeline{
				CreatedAt:            time.Unix(1, 0),
				UpdatedAt:            time.Unix(4, 0),
				TranslatingStartedAt: timePtr(time.Unix(2, 0)),
				StoppedAt:            timePtr(time.Unix(4, 0)),
				CompletedAt:          timePtr(time.Unix(5, 0)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateTimelineForStatus(tt.status, tt.timeline); err == nil {
				t.Fatal("expected error")
			}
		})
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

func mustNewTimeline(t *testing.T, createdAt time.Time) Timeline {
	t.Helper()

	timeline, err := NewTimeline(createdAt)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	return timeline
}
