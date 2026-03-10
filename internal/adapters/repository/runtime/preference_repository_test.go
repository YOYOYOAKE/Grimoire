package runtime

import (
	"os"
	"path/filepath"
	"sync"
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
	if preference.Shape != draw.ShapeSquare {
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

func TestPreferenceRepositoryConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewPreferenceRepository(func() (string, error) {
		return filepath.Join(dir, "grimoire-bot"), nil
	})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	var wg sync.WaitGroup
	for idx := 0; idx < 16; idx++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			preference, err := repo.Get()
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			if i%2 == 0 {
				preference.SetShape(draw.ShapeLandscape)
			} else {
				preference.SetShape(draw.ShapeSquare)
			}
			preference.SetArtists("artist:foo")
			if err := repo.Save(preference); err != nil {
				t.Errorf("save: %v", err)
			}
		}(idx)
	}
	wg.Wait()
}
