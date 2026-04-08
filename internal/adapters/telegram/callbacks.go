package telegram

import (
	"strings"

	domaindraw "grimoire/internal/domain/draw"
)

const (
	requestShapePrefix  = "request:shape:"
	requestArtistsSet   = "request:artists:set"
	requestArtistsClear = "request:artists:clear"
)

const (
	taskStopPrefix           = "task:stop:"
	taskPromptPrefix         = "task:prompt:"
	taskRetryTranslatePrefix = "task:retry:translate:"
	taskRetryDrawPrefix      = "task:retry:draw:"
)

type requestActionKind string

const (
	requestActionUpdateShape  requestActionKind = "update_shape"
	requestActionSetArtists   requestActionKind = "set_artists"
	requestActionClearArtists requestActionKind = "clear_artists"
)

type requestAction struct {
	Kind  requestActionKind
	Shape domaindraw.Shape
}

type taskActionKind string

const (
	taskActionStop           taskActionKind = "stop"
	taskActionPrompt         taskActionKind = "prompt"
	taskActionRetryTranslate taskActionKind = "retry_translate"
	taskActionRetryDraw      taskActionKind = "retry_draw"
)

type taskAction struct {
	Kind   taskActionKind
	TaskID string
}

func parseRequestAction(data string) (requestAction, bool) {
	data = strings.TrimSpace(data)
	switch {
	case data == requestArtistsSet:
		return requestAction{Kind: requestActionSetArtists}, true
	case data == requestArtistsClear:
		return requestAction{Kind: requestActionClearArtists}, true
	case strings.HasPrefix(data, requestShapePrefix):
		shape := domaindraw.Shape(strings.TrimSpace(strings.TrimPrefix(data, requestShapePrefix)))
		if !shape.Valid() {
			return requestAction{}, false
		}
		return requestAction{Kind: requestActionUpdateShape, Shape: shape}, true
	default:
		return requestAction{}, false
	}
}

func requestShapeCallback(shape domaindraw.Shape) string {
	return requestShapePrefix + string(shape)
}

func parseTaskAction(data string) (taskAction, bool) {
	data = strings.TrimSpace(data)
	switch {
	case strings.HasPrefix(data, taskStopPrefix):
		taskID := strings.TrimSpace(strings.TrimPrefix(data, taskStopPrefix))
		if taskID == "" {
			return taskAction{}, false
		}
		return taskAction{Kind: taskActionStop, TaskID: taskID}, true
	case strings.HasPrefix(data, taskPromptPrefix):
		taskID := strings.TrimSpace(strings.TrimPrefix(data, taskPromptPrefix))
		if taskID == "" {
			return taskAction{}, false
		}
		return taskAction{Kind: taskActionPrompt, TaskID: taskID}, true
	case strings.HasPrefix(data, taskRetryTranslatePrefix):
		taskID := strings.TrimSpace(strings.TrimPrefix(data, taskRetryTranslatePrefix))
		if taskID == "" {
			return taskAction{}, false
		}
		return taskAction{Kind: taskActionRetryTranslate, TaskID: taskID}, true
	case strings.HasPrefix(data, taskRetryDrawPrefix):
		taskID := strings.TrimSpace(strings.TrimPrefix(data, taskRetryDrawPrefix))
		if taskID == "" {
			return taskAction{}, false
		}
		return taskAction{Kind: taskActionRetryDraw, TaskID: taskID}, true
	default:
		return taskAction{}, false
	}
}
