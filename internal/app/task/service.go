package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domaintask "grimoire/internal/domain/task"
)

var (
	ErrTxRunnerRequired  = errors.New("tx runner is required")
	ErrSchedulerRequired = errors.New("scheduler is required")
	ErrTaskAccessDenied  = errors.New("task does not belong to user")
)

type Service struct {
	tasks       TaskRepository
	txRunner    TxRunner
	schedule    Scheduler
	now         func() time.Time
	idGenerator func() string
}

func NewService(
	tasks TaskRepository,
	txRunner TxRunner,
	scheduler Scheduler,
	now func() time.Time,
	idGenerator func() string,
) *Service {
	if now == nil {
		now = time.Now
	}
	if idGenerator == nil {
		idGenerator = func() string {
			return fmt.Sprintf("task-%d", now().UnixNano())
		}
	}
	return &Service{
		tasks:       tasks,
		txRunner:    txRunner,
		schedule:    scheduler,
		now:         now,
		idGenerator: idGenerator,
	}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domaintask.Task, error) {
	return s.createAndEnqueue(ctx, func(context.Context) (domaintask.Task, error) {
		return domaintask.New(
			s.idGenerator(),
			command.UserID,
			command.SessionID,
			command.Request,
			command.Context,
			s.now(),
		)
	})
}

func (s *Service) Stop(ctx context.Context, command StopCommand) (domaintask.Task, error) {
	if s.txRunner == nil {
		return domaintask.Task{}, ErrTxRunnerRequired
	}

	var stopped domaintask.Task
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		task, err := s.loadOwnedTask(txCtx, command.TaskID, command.UserID)
		if err != nil {
			return err
		}
		if task.Status == domaintask.StatusStopped {
			stopped = task
			return nil
		}
		if err := task.MarkStopped(s.now()); err != nil {
			return err
		}
		if err := s.tasks.Update(txCtx, task); err != nil {
			return err
		}
		stopped = task
		return nil
	})
	if err != nil {
		return domaintask.Task{}, err
	}
	return stopped, nil
}

func (s *Service) RetryTranslate(ctx context.Context, command RetryCommand) (domaintask.Task, error) {
	return s.retry(ctx, command, false)
}

func (s *Service) RetryDraw(ctx context.Context, command RetryCommand) (domaintask.Task, error) {
	return s.retry(ctx, command, true)
}

func (s *Service) retry(ctx context.Context, command RetryCommand, reusePrompt bool) (domaintask.Task, error) {
	return s.createAndEnqueue(ctx, func(txCtx context.Context) (domaintask.Task, error) {
		source, err := s.loadOwnedTask(txCtx, command.TaskID, command.UserID)
		if err != nil {
			return domaintask.Task{}, err
		}

		task, err := domaintask.New(
			s.idGenerator(),
			source.UserID,
			source.SessionID,
			source.Request,
			source.Context,
			s.now(),
		)
		if err != nil {
			return domaintask.Task{}, err
		}
		if err := task.SetSourceTask(source.ID); err != nil {
			return domaintask.Task{}, err
		}
		if reusePrompt {
			if err := task.SetPrompt(source.Prompt); err != nil {
				return domaintask.Task{}, err
			}
		}
		return task, nil
	})
}

func (s *Service) createAndEnqueue(
	ctx context.Context,
	build func(txCtx context.Context) (domaintask.Task, error),
) (domaintask.Task, error) {
	if s.txRunner == nil {
		return domaintask.Task{}, ErrTxRunnerRequired
	}
	if s.schedule == nil {
		return domaintask.Task{}, ErrSchedulerRequired
	}

	var created domaintask.Task
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		task, err := build(txCtx)
		if err != nil {
			return err
		}
		if err := s.tasks.Create(txCtx, task); err != nil {
			return err
		}
		created = task
		return nil
	})
	if err != nil {
		return domaintask.Task{}, err
	}

	if err := s.schedule.Enqueue(created.ID); err != nil {
		return created, fmt.Errorf("enqueue task %s: %w", created.ID, err)
	}
	return created, nil
}

func (s *Service) Get(ctx context.Context, taskID string) (domaintask.Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return domaintask.Task{}, fmt.Errorf("task id is required")
	}
	return s.tasks.Get(ctx, taskID)
}

func (s *Service) GetPrompt(ctx context.Context, command GetPromptCommand) (string, error) {
	task, err := s.loadOwnedTask(ctx, command.TaskID, command.UserID)
	if err != nil {
		return "", err
	}
	return task.Prompt, nil
}

func (s *Service) loadOwnedTask(ctx context.Context, taskID string, userID string) (domaintask.Task, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domaintask.Task{}, fmt.Errorf("user id is required")
	}
	task, err := s.Get(ctx, taskID)
	if err != nil {
		return domaintask.Task{}, err
	}
	if task.UserID != userID {
		return domaintask.Task{}, ErrTaskAccessDenied
	}
	return task, nil
}
