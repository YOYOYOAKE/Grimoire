package memory

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWorkerHandlesTasks(t *testing.T) {
	var mu sync.Mutex
	processed := make([]string, 0, 2)
	done := make(chan struct{}, 2)

	worker := NewWorker(3, func(_ context.Context, taskID string) {
		mu.Lock()
		processed = append(processed, taskID)
		mu.Unlock()
		done <- struct{}{}
	}, nil)
	worker.Start(context.Background())
	worker.Enqueue("task-1")
	worker.Enqueue("task-2")

	timeout := time.After(2 * time.Second)
	for idx := 0; idx < 2; idx++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("timeout waiting for tasks")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(processed) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(processed))
	}
}
