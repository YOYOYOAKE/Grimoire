package recovery

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrTaskRepositoryRequired = errors.New("task repository is required")
	ErrSchedulerRequired      = errors.New("scheduler is required")
)

type Service struct {
	tasks     TaskRepository
	scheduler Scheduler
}

func NewService(tasks TaskRepository, scheduler Scheduler) *Service {
	return &Service{
		tasks:     tasks,
		scheduler: scheduler,
	}
}

func (s *Service) Recover(ctx context.Context, _ RecoverCommand) (RecoverResult, error) {
	if s.tasks == nil {
		return RecoverResult{}, ErrTaskRepositoryRequired
	}
	if s.scheduler == nil {
		return RecoverResult{}, ErrSchedulerRequired
	}

	tasks, err := s.tasks.ListRecoverable(ctx)
	if err != nil {
		return RecoverResult{}, err
	}

	result := RecoverResult{
		RequeuedTaskIDs: make([]string, 0, len(tasks)),
	}
	for _, task := range tasks {
		if err := s.scheduler.Enqueue(task.ID); err != nil {
			return result, fmt.Errorf("enqueue task %s: %w", task.ID, err)
		}
		result.RequeuedTaskIDs = append(result.RequeuedTaskIDs, task.ID)
	}
	return result, nil
}
