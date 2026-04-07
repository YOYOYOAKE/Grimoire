package runner

import domaintask "grimoire/internal/domain/task"

type RunCommand struct {
	TaskID string
}

type StartDrawingCommand struct {
	TaskID string
	Prompt string
}

type CompleteCommand struct {
	TaskID string
	Image  string
}

type FailCommand struct {
	TaskID string
	Error  domaintask.TaskError
}
