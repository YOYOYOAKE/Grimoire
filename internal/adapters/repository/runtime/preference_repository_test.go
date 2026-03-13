package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

func TestNewPreferenceRepositoryUsesDefaultWhenRuntimeMissing(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference, err := repo.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if preference.Shape != draw.ShapeSmallSquare {
		t.Fatalf("unexpected shape: %s", preference.Shape)
	}
	if preference.Artists != "" {
		t.Fatalf("unexpected artists: %q", preference.Artists)
	}
}

func TestNewPreferenceRepositoryRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewPreferenceRepositoryRejectsInvalidShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.json")
	if err := os.WriteFile(path, []byte(`{"shape":"invalid","artists":""}`), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveCreatesRuntimeFile(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	preference.SetShape(draw.ShapePortrait)
	preference.SetArtists(" artist:foo ")
	if err := repo.Save(preference); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "runtime.json"))
	if err != nil {
		t.Fatalf("read runtime: %v", err)
	}
	if string(data) != `{"shape":"portrait","artists":"artist:foo"}` {
		t.Fatalf("unexpected runtime content: %s", string(data))
	}
}

func TestGetFreshReadsExternalRuntimeChanges(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	preference.SetShape(draw.ShapeLandscape)
	preference.SetArtists("artist:foo")
	if err := repo.Save(preference); err != nil {
		t.Fatalf("save: %v", err)
	}

	path := filepath.Join(dir, "runtime.json")
	if err := os.WriteFile(path, []byte(`{"shape":"square","artists":"artist:bar"}`), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	got, err := repo.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Shape != draw.ShapeSquare {
		t.Fatalf("unexpected shape: %s", got.Shape)
	}
	if got.Artists != "artist:bar" {
		t.Fatalf("unexpected artists: %q", got.Artists)
	}
}
