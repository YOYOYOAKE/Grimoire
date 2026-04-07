package memory

import (
	"context"
	"sync"
	"sync/atomic"
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

func TestWorkerForcesSingleConcurrency(t *testing.T) {
	var inFlight atomic.Int64
	var maxInFlight atomic.Int64
	firstStarted := make(chan struct{}, 1)
	secondStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})

	worker := NewWorker(3, func(_ context.Context, taskID string) {
		current := inFlight.Add(1)
		for {
			max := maxInFlight.Load()
			if current <= max || maxInFlight.CompareAndSwap(max, current) {
				break
			}
		}
		defer inFlight.Add(-1)

		switch taskID {
		case "task-1":
			firstStarted <- struct{}{}
			<-releaseFirst
		case "task-2":
			secondStarted <- struct{}{}
		}
	}, nil)
	worker.Start(context.Background())

	worker.Enqueue("task-1")
	worker.Enqueue("task-2")

	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first task")
	}

	select {
	case <-secondStarted:
		t.Fatal("expected second task to wait for the first one")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second task")
	}

	if maxInFlight.Load() != 1 {
		t.Fatalf("expected max in-flight 1, got %d", maxInFlight.Load())
	}
}

func TestSchedulerEnqueueRejectsBlankTaskID(t *testing.T) {
	scheduler := NewScheduler(NewWorker(1, func(context.Context, string) {}, nil))

	err := scheduler.Enqueue(" \t ")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSchedulerEnqueueDispatchesToWorker(t *testing.T) {
	done := make(chan string, 1)
	worker := NewWorker(1, func(_ context.Context, taskID string) {
		done <- taskID
	}, nil)
	worker.Start(context.Background())
	scheduler := NewScheduler(worker)

	if err := scheduler.Enqueue("task-1"); err != nil {
		t.Fatalf("enqueue task: %v", err)
	}

	select {
	case taskID := <-done:
		if taskID != "task-1" {
			t.Fatalf("unexpected task id: %q", taskID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for scheduled task")
	}
}

func TestSchedulerRequiresWorker(t *testing.T) {
	var scheduler *Scheduler
	err := scheduler.Enqueue("task-1")
	if err == nil {
		t.Fatal("expected error")
	}
}
