package memory

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

type HandlerFunc func(ctx context.Context, taskID string)

type Worker struct {
	jobs        chan string
	handler     HandlerFunc
	logger      *slog.Logger
	concurrency int
	pending     atomic.Int64
	startOnce   sync.Once
}

func NewWorker(concurrency int, handler HandlerFunc, logger *slog.Logger) *Worker {
	// NAI requests must stay serialized, so the in-memory worker keeps a single consumer
	// even if callers accidentally pass a higher concurrency.
	concurrency = 1
	worker := &Worker{
		jobs:        make(chan string, 1024),
		handler:     handler,
		logger:      logger,
		concurrency: concurrency,
	}
	return worker
}

func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		for idx := 0; idx < w.concurrency; idx++ {
			go w.loop(ctx, idx)
		}
	})
}

func (w *Worker) Enqueue(taskID string) int {
	w.pending.Add(1)
	position := int(w.pending.Load())
	w.jobs <- taskID
	return position
}

func (w *Worker) loop(ctx context.Context, index int) {
	for {
		select {
		case <-ctx.Done():
			return
		case taskID := <-w.jobs:
			w.pending.Add(-1)
			if w.logger != nil {
				w.logger.Info("task dequeued", "worker", index, "task_id", taskID)
			}
			w.handler(ctx, taskID)
		}
	}
}
