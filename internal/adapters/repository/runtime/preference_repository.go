package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type PreferenceRepository struct {
	mu         sync.RWMutex
	path       string
	preference domainpreferences.Preference
}

type preferenceFile struct {
	Shape   string `json:"shape"`
	Artists string `json:"artists"`
}

func NewPreferenceRepository(executablePath func() (string, error)) (*PreferenceRepository, error) {
	path, err := resolveRuntimePath(executablePath)
	if err != nil {
		return nil, err
	}

	preference, err := loadPreference(path)
	if err != nil {
		return nil, err
	}

	return &PreferenceRepository{
		path:       path,
		preference: preference,
	}, nil
}

func (r *PreferenceRepository) Get() (domainpreferences.Preference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.preference, nil
}

func (r *PreferenceRepository) Save(preference domainpreferences.Preference) error {
	if !preference.Shape.Valid() {
		return fmt.Errorf("invalid runtime shape %q", preference.Shape)
	}

	preference.SetArtists(preference.Artists)
	filePreference := preferenceFile{
		Shape:   string(preference.Shape),
		Artists: preference.Artists,
	}

	data, err := json.Marshal(filePreference)
	if err != nil {
		return fmt.Errorf("marshal runtime %s: %w", r.path, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := writeAtomically(r.path, data); err != nil {
		return err
	}

	r.preference = preference
	return nil
}

func resolveRuntimePath(executablePath func() (string, error)) (string, error) {
	if executablePath == nil {
		executablePath = os.Executable
	}

	executable, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), "runtime.json"), nil
}

func loadPreference(path string) (domainpreferences.Preference, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domainpreferences.DefaultPreference(), nil
		}
		return domainpreferences.Preference{}, fmt.Errorf("read runtime %s: %w", path, err)
	}

	var filePreference preferenceFile
	if err := json.Unmarshal(data, &filePreference); err != nil {
		return domainpreferences.Preference{}, fmt.Errorf("decode runtime %s: %w", path, err)
	}

	shape := domaindraw.Shape(strings.TrimSpace(filePreference.Shape))
	if !shape.Valid() {
		return domainpreferences.Preference{}, fmt.Errorf("invalid runtime shape %q in %s", filePreference.Shape, path)
	}

	preference := domainpreferences.DefaultPreference()
	preference.SetShape(shape)
	preference.SetArtists(filePreference.Artists)
	return preference, nil
}

func writeAtomically(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "runtime.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create runtime temp file for %s: %w", path, err)
	}

	tempPath := tempFile.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err = tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write runtime temp file for %s: %w", path, err)
	}
	if err = tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod runtime temp file for %s: %w", path, err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("close runtime temp file for %s: %w", path, err)
	}
	if err = os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace runtime %s: %w", path, err)
	}
	return nil
}
