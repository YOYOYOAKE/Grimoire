package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"grimoire/internal/domain/draw"
	"grimoire/internal/domain/preferences"
)

func TestTaskRepositoryCRUD(t *testing.T) {
	repo := NewTaskRepository()
	task, err := draw.NewTask("task-1", 1, 2, 3, "hello", draw.ShapeSquare, "", time.Now())
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

func TestPreferenceRepositoryConcurrentAccess(t *testing.T) {
	repo := NewPreferenceRepository()
	ctx := context.Background()
	var wg sync.WaitGroup
	for idx := 0; idx < 16; idx++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = repo.Save(ctx, preferences.UserPreference{
				UserID:       int64(i),
				DefaultShape: draw.ShapeSquare,
				UpdatedAt:    time.Now(),
			})
		}(idx)
	}
	wg.Wait()
}
