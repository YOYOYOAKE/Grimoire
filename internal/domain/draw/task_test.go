package draw

import (
	"testing"
	"time"
)

func TestTaskLifecycle(t *testing.T) {
	now := time.Now()
	task, err := NewTask("task-1", 1, 3, "hello", ShapeSquare, "", now)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := task.MarkTranslating(now); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := task.MarkGenerating(now); err != nil {
		t.Fatalf("mark generating: %v", err)
	}
	if err := task.MarkCompleted(now); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	if task.Status != StatusCompleted {
		t.Fatalf("unexpected status: %s", task.Status)
	}
}

func TestTaskRejectsDuplicateTerminalTransition(t *testing.T) {
	now := time.Now()
	task, err := NewTask("task-1", 1, 3, "hello", ShapeSquare, "", now)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if err := task.MarkFailed("x", now); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if err := task.MarkFailed("y", now); err == nil {
		t.Fatal("expected error")
	}
}
