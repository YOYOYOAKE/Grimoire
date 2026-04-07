package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	platformdb "grimoire/internal/platform/db"
)

func TestImageStoreSaveCreatesDirectoriesAndReturnsRelativePath(t *testing.T) {
	rootDir := t.TempDir()
	store := newTestImageStore(t, rootDir, filepath.Join(rootDir, "data", "images"))

	relativePath, err := store.Save(context.Background(), "user-1", "task-1", []byte("image-bytes"))
	if err != nil {
		t.Fatalf("save image: %v", err)
	}

	if relativePath != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected relative path: %q", relativePath)
	}
	content, err := os.ReadFile(filepath.Join(rootDir, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read stored image: %v", err)
	}
	if string(content) != "image-bytes" {
		t.Fatalf("unexpected image content: %q", string(content))
	}
}

func TestImageStoreSaveReplacesExistingContent(t *testing.T) {
	rootDir := t.TempDir()
	store := newTestImageStore(t, rootDir, filepath.Join(rootDir, "data", "images"))

	if _, err := store.Save(context.Background(), "user-1", "task-1", []byte("first")); err != nil {
		t.Fatalf("save first image: %v", err)
	}
	relativePath, err := store.Save(context.Background(), "user-1", "task-1", []byte("second"))
	if err != nil {
		t.Fatalf("save replacement image: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(rootDir, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read replaced image: %v", err)
	}
	if string(content) != "second" {
		t.Fatalf("unexpected replaced content: %q", string(content))
	}
}

func TestImageStoreSaveRejectsInvalidPathSegment(t *testing.T) {
	rootDir := t.TempDir()
	store := newTestImageStore(t, rootDir, filepath.Join(rootDir, "data", "images"))

	if _, err := store.Save(context.Background(), "../user", "task-1", []byte("image")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := store.Save(context.Background(), "user-1", "task/1", []byte("image")); err == nil {
		t.Fatal("expected error")
	}
}

func TestImageStoreSaveRejectsEmptyContent(t *testing.T) {
	rootDir := t.TempDir()
	store := newTestImageStore(t, rootDir, filepath.Join(rootDir, "data", "images"))

	if _, err := store.Save(context.Background(), "user-1", "task-1", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestImageStoreSaveSupportsAbsoluteImageDir(t *testing.T) {
	rootDir := t.TempDir()
	imageDir := filepath.Join(rootDir, "var", "images")
	store := newTestImageStore(t, rootDir, imageDir)

	relativePath, err := store.Save(context.Background(), "user-1", "task-1", []byte("image"))
	if err != nil {
		t.Fatalf("save image: %v", err)
	}

	if relativePath != "var/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected relative path: %q", relativePath)
	}
}

func TestImageStoreSaveAnchorsRelativeImageDirToRoot(t *testing.T) {
	rootDir := t.TempDir()
	store := newTestImageStore(t, rootDir, filepath.Join("data", "images"))

	relativePath, err := store.Save(context.Background(), "user-1", "task-1", []byte("image"))
	if err != nil {
		t.Fatalf("save image: %v", err)
	}

	if relativePath != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected relative path: %q", relativePath)
	}
}

func TestNewImageStoreRejectsImageDirOutsideRoot(t *testing.T) {
	rootDir := t.TempDir()
	imageDir := filepath.Join(rootDir, "..", "images")

	if _, err := NewImageStore(platformdb.SQLiteLayout{
		RootDir:  rootDir,
		ImageDir: imageDir,
	}); err == nil {
		t.Fatal("expected error")
	}
}

func newTestImageStore(t *testing.T, rootDir string, imageDir string) *ImageStore {
	t.Helper()

	store, err := NewImageStore(platformdb.SQLiteLayout{
		RootDir:  rootDir,
		ImageDir: imageDir,
	})
	if err != nil {
		t.Fatalf("new image store: %v", err)
	}
	return store
}
