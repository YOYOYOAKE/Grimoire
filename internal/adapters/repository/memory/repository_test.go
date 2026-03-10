package memory

import (
	"context"
	"testing"
	"time"

	"grimoire/internal/domain/draw"
)

func TestTaskRepositoryCRUD(t *testing.T) {
	repo := NewTaskRepository()
	task, err := draw.NewTask("task-1", 1, 3, "hello", draw.ShapeSquare, "", time.Now())
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := repo.Create(context.Background(), task); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("unexpected task id: %s", got.ID)
	}
	if err := repo.Delete(context.Background(), task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
