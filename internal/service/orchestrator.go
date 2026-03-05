package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/store"
	"grimoire/internal/types"
)

type Orchestrator struct {
	translator types.Translator
	generator  types.ImageGenerator
	notifier   types.Notifier
	cfg        *config.Manager
	taskStore  store.TaskStore
	logger     *slog.Logger

	processingWarningAfter time.Duration
	pollIntervalOverride   time.Duration

	mu             sync.Mutex
	activeCancels  map[string]context.CancelFunc
	pendingCancels map[string]struct{}
}

func NewOrchestrator(
	translator types.Translator,
	generator types.ImageGenerator,
	notifier types.Notifier,
	cfg *config.Manager,
	taskStore store.TaskStore,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		translator:             translator,
		generator:              generator,
		notifier:               notifier,
		cfg:                    cfg,
		taskStore:              taskStore,
		logger:                 logger,
		processingWarningAfter: 3 * time.Minute,
		activeCancels:          make(map[string]context.CancelFunc),
		pendingCancels:         make(map[string]struct{}),
	}
}

func (o *Orchestrator) CancelTask(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if cancel, ok := o.activeCancels[taskID]; ok {
		cancel()
		return true
	}
	o.pendingCancels[taskID] = struct{}{}
	return true
}
