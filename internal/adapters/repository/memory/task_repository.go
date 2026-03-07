package memory

import (
	"context"
	"sync"

	drawapp "grimoire/internal/app/draw"
	domaindraw "grimoire/internal/domain/draw"
)

type TaskRepository struct {
	mu    sync.RWMutex
	tasks map[string]domaindraw.Task
}

func NewTaskRepository() *TaskRepository {
	return &TaskRepository{tasks: make(map[string]domaindraw.Task)}
}

func (r *TaskRepository) Create(_ context.Context, task domaindraw.Task) error {
	r.mu.Lock()
	r.tasks[task.ID] = task
	r.mu.Unlock()
	return nil
}

func (r *TaskRepository) Get(_ context.Context, taskID string) (domaindraw.Task, error) {
	r.mu.RLock()
	task, ok := r.tasks[taskID]
	r.mu.RUnlock()
	if !ok {
		return domaindraw.Task{}, drawapp.ErrTaskNotFound
	}
	return task, nil
}

func (r *TaskRepository) Update(_ context.Context, task domaindraw.Task) error {
	r.mu.Lock()
	r.tasks[task.ID] = task
	r.mu.Unlock()
	return nil
}

func (r *TaskRepository) Delete(_ context.Context, taskID string) error {
	r.mu.Lock()
	delete(r.tasks, taskID)
	r.mu.Unlock()
	return nil
}
