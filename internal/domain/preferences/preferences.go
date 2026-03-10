package preferences

import (
	"strings"

	"grimoire/internal/domain/draw"
)

type Preference struct {
	Shape   draw.Shape
	Artists string
}

func DefaultPreference() Preference {
	return Preference{
		Shape:   draw.ShapeSquare,
		Artists: "",
	}
}

func (p *Preference) SetShape(shape draw.Shape) {
	p.Shape = shape
}

func (p *Preference) SetArtists(artists string) {
	p.Artists = strings.TrimSpace(artists)
}

func (p *Preference) ClearArtists() {
	p.Artists = ""
}
