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

func TestNewTrimsArtists(t *testing.T) {
	preference, err := New(domaindraw.ShapePortrait, " artist:foo ")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	if preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", preference.Artists)
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

func TestClearArtistsResetsArtists(t *testing.T) {
	preference := DefaultPreference()
	preference.SetArtists("artist:foo")

	preference.ClearArtists()

	if preference.Artists != "" {
		t.Fatalf("unexpected artists after clear: %q", preference.Artists)
	}
}

func TestValidateAcceptsDefaultPreference(t *testing.T) {
	if err := DefaultPreference().Validate(); err != nil {
		t.Fatalf("validate default preference: %v", err)
	}
}

func TestDefaultPreferenceUsesExpertMode(t *testing.T) {
	if DefaultPreference().Mode != ModeExpert {
		t.Fatalf("expected expert mode by default, got %q", DefaultPreference().Mode)
	}
}

func TestSetModeRejectsInvalidMode(t *testing.T) {
	preference := DefaultPreference()

	if err := preference.SetMode(Mode("invalid")); err == nil {
		t.Fatal("expected error")
	}
	if preference.Mode != ModeExpert {
		t.Fatalf("unexpected mode after failed update: %q", preference.Mode)
	}
}

func TestSetModeUpdatesMode(t *testing.T) {
	preference := DefaultPreference()

	if err := preference.SetMode(ModeFast); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if preference.Mode != ModeFast {
		t.Fatalf("unexpected mode: %q", preference.Mode)
	}
}
