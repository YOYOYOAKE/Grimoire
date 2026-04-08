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
