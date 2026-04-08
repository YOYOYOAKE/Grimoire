package task

import (
	domaindraw "grimoire/internal/domain/draw"
	domaintask "grimoire/internal/domain/task"
)

type CreateCommand struct {
	UserID    string
	SessionID string
	Request   string
	Context   domaintask.Context
}

type StopCommand struct {
	TaskID string
	UserID string
}

type RetryCommand struct {
	TaskID string
	UserID string
}

type GetPromptCommand struct {
	TaskID string
	UserID string
}

type PromptDetails struct {
	Prompt         string
	NegativePrompt string
	Characters     []domaindraw.CharacterPrompt
}

type TaskView struct {
	TaskID string
	Status domaintask.Status
}
