package db

import (
	"path/filepath"
	"testing"
)

func TestResolveSQLiteLayoutDefaultsFromConfigDirectory(t *testing.T) {
	layout, err := ResolveSQLiteLayout("/tmp/grimoire/config/config.yaml", "", "", "")
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	if layout.RootDir != "/tmp/grimoire" {
		t.Fatalf("unexpected root dir: %q", layout.RootDir)
	}
	if layout.DataDir != "/tmp/grimoire/data" {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
	if layout.DatabasePath != "/tmp/grimoire/data/grimoire.db" {
		t.Fatalf("unexpected database path: %q", layout.DatabasePath)
	}
	if layout.ImageDir != "/tmp/grimoire/data/images" {
		t.Fatalf("unexpected image dir: %q", layout.ImageDir)
	}
}

func TestResolveSQLiteLayoutUsesConfigParentWhenPathIsCustom(t *testing.T) {
	layout, err := ResolveSQLiteLayout("/tmp/custom.yaml", "", "", "")
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	if layout.RootDir != "/tmp" {
		t.Fatalf("unexpected root dir: %q", layout.RootDir)
	}
	if layout.DataDir != "/tmp/data" {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
}

func TestResolveSQLiteLayoutSupportsRelativeOverrides(t *testing.T) {
	layout, err := ResolveSQLiteLayout(
		"/tmp/grimoire/config/config.yaml",
		"var/runtime",
		"var/db/grimoire.sqlite",
		"var/images",
	)
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	if layout.DataDir != "/tmp/grimoire/var/runtime" {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
	if layout.DatabasePath != "/tmp/grimoire/var/db/grimoire.sqlite" {
		t.Fatalf("unexpected database path: %q", layout.DatabasePath)
	}
	if layout.ImageDir != "/tmp/grimoire/var/images" {
		t.Fatalf("unexpected image dir: %q", layout.ImageDir)
	}
}

func TestResolveSQLiteLayoutSupportsAbsoluteOverrides(t *testing.T) {
	layout, err := ResolveSQLiteLayout(
		"/tmp/grimoire/config/config.yaml",
		"/srv/grimoire/data",
		"/srv/grimoire/db/grimoire.sqlite",
		"/srv/grimoire/images",
	)
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	if layout.DataDir != "/srv/grimoire/data" {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
	if layout.DatabasePath != "/srv/grimoire/db/grimoire.sqlite" {
		t.Fatalf("unexpected database path: %q", layout.DatabasePath)
	}
	if layout.ImageDir != "/srv/grimoire/images" {
		t.Fatalf("unexpected image dir: %q", layout.ImageDir)
	}
}

func TestResolveSQLiteLayoutRejectsEmptyConfigPath(t *testing.T) {
	if _, err := ResolveSQLiteLayout("", "", "", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSQLiteLayoutUsesDataDirForDerivedPaths(t *testing.T) {
	layout, err := ResolveSQLiteLayout("/tmp/grimoire/config/config.yaml", "runtime", "", "")
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	expectedDataDir := "/tmp/grimoire/runtime"
	if layout.DataDir != expectedDataDir {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
	if layout.DatabasePath != filepath.Join(expectedDataDir, defaultDBFileName) {
		t.Fatalf("unexpected database path: %q", layout.DatabasePath)
	}
	if layout.ImageDir != filepath.Join(expectedDataDir, defaultImageDirName) {
		t.Fatalf("unexpected image dir: %q", layout.ImageDir)
	}
}
