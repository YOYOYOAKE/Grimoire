package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domaintask "grimoire/internal/domain/task"
)

var ErrTxRunnerRequired = errors.New("tx runner is required")

type Service struct {
	tasks      TaskRepository
	txRunner   TxRunner
	translator PromptTranslator
	generator  ImageGenerator
	imageStore ImageStore
	notifier   Notifier
	now        func() time.Time
}

func NewService(
	tasks TaskRepository,
	txRunner TxRunner,
	translator PromptTranslator,
	generator ImageGenerator,
	imageStore ImageStore,
	notifier Notifier,
	now func() time.Time,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		tasks:      tasks,
		txRunner:   txRunner,
		translator: translator,
		generator:  generator,
		imageStore: imageStore,
		notifier:   notifier,
		now:        now,
	}
}

func (s *Service) Get(ctx context.Context, taskID string) (domaintask.Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return domaintask.Task{}, fmt.Errorf("task id is required")
	}
	return s.tasks.Get(ctx, taskID)
}

func (s *Service) StartTranslating(ctx context.Context, command RunCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkTranslating(s.now())
	})
}

func (s *Service) StartDrawing(ctx context.Context, command StartDrawingCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		if prompt := strings.TrimSpace(command.Prompt); prompt != "" {
			if err := task.SetPrompt(prompt); err != nil {
				return err
			}
		}
		return task.MarkDrawing(s.now())
	})
}

func (s *Service) Complete(ctx context.Context, command CompleteCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkCompleted(command.Image, s.now())
	})
}

func (s *Service) Fail(ctx context.Context, command FailCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkFailed(command.Error, s.now())
	})
}

func (s *Service) updateTask(
	ctx context.Context,
	taskID string,
	update func(task *domaintask.Task) error,
) (domaintask.Task, error) {
	if s.txRunner == nil {
		return domaintask.Task{}, ErrTxRunnerRequired
	}

	var mutated domaintask.Task
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		task, err := s.Get(txCtx, taskID)
		if err != nil {
			return err
		}
		if err := update(&task); err != nil {
			return err
		}
		if err := s.tasks.Update(txCtx, task); err != nil {
			return err
		}
		mutated = task
		return nil
	})
	if err != nil {
		return domaintask.Task{}, err
	}
	return mutated, nil
}
