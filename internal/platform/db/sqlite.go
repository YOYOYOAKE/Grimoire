package db

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	defaultDataDirName  = "data"
	defaultDBFileName   = "grimoire.db"
	defaultImageDirName = "images"
)

type SQLiteLayout struct {
	RootDir      string
	DataDir      string
	DatabasePath string
	ImageDir     string
}

func ResolveSQLiteLayout(configPath string, dataDir string, databasePath string, imageDir string) (SQLiteLayout, error) {
	rootDir, err := resolveRootDir(configPath)
	if err != nil {
		return SQLiteLayout{}, err
	}

	resolvedDataDir := resolvePath(rootDir, dataDir)
	if resolvedDataDir == "" {
		resolvedDataDir = filepath.Join(rootDir, defaultDataDirName)
	}

	resolvedDatabasePath := resolvePath(rootDir, databasePath)
	if resolvedDatabasePath == "" {
		resolvedDatabasePath = filepath.Join(resolvedDataDir, defaultDBFileName)
	}

	resolvedImageDir := resolvePath(rootDir, imageDir)
	if resolvedImageDir == "" {
		resolvedImageDir = filepath.Join(resolvedDataDir, defaultImageDirName)
	}

	return SQLiteLayout{
		RootDir:      rootDir,
		DataDir:      filepath.Clean(resolvedDataDir),
		DatabasePath: filepath.Clean(resolvedDatabasePath),
		ImageDir:     filepath.Clean(resolvedImageDir),
	}, nil
}

func resolveRootDir(configPath string) (string, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return "", fmt.Errorf("config path is required")
	}

	configDir := filepath.Dir(filepath.Clean(configPath))
	if filepath.Base(configDir) == "config" {
		return filepath.Dir(configDir), nil
	}
	return configDir, nil
}

func resolvePath(rootDir string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(rootDir, value)
}
