package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfigYAML = `telegram:
  bot_token: "token"
  admin_user_id: 123
llm:
  base_url: "https://api.openai.com/v1"
  api_key: "old-llm-key"
  model: "gpt-4o-mini"
  timeout_sec: 30
nai:
  base_url: "https://image.idlecloud.cc/api"
  api_key: "nai-key"
  model: "nai-diffusion-4-5-full"
  poll_interval_sec: 5
generation:
  shape_default: "square"
  artist: ""
  shape_map:
    square: "1024x1024"
    landscape: "1216x832"
    portrait: "832x1216"
  steps: 28
  scale: 5
  sampler: "k_euler"
  n_samples: 1
runtime:
  worker_concurrency: 1
  save_dir: "/tmp/images"
  sqlite_path: "/tmp/grimoire.db"
`

func TestManagerSetByPathWritesAndReloads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfigYAML), 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.SetByPath("llm.api_key", "new-llm-key"); err != nil {
		t.Fatalf("SetByPath: %v", err)
	}

	snapshot := m.Snapshot()
	if snapshot.LLM.APIKey != "new-llm-key" {
		t.Fatalf("expected in-memory API key updated, got %q", snapshot.LLM.APIKey)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(string(content), "new-llm-key") {
		t.Fatalf("expected file contains updated API key, content=%s", string(content))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected file perm 0600, got %o", info.Mode().Perm())
	}
}

func TestManagerSetShapeDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfigYAML), 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.SetByPath("generation.shape_default", "portrait"); err != nil {
		t.Fatalf("SetByPath portrait: %v", err)
	}
	if got := m.Snapshot().Generation.ShapeDefault; got != "portrait" {
		t.Fatalf("expected portrait, got %q", got)
	}

	if err := m.SetByPath("generation.shape_default", "unknown-shape"); err == nil {
		t.Fatalf("expected error for invalid shape default")
	}
}

func TestManagerSetArtist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfigYAML), 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	value := "artist:kuon, artist:foo"
	if err := m.SetByPath("generation.artist", value); err != nil {
		t.Fatalf("SetByPath generation.artist: %v", err)
	}
	if got := m.Snapshot().Generation.Artist; got != value {
		t.Fatalf("unexpected artist: %q", got)
	}
}

func TestManagerSetTelegramProxyURL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfigYAML), 0o600); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.SetByPath("telegram.proxy_url", "http://127.0.0.1:7890"); err != nil {
		t.Fatalf("SetByPath telegram.proxy_url: %v", err)
	}
	if got := m.Snapshot().Telegram.ProxyURL; got != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy url: %q", got)
	}

	if err := m.SetByPath("telegram.proxy_url", "://bad-proxy"); err == nil {
		t.Fatalf("expected invalid proxy url error")
	}
}
