package preferences

import (
	"fmt"
	"strings"

	"grimoire/internal/domain/draw"
)

type Mode string

const (
	ModeExpert Mode = "expert"
	ModeFast   Mode = "fast"
)

type Preference struct {
	Shape   draw.Shape
	Artists string
	Mode    Mode
}

func New(shape draw.Shape, artists string) (Preference, error) {
	preference := Preference{
		Shape:   shape,
		Artists: strings.TrimSpace(artists),
		Mode:    ModeExpert,
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
		Mode:    ModeExpert,
	}
}

func (p Preference) Validate() error {
	if !p.Shape.Valid() {
		return fmt.Errorf("invalid shape %q", p.Shape)
	}
	if !p.Mode.Valid() {
		return fmt.Errorf("invalid mode %q", p.Mode)
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

func (m Mode) Valid() bool {
	switch m {
	case ModeExpert, ModeFast:
		return true
	default:
		return false
	}
}

func (p *Preference) SetMode(mode Mode) error {
	if !mode.Valid() {
		return fmt.Errorf("invalid mode %q", mode)
	}
	p.Mode = mode
	return nil
}
