package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	domaintask "grimoire/internal/domain/task"
)

type runnerTaskRepositoryStub struct {
	getErr     error
	updateErr  error
	storedTask domaintask.Task
	updated    domaintask.Task
	order      *[]string
}

func (s *runnerTaskRepositoryStub) Get(_ context.Context, id string) (domaintask.Task, error) {
	if s.order != nil {
		*s.order = append(*s.order, "repo:get")
	}
	if s.getErr != nil {
		return domaintask.Task{}, s.getErr
	}
	if s.storedTask.ID != id {
		return domaintask.Task{}, fmt.Errorf("task %s not found", id)
	}
	return s.storedTask, nil
}

func (s *runnerTaskRepositoryStub) Update(_ context.Context, task domaintask.Task) error {
	if s.order != nil {
		*s.order = append(*s.order, "repo:update")
	}
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = task
	s.storedTask = task
	return nil
}

type runnerTxRunnerStub struct {
	calls int
	order *[]string
}

func (s *runnerTxRunnerStub) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	s.calls++
	if s.order != nil {
		*s.order = append(*s.order, "tx:start")
	}
	if err := fn(ctx); err != nil {
		return err
	}
	if s.order != nil {
		*s.order = append(*s.order, "tx:commit")
	}
	return nil
}

func TestStartTranslatingMovesQueuedTaskToTranslating(t *testing.T) {
	order := []string{}
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerQueuedTask(t, "task-1"),
		order:      &order,
	}
	txRunner := &runnerTxRunnerStub{order: &order}
	service := NewService(repository, txRunner, nil, nil, nil, nil, func() time.Time { return time.Unix(2, 0).UTC() })

	task, err := service.StartTranslating(context.Background(), RunCommand{TaskID: " task-1 "})
	if err != nil {
		t.Fatalf("start translating: %v", err)
	}

	if task.Status != domaintask.StatusTranslating {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Timeline.TranslatingStartedAt == nil || !task.Timeline.TranslatingStartedAt.Equal(time.Unix(2, 0).UTC()) {
		t.Fatalf("unexpected translating timeline: %#v", task.Timeline)
	}
	expectedOrder := []string{"tx:start", "repo:get", "repo:update", "tx:commit"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestStartDrawingPersistsPromptAndMovesTaskToDrawing(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerTranslatingTask(t, "task-1", ""),
	}
	service := NewService(repository, &runnerTxRunnerStub{}, nil, nil, nil, nil, func() time.Time { return time.Unix(3, 0).UTC() })

	task, err := service.StartDrawing(context.Background(), StartDrawingCommand{
		TaskID: "task-1",
		Prompt: " masterpiece, moonlit_girl ",
	})
	if err != nil {
		t.Fatalf("start drawing: %v", err)
	}

	if task.Status != domaintask.StatusDrawing {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", task.Prompt)
	}
	if task.Timeline.DrawingStartedAt == nil || !task.Timeline.DrawingStartedAt.Equal(time.Unix(3, 0).UTC()) {
		t.Fatalf("unexpected drawing timeline: %#v", task.Timeline)
	}
}

func TestStartDrawingUsesExistingPromptForRetryTask(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerTranslatingTask(t, "task-1", "masterpiece, moonlit_girl"),
	}
	service := NewService(repository, &runnerTxRunnerStub{}, nil, nil, nil, nil, func() time.Time { return time.Unix(3, 0).UTC() })

	task, err := service.StartDrawing(context.Background(), StartDrawingCommand{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("start drawing: %v", err)
	}
	if task.Status != domaintask.StatusDrawing {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", task.Prompt)
	}
}

func TestStartDrawingRejectsBlankPromptWithoutExistingPrompt(t *testing.T) {
	service := NewService(
		&runnerTaskRepositoryStub{storedTask: mustRunnerTranslatingTask(t, "task-1", "")},
		&runnerTxRunnerStub{},
		nil,
		nil,
		nil,
		nil,
		func() time.Time { return time.Unix(3, 0).UTC() },
	)

	_, err := service.StartDrawing(context.Background(), StartDrawingCommand{TaskID: "task-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCompleteMovesDrawingTaskToCompleted(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerDrawingTask(t, "task-1"),
	}
	service := NewService(repository, &runnerTxRunnerStub{}, nil, nil, nil, nil, func() time.Time { return time.Unix(4, 0).UTC() })

	task, err := service.Complete(context.Background(), CompleteCommand{
		TaskID: "task-1",
		Image:  "data/images/user-1/task-1.png",
	})
	if err != nil {
		t.Fatalf("complete task: %v", err)
	}

	if task.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Image != "data/images/user-1/task-1.png" {
		t.Fatalf("unexpected image: %q", task.Image)
	}
	if task.Timeline.CompletedAt == nil || !task.Timeline.CompletedAt.Equal(time.Unix(4, 0).UTC()) {
		t.Fatalf("unexpected completed timeline: %#v", task.Timeline)
	}
}

func TestFailMovesTaskToFailed(t *testing.T) {
	taskError, err := domaintask.NewError("NAI_TIMEOUT", "drawing", "request timeout after 60s")
	if err != nil {
		t.Fatalf("new task error: %v", err)
	}
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerDrawingTask(t, "task-1"),
	}
	service := NewService(repository, &runnerTxRunnerStub{}, nil, nil, nil, nil, func() time.Time { return time.Unix(4, 0).UTC() })

	task, err := service.Fail(context.Background(), FailCommand{
		TaskID: "task-1",
		Error:  taskError,
	})
	if err != nil {
		t.Fatalf("fail task: %v", err)
	}

	if task.Status != domaintask.StatusFailed {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Error == nil || task.Error.Code != "NAI_TIMEOUT" {
		t.Fatalf("unexpected task error: %#v", task.Error)
	}
	if task.Timeline.FailedAt == nil || !task.Timeline.FailedAt.Equal(time.Unix(4, 0).UTC()) {
		t.Fatalf("unexpected failed timeline: %#v", task.Timeline)
	}
}

func TestStoppedTaskRejectsFurtherTransition(t *testing.T) {
	task := mustRunnerDrawingTask(t, "task-1")
	if err := task.MarkStopped(time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("mark stopped: %v", err)
	}
	service := NewService(
		&runnerTaskRepositoryStub{storedTask: task},
		&runnerTxRunnerStub{},
		nil,
		nil,
		nil,
		nil,
		func() time.Time { return time.Unix(5, 0).UTC() },
	)

	_, err := service.StartTranslating(context.Background(), RunCommand{TaskID: "task-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTaskRequiresTxRunner(t *testing.T) {
	service := NewService(&runnerTaskRepositoryStub{}, nil, nil, nil, nil, nil, nil)

	_, err := service.StartTranslating(context.Background(), RunCommand{TaskID: "task-1"})
	if !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected tx runner required error, got %v", err)
	}
}

func mustRunnerQueuedTask(t *testing.T, taskID string) domaintask.Task {
	t.Helper()
	contextSnapshot, err := domaintask.NewContext(`{"summary":{"topic":"moon"}}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(taskID, "user-1", "session-1", "draw a moonlit girl", contextSnapshot, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}

func mustRunnerTranslatingTask(t *testing.T, taskID string, prompt string) domaintask.Task {
	t.Helper()
	task := mustRunnerQueuedTask(t, taskID)
	if prompt != "" {
		if err := task.SetPrompt(prompt); err != nil {
			t.Fatalf("set prompt: %v", err)
		}
	}
	if err := task.MarkTranslating(time.Unix(2, 0).UTC()); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	return task
}

func mustRunnerDrawingTask(t *testing.T, taskID string) domaintask.Task {
	t.Helper()
	task := mustRunnerTranslatingTask(t, taskID, "masterpiece, moonlit_girl")
	if err := task.MarkDrawing(time.Unix(3, 0).UTC()); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	return task
}
