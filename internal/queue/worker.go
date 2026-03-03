package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"grimoire/internal/types"
)

const defaultQueueBuffer = 1024

type HandlerFunc func(ctx context.Context, task types.DrawTask)

type Worker struct {
	jobs        chan types.DrawTask
	handler     HandlerFunc
	logger      *slog.Logger
	concurrency int

	pending   atomic.Int64
	running   atomic.Int64
	idCounter atomic.Uint64

	mu            sync.RWMutex
	currentTaskID string
	startOnce     sync.Once
}

func NewWorker(concurrency int, handler HandlerFunc, logger *slog.Logger) *Worker {
	if concurrency <= 0 {
		concurrency = 1
	}
	return &Worker{
		jobs:        make(chan types.DrawTask, defaultQueueBuffer),
		handler:     handler,
		logger:      logger,
		concurrency: concurrency,
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.startOnce.Do(func() {
		for i := 0; i < w.concurrency; i++ {
			go w.loop(ctx, i)
		}
	})
}

func (w *Worker) Enqueue(task types.DrawTask) (string, int) {
	if task.TaskID == "" {
		nextID := w.idCounter.Add(1)
		task.TaskID = fmt.Sprintf("task-%06d", nextID)
	}
	w.pending.Add(1)
	queuePos := int(w.pending.Load())
	w.jobs <- task
	w.logger.Info("task enqueued", "task_id", task.TaskID, "queue_pos", queuePos)
	return task.TaskID, queuePos
}

func (w *Worker) Stats() types.QueueStats {
	w.mu.RLock()
	current := w.currentTaskID
	w.mu.RUnlock()
	return types.QueueStats{
		Pending:       int(w.pending.Load()),
		Running:       w.running.Load() > 0,
		CurrentTaskID: current,
	}
}

func (w *Worker) loop(ctx context.Context, workerIndex int) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.jobs:
			w.pending.Add(-1)
			w.running.Add(1)
			w.setCurrentTask(task.TaskID)
			w.logger.Info("task started", "worker", workerIndex, "task_id", task.TaskID)
			w.handler(ctx, task)
			w.logger.Info("task finished", "worker", workerIndex, "task_id", task.TaskID)
			w.running.Add(-1)
			w.setCurrentTask("")
		}
	}
}

func (w *Worker) setCurrentTask(taskID string) {
	w.mu.Lock()
	w.currentTaskID = taskID
	w.mu.Unlock()
}
