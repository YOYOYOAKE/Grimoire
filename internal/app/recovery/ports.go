package recovery

import (
	"context"

	domaintask "grimoire/internal/domain/task"
)

type TaskRepository interface {
	ListRecoverable(ctx context.Context) ([]domaintask.Task, error)
}

type Scheduler interface {
	Enqueue(taskID string) error
}
