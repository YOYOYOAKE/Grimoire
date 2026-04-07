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
	if s.txRunner == nil {
		return domaintask.Task{}, ErrTxRunnerRequired
	}
	if s.schedule == nil {
		return domaintask.Task{}, ErrSchedulerRequired
	}

	var created domaintask.Task
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		task, err := domaintask.New(
			s.idGenerator(),
			command.UserID,
			command.SessionID,
			command.Request,
			command.Context,
			s.now(),
		)
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

func (s *Service) GetPrompt(ctx context.Context, taskID string) (string, error) {
	task, err := s.Get(ctx, taskID)
	if err != nil {
		return "", err
	}
	return task.Prompt, nil
}
