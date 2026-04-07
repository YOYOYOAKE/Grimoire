package preferences

import (
	"testing"

	domaindraw "grimoire/internal/domain/draw"
)

func TestNewRejectsInvalidShape(t *testing.T) {
	if _, err := New(domaindraw.Shape("invalid"), "artist:foo"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSetShapeRejectsInvalidShape(t *testing.T) {
	preference := DefaultPreference()

	if err := preference.SetShape(domaindraw.Shape("invalid")); err == nil {
		t.Fatal("expected error")
	}
	if preference.Shape != domaindraw.ShapeSmallSquare {
		t.Fatalf("unexpected shape after failed update: %q", preference.Shape)
	}
}

func TestSetArtistsTrimsWhitespace(t *testing.T) {
	preference := DefaultPreference()
	preference.SetArtists(" artist:foo ")

	if preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", preference.Artists)
	}
}

func TestValidateAcceptsDefaultPreference(t *testing.T) {
	if err := DefaultPreference().Validate(); err != nil {
		t.Fatalf("validate default preference: %v", err)
	}
}
