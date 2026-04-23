package preferences

import (
	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type GetCommand struct {
	UserID string
}

type UpdateShapeCommand struct {
	UserID string
	Shape  draw.Shape
}

type UpdateArtistsCommand struct {
	UserID  string
	Artists string
}

type ClearArtistsCommand struct {
	UserID string
}

type UpdateModeCommand struct {
	UserID string
	Mode   domainpreferences.Mode
}
