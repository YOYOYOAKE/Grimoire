package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	logger      *slog.Logger
}

func NewService(
	tasks TaskRepository,
	txRunner TxRunner,
	scheduler Scheduler,
	now func() time.Time,
	idGenerator func() string,
	logger *slog.Logger,
) *Service {
	if now == nil {
		now = time.Now
	}
	if idGenerator == nil {
		idGenerator = func() string {
			return fmt.Sprintf("task-%d", now().UnixNano())
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		tasks:       tasks,
		txRunner:    txRunner,
		schedule:    scheduler,
		now:         now,
		idGenerator: idGenerator,
		logger:      logger,
	}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (domaintask.Task, error) {
	s.logger.Info(
		"task create requested",
		"user_id", command.UserID,
		"session_id", command.SessionID,
		"request", command.Request,
		"task_context", command.Context.Raw(),
	)
	return s.createAndEnqueue(ctx, "create", func(context.Context) (domaintask.Task, error) {
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
	s.logger.Info(
		"task stop requested",
		"task_id", command.TaskID,
		"user_id", command.UserID,
	)

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
	s.logger.Info("task stopped", taskLogAttrs(stopped)...)
	return stopped, nil
}

func (s *Service) RetryTranslate(ctx context.Context, command RetryCommand) (domaintask.Task, error) {
	return s.retry(ctx, command, false)
}

func (s *Service) RetryDraw(ctx context.Context, command RetryCommand) (domaintask.Task, error) {
	return s.retry(ctx, command, true)
}

func (s *Service) retry(ctx context.Context, command RetryCommand, reusePrompt bool) (domaintask.Task, error) {
	operation := "retry_translate"
	if reusePrompt {
		operation = "retry_draw"
	}
	s.logger.Info(
		"task retry requested",
		"operation", operation,
		"task_id", command.TaskID,
		"user_id", command.UserID,
		"reuse_prompt", reusePrompt,
	)
	return s.createAndEnqueue(ctx, operation, func(txCtx context.Context) (domaintask.Task, error) {
		source, err := s.loadOwnedTask(txCtx, command.TaskID, command.UserID)
		if err != nil {
			return domaintask.Task{}, err
		}
		s.logger.Info(
			"task retry source loaded",
			"operation", operation,
			"source_task_id", source.ID,
			"user_id", source.UserID,
			"session_id", source.SessionID,
			"request", source.Request,
			"task_context", source.Context.Raw(),
			"prompt", source.Prompt,
			"reuse_prompt", reusePrompt,
		)

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
	operation string,
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
	attrs := append([]any{"operation", operation}, taskLogAttrs(created)...)
	s.logger.Info("task persisted", attrs...)

	if err := s.schedule.Enqueue(created.ID); err != nil {
		s.logger.Error("task enqueue failed", append(attrs, "error", err)...)
		return created, fmt.Errorf("enqueue task %s: %w", created.ID, err)
	}
	s.logger.Info("task enqueued", attrs...)
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

func taskLogAttrs(task domaintask.Task) []any {
	return []any{
		"task_id", task.ID,
		"user_id", task.UserID,
		"session_id", task.SessionID,
		"source_task_id", task.SourceTaskID,
		"request", task.Request,
		"prompt", task.Prompt,
		"status", task.Status,
		"task_context", task.Context.Raw(),
	}
}
