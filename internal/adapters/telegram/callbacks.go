package telegram

import (
	"strings"

	domaindraw "grimoire/internal/domain/draw"
)

const (
	cbShapeSmallPortrait  = "img:shape:small-portrait"
	cbShapeSmallLandscape = "img:shape:small-landscape"
	cbShapeSmallSquare    = "img:shape:small-square"
	cbShapePortrait       = "img:shape:portrait"
	cbShapeLandscape      = "img:shape:landscape"
	cbShapeSquare         = "img:shape:square"
	cbShapeLargePortrait  = "img:shape:large-portrait"
	cbShapeLargeLandscape = "img:shape:large-landscape"
	cbShapeLargeSquare    = "img:shape:large-square"
	cbSetArtists          = "img:artists:set"
	cbClearArtists        = "img:artists:clear"
)

const (
	taskStopPrefix           = "task:stop:"
	taskPromptPrefix         = "task:prompt:"
	taskRetryTranslatePrefix = "task:retry:translate:"
	taskRetryDrawPrefix      = "task:retry:draw:"
)

type callbackActionKind string

const (
	callbackActionUpdateShape  callbackActionKind = "update_shape"
	callbackActionSetArtists   callbackActionKind = "set_artists"
	callbackActionClearArtists callbackActionKind = "clear_artists"
)

type callbackAction struct {
	Kind  callbackActionKind
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

func parseCallbackAction(data string) (callbackAction, bool) {
	switch strings.TrimSpace(data) {
	case cbShapeSmallPortrait:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeSmallPortrait}, true
	case cbShapeSmallLandscape:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeSmallLandscape}, true
	case cbShapeSmallSquare:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeSmallSquare}, true
	case cbShapePortrait:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapePortrait}, true
	case cbShapeLandscape:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeLandscape}, true
	case cbShapeSquare:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeSquare}, true
	case cbShapeLargePortrait:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeLargePortrait}, true
	case cbShapeLargeLandscape:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeLargeLandscape}, true
	case cbShapeLargeSquare:
		return callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeLargeSquare}, true
	case cbSetArtists:
		return callbackAction{Kind: callbackActionSetArtists}, true
	case cbClearArtists:
		return callbackAction{Kind: callbackActionClearArtists}, true
	default:
		return callbackAction{}, false
	}
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
