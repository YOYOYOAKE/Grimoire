package task

import domaintask "grimoire/internal/domain/task"

type CreateCommand struct {
	UserID    string
	SessionID string
	Request   string
	Context   domaintask.Context
}

type TaskView struct {
	TaskID string
	Status domaintask.Status
}
