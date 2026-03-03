package queue

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"grimoire/internal/types"
)

func TestWorkerSingleConcurrencyOrder(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		mu      sync.Mutex
		order   []string
		wg      sync.WaitGroup
		taskIDs = []string{"a", "b", "c"}
	)

	wg.Add(len(taskIDs))
	w := NewWorker(1, func(_ context.Context, task types.DrawTask) {
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		order = append(order, task.TaskID)
		mu.Unlock()
		wg.Done()
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w.Start(ctx)
	for _, id := range taskIDs {
		w.Enqueue(types.DrawTask{TaskID: id})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting worker to finish")
	}

	if !reflect.DeepEqual(order, taskIDs) {
		t.Fatalf("unexpected order: got=%v want=%v", order, taskIDs)
	}
}
