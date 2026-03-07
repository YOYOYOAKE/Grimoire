package preferences

import (
	"strings"
	"time"

	"grimoire/internal/domain/draw"
)

type UserPreference struct {
	UserID       int64
	DefaultShape draw.Shape
	Artist       string
	UpdatedAt    time.Time
}

func NewUserPreference(userID int64, now time.Time) UserPreference {
	if now.IsZero() {
		now = time.Now()
	}
	return UserPreference{
		UserID:       userID,
		DefaultShape: draw.ShapeSquare,
		UpdatedAt:    now,
	}
}

func (p *UserPreference) SetShape(shape draw.Shape, now time.Time) {
	p.DefaultShape = shape
	p.touch(now)
}

func (p *UserPreference) SetArtist(artist string, now time.Time) {
	p.Artist = strings.TrimSpace(artist)
	p.touch(now)
}

func (p *UserPreference) ClearArtist(now time.Time) {
	p.Artist = ""
	p.touch(now)
}

func (p *UserPreference) touch(now time.Time) {
	if now.IsZero() {
		now = time.Now()
	}
	p.UpdatedAt = now
}
