package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintask "grimoire/internal/domain/task"
)

type recoveryTaskRepositoryStub struct {
	tasks []domaintask.Task
	err   error
}

func (s *recoveryTaskRepositoryStub) ListRecoverable(context.Context) ([]domaintask.Task, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]domaintask.Task(nil), s.tasks...), nil
}

type recoverySchedulerStub struct {
	taskIDs []string
	err     error
	failAt  string
}

func (s *recoverySchedulerStub) Enqueue(taskID string) error {
	if s.err != nil {
		return s.err
	}
	if s.failAt == taskID {
		return errors.New("enqueue failed")
	}
	s.taskIDs = append(s.taskIDs, taskID)
	return nil
}

func TestRecoverRequeuesRecoverableTasksInOrder(t *testing.T) {
	service := NewService(
		&recoveryTaskRepositoryStub{tasks: []domaintask.Task{
			mustRecoveryTask(t, "task-1", domaintask.StatusQueued),
			mustRecoveryTask(t, "task-2", domaintask.StatusTranslating),
			mustRecoveryTask(t, "task-3", domaintask.StatusDrawing),
		}},
		&recoverySchedulerStub{},
	)

	result, err := service.Recover(context.Background(), RecoverCommand{})
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(result.RequeuedTaskIDs) != 3 {
		t.Fatalf("unexpected requeued count: %d", len(result.RequeuedTaskIDs))
	}
	if result.RequeuedTaskIDs[0] != "task-1" || result.RequeuedTaskIDs[1] != "task-2" || result.RequeuedTaskIDs[2] != "task-3" {
		t.Fatalf("unexpected requeued ids: %#v", result.RequeuedTaskIDs)
	}
}

func TestRecoverReturnsRepositoryError(t *testing.T) {
	repositoryErr := errors.New("list failed")
	service := NewService(
		&recoveryTaskRepositoryStub{err: repositoryErr},
		&recoverySchedulerStub{},
	)

	_, err := service.Recover(context.Background(), RecoverCommand{})
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestRecoverReturnsPartialResultWhenSchedulerFails(t *testing.T) {
	service := NewService(
		&recoveryTaskRepositoryStub{tasks: []domaintask.Task{
			mustRecoveryTask(t, "task-1", domaintask.StatusQueued),
			mustRecoveryTask(t, "task-2", domaintask.StatusTranslating),
		}},
		&recoverySchedulerStub{failAt: "task-2"},
	)

	result, err := service.Recover(context.Background(), RecoverCommand{})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(result.RequeuedTaskIDs) != 1 || result.RequeuedTaskIDs[0] != "task-1" {
		t.Fatalf("unexpected partial result: %#v", result.RequeuedTaskIDs)
	}
}

func TestRecoverRequiresTaskRepository(t *testing.T) {
	service := NewService(nil, &recoverySchedulerStub{})

	_, err := service.Recover(context.Background(), RecoverCommand{})
	if !errors.Is(err, ErrTaskRepositoryRequired) {
		t.Fatalf("expected task repository required error, got %v", err)
	}
}

func TestRecoverRequiresScheduler(t *testing.T) {
	service := NewService(&recoveryTaskRepositoryStub{}, nil)

	_, err := service.Recover(context.Background(), RecoverCommand{})
	if !errors.Is(err, ErrSchedulerRequired) {
		t.Fatalf("expected scheduler required error, got %v", err)
	}
}

func mustRecoveryTask(t *testing.T, id string, status domaintask.Status) domaintask.Task {
	t.Helper()

	contextSnapshot, err := domaintask.NewContext(`{"version":1,"shape":"square"}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(id, "user-1", "session-1", "draw a moonlit girl", contextSnapshot, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	switch status {
	case domaintask.StatusQueued:
		return task
	case domaintask.StatusTranslating:
		if err := task.MarkTranslating(time.Unix(2, 0).UTC()); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		return task
	case domaintask.StatusDrawing:
		if err := task.MarkTranslating(time.Unix(2, 0).UTC()); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		if err := task.SetPrompt("masterpiece, moonlit_girl"); err != nil {
			t.Fatalf("set prompt: %v", err)
		}
		if err := task.MarkDrawing(time.Unix(3, 0).UTC()); err != nil {
			t.Fatalf("mark drawing: %v", err)
		}
		return task
	default:
		t.Fatalf("unsupported status: %s", status)
		return domaintask.Task{}
	}
}
