package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type PreferenceRepository struct {
	path string
}

type preferenceFile struct {
	Shape   string `json:"shape"`
	Artists string `json:"artists"`
}

func NewPreferenceRepository(configPath string) (*PreferenceRepository, error) {
	path, err := resolveRuntimePath(configPath)
	if err != nil {
		return nil, err
	}
	if _, err := loadPreference(path); err != nil {
		return nil, err
	}
	return &PreferenceRepository{path: path}, nil
}

func (r *PreferenceRepository) Get() (domainpreferences.Preference, error) {
	return loadPreference(r.path)
}

func (r *PreferenceRepository) Save(preference domainpreferences.Preference) error {
	if err := preference.Validate(); err != nil {
		return err
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

	if err := writeAtomically(r.path, data); err != nil {
		return err
	}
	return nil
}

func resolveRuntimePath(configPath string) (string, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", fmt.Errorf("config path is required")
	}

	return filepath.Join(filepath.Dir(filepath.Clean(configPath)), "runtime.json"), nil
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
	if err := preference.SetShape(shape); err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.SetArtists(filePreference.Artists)
	return preference, nil
}

func writeAtomically(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create runtime dir for %s: %w", path, err)
	}

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
