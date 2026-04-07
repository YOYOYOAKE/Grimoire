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
	configPath := filepath.Join(dir, "config", "config.yaml")
	repo, err := NewPreferenceRepository(configPath)
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
	configPath := filepath.Join(dir, "config", "config.yaml")
	path := filepath.Join(dir, "config", "runtime.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, err := NewPreferenceRepository(configPath)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewPreferenceRepositoryRejectsInvalidShape(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.yaml")
	path := filepath.Join(dir, "config", "runtime.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"shape":"invalid","artists":""}`), 0o644); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, err := NewPreferenceRepository(configPath)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveCreatesRuntimeFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.yaml")
	repo, err := NewPreferenceRepository(configPath)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	if err := preference.SetShape(draw.ShapePortrait); err != nil {
		t.Fatalf("set shape: %v", err)
	}
	preference.SetArtists(" artist:foo ")
	if err := repo.Save(preference); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "config", "runtime.json"))
	if err != nil {
		t.Fatalf("read runtime: %v", err)
	}
	if string(data) != `{"shape":"portrait","artists":"artist:foo"}` {
		t.Fatalf("unexpected runtime content: %s", string(data))
	}
}

func TestGetFreshReadsExternalRuntimeChanges(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.yaml")
	repo, err := NewPreferenceRepository(configPath)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	if err := preference.SetShape(draw.ShapeLandscape); err != nil {
		t.Fatalf("set shape: %v", err)
	}
	preference.SetArtists("artist:foo")
	if err := repo.Save(preference); err != nil {
		t.Fatalf("save: %v", err)
	}

	path := filepath.Join(dir, "config", "runtime.json")
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

func TestResolveRuntimePathUsesConfigDir(t *testing.T) {
	path, err := resolveRuntimePath("/tmp/grimoire/config/config.yaml")
	if err != nil {
		t.Fatalf("resolve runtime path: %v", err)
	}
	expected := filepath.Join("/tmp/grimoire/config", "runtime.json")
	if path != expected {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestResolveRuntimePathRejectsEmptyConfigPath(t *testing.T) {
	if _, err := resolveRuntimePath(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveRuntimePathUsesExplicitConfigDir(t *testing.T) {
	path, err := resolveRuntimePath("/tmp/custom/custom.yaml")
	if err != nil {
		t.Fatalf("resolve runtime path: %v", err)
	}
	expected := filepath.Join("/tmp/custom", "runtime.json")
	if path != expected {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestSaveRejectsInvalidPreference(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.yaml")
	repo, err := NewPreferenceRepository(configPath)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	preference.Shape = draw.Shape("invalid")

	if err := repo.Save(preference); err == nil {
		t.Fatal("expected error")
	}
}
