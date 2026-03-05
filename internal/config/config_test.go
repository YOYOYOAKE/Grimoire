package config

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewManagerRequiresTelegramEnv(t *testing.T) {
	t.Setenv(EnvTelegramBotToken, "")
	t.Setenv(EnvTelegramAdminUserID, "")
	t.Setenv(EnvTelegramProxyURL, "")

	_, err := NewManager(filepath.Join(t.TempDir(), "grimoire.db"))
	if err == nil {
		t.Fatalf("expected env validation error")
	}
	if !strings.Contains(err.Error(), EnvTelegramBotToken) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagerLoadsWithEnvAndAllowsMissingDrawConfig(t *testing.T) {
	setRequiredTelegramEnv(t)
	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	snapshot := mgr.Snapshot()
	if snapshot.Telegram.BotToken != "token" {
		t.Fatalf("unexpected bot token: %q", snapshot.Telegram.BotToken)
	}
	if snapshot.Telegram.AdminUserID != 123 {
		t.Fatalf("unexpected admin id: %d", snapshot.Telegram.AdminUserID)
	}
	if snapshot.NAI.BaseURL != "https://image.idlecloud.cc/api" {
		t.Fatalf("unexpected nai base url: %q", snapshot.NAI.BaseURL)
	}
	if snapshot.Generation.ShapeDefault != "square" {
		t.Fatalf("unexpected shape default: %q", snapshot.Generation.ShapeDefault)
	}
	missing := mgr.MissingDrawConfigKeys()
	expected := []string{"llm.base_url", "llm.api_key", "llm.model", "nai.api_key", "nai.model"}
	if !reflect.DeepEqual(missing, expected) {
		t.Fatalf("unexpected missing keys: got=%v want=%v", missing, expected)
	}
}

func TestManagerSetByPathPersistsAndReloads(t *testing.T) {
	setRequiredTelegramEnv(t)
	path := filepath.Join(t.TempDir(), "grimoire.db")
	mgr := mustNewManager(t, path)
	defer func() { _ = mgr.Close() }()

	updates := map[string]string{
		"llm.base_url":             "https://example-llm.com/v1/",
		"llm.api_key":              "llm-key",
		"llm.model":                "gpt-4.1-mini",
		"nai.api_key":              "nai-key",
		"nai.model":                "nai-model",
		"generation.shape_default": "portrait",
		"generation.artist":        "artist:a, artist:b",
	}
	for path, value := range updates {
		if err := mgr.SetByPath(path, value); err != nil {
			t.Fatalf("SetByPath(%s) error: %v", path, err)
		}
	}
	if missing := mgr.MissingDrawConfigKeys(); len(missing) != 0 {
		t.Fatalf("expected missing keys resolved, got %v", missing)
	}

	_ = mgr.Close()
	reloaded := mustNewManager(t, path)
	defer func() { _ = reloaded.Close() }()

	cfg := reloaded.Snapshot()
	if cfg.LLM.BaseURL != "https://example-llm.com/v1" {
		t.Fatalf("unexpected llm base url: %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "llm-key" || cfg.LLM.Model != "gpt-4.1-mini" {
		t.Fatalf("unexpected llm config: %+v", cfg.LLM)
	}
	if cfg.NAI.APIKey != "nai-key" || cfg.NAI.Model != "nai-model" {
		t.Fatalf("unexpected nai config: %+v", cfg.NAI)
	}
	if cfg.Generation.ShapeDefault != "portrait" {
		t.Fatalf("unexpected shape default: %q", cfg.Generation.ShapeDefault)
	}
	if cfg.Generation.Artist != "artist:a, artist:b" {
		t.Fatalf("unexpected artist: %q", cfg.Generation.Artist)
	}
}

func TestManagerSetShapeDefaultValidation(t *testing.T) {
	setRequiredTelegramEnv(t)
	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	if err := mgr.SetByPath("generation.shape_default", "invalid-shape"); err == nil {
		t.Fatalf("expected invalid shape error")
	}
}

func TestManagerSetUnsupportedPath(t *testing.T) {
	setRequiredTelegramEnv(t)
	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	if err := mgr.SetByPath("nai.base_url", "https://example.com/api"); err == nil {
		t.Fatalf("expected unsupported path error")
	}
}

func setRequiredTelegramEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvTelegramBotToken, "token")
	t.Setenv(EnvTelegramAdminUserID, "123")
	t.Setenv(EnvTelegramProxyURL, "")
}

func mustNewManager(t *testing.T, path string) *Manager {
	t.Helper()
	mgr, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}
