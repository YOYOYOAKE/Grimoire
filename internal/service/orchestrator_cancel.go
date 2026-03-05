package service

import "context"

func (o *Orchestrator) registerCancel(taskID string, cancel context.CancelFunc) {
	o.mu.Lock()
	o.activeCancels[taskID] = cancel
	o.mu.Unlock()
}

func (o *Orchestrator) unregisterCancel(taskID string) {
	o.mu.Lock()
	delete(o.activeCancels, taskID)
	delete(o.pendingCancels, taskID)
	o.mu.Unlock()
}

func (o *Orchestrator) consumePendingCancel(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.pendingCancels[taskID]
	if ok {
		delete(o.pendingCancels, taskID)
	}
	return ok
}
