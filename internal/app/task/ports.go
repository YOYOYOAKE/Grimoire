package task

import (
	"context"

	domaintask "grimoire/internal/domain/task"
)

type TaskRepository interface {
	Create(ctx context.Context, task domaintask.Task) error
	Get(ctx context.Context, id string) (domaintask.Task, error)
	Update(ctx context.Context, task domaintask.Task) error
	ListRecoverable(ctx context.Context) ([]domaintask.Task, error)
	ListBySourceTask(ctx context.Context, sourceTaskID string) ([]domaintask.Task, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
