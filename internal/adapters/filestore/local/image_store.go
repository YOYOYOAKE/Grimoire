package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	platformdb "grimoire/internal/platform/db"
)

type ImageStore struct {
	rootDir  string
	imageDir string
}

func NewImageStore(layout platformdb.SQLiteLayout) (*ImageStore, error) {
	rootDir := strings.TrimSpace(layout.RootDir)
	imageDir := strings.TrimSpace(layout.ImageDir)
	if rootDir == "" {
		return nil, fmt.Errorf("image store root dir is required")
	}
	if imageDir == "" {
		return nil, fmt.Errorf("image store dir is required")
	}
	rootDir = filepath.Clean(rootDir)
	if !filepath.IsAbs(rootDir) {
		return nil, fmt.Errorf("image store root dir must be absolute")
	}
	if !filepath.IsAbs(imageDir) {
		imageDir = filepath.Join(rootDir, imageDir)
	}
	imageDir = filepath.Clean(imageDir)
	if _, err := relativeWithinRoot(rootDir, imageDir); err != nil {
		return nil, fmt.Errorf("image store dir %s: %w", imageDir, err)
	}

	return &ImageStore{
		rootDir:  rootDir,
		imageDir: imageDir,
	}, nil
}

func (s *ImageStore) Save(ctx context.Context, userID string, taskID string, content []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	userID, err := validatePathSegment("user id", userID)
	if err != nil {
		return "", err
	}
	taskID, err = validatePathSegment("task id", taskID)
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", fmt.Errorf("image content is required")
	}

	targetPath := filepath.Join(s.imageDir, userID, taskID+".jpg")
	if err := writeAtomically(targetPath, content); err != nil {
		return "", err
	}

	relativePath, err := relativeWithinRoot(s.rootDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("build relative image path for %s: %w", targetPath, err)
	}
	return filepath.ToSlash(filepath.Clean(relativePath)), nil
}

func validatePathSegment(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", fmt.Errorf("%s contains invalid path characters", name)
	}
	return value, nil
}

func writeAtomically(path string, content []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create image dir for %s: %w", path, err)
	}

	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create image temp file for %s: %w", path, err)
	}

	tempPath := tempFile.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err = tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write image temp file for %s: %w", path, err)
	}
	if err = tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod image temp file for %s: %w", path, err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("close image temp file for %s: %w", path, err)
	}
	if err = os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace image %s: %w", path, err)
	}
	return nil
}

func relativeWithinRoot(rootDir string, targetPath string) (string, error) {
	relativePath, err := filepath.Rel(rootDir, targetPath)
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root dir")
	}
	return relativePath, nil
}
