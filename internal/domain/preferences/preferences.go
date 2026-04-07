package preferences

import (
	"fmt"
	"strings"

	"grimoire/internal/domain/draw"
)

type Preference struct {
	Shape   draw.Shape
	Artists string
}

func New(shape draw.Shape, artists string) (Preference, error) {
	preference := Preference{
		Shape:   shape,
		Artists: strings.TrimSpace(artists),
	}
	if err := preference.Validate(); err != nil {
		return Preference{}, err
	}
	return preference, nil
}

func DefaultPreference() Preference {
	return Preference{
		Shape:   draw.ShapeSmallSquare,
		Artists: "",
	}
}

func (p Preference) Validate() error {
	if !p.Shape.Valid() {
		return fmt.Errorf("invalid shape %q", p.Shape)
	}
	return nil
}

func (p *Preference) SetShape(shape draw.Shape) error {
	if !shape.Valid() {
		return fmt.Errorf("invalid shape %q", shape)
	}
	p.Shape = shape
	return nil
}

func (p *Preference) SetArtists(artists string) {
	p.Artists = strings.TrimSpace(artists)
}

func (p *Preference) ClearArtists() {
	p.Artists = ""
}
