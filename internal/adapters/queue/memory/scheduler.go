package memory

import (
	"fmt"
	"strings"
)

type Scheduler struct {
	worker *Worker
}

func NewScheduler(worker *Worker) *Scheduler {
	return &Scheduler{worker: worker}
}

func (s *Scheduler) Enqueue(taskID string) error {
	if s == nil || s.worker == nil {
		return fmt.Errorf("memory scheduler requires a worker")
	}

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task id is required")
	}
	s.worker.Enqueue(taskID)
	return nil
}
