package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadNormalizesOpenAIBaseURL(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms:
  - base_url: "https://api.openai.com"
    api_key: "key"
    model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := len(cfg.LLMs); got != 1 {
		t.Fatalf("unexpected llm count: %d", got)
	}
	if cfg.LLMs[0].BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected llm base url: %q", cfg.LLMs[0].BaseURL)
	}
	if cfg.LLMs[0].TimeoutSec != 180 {
		t.Fatalf("unexpected llm timeout: %d", cfg.LLMs[0].TimeoutSec)
	}
	if cfg.LLMs[0].ReasoningEffort != "" {
		t.Fatalf("unexpected llm reasoning effort: %q", cfg.LLMs[0].ReasoningEffort)
	}
	if cfg.Conversation.RecentMessageLimit != 15 {
		t.Fatalf("unexpected conversation recent message limit: %d", cfg.Conversation.RecentMessageLimit)
	}
}

func TestLoadTrimsLLMReasoningEffort(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
    reasoning_effort: " custom-effort "
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LLMs[0].ReasoningEffort != "custom-effort" {
		t.Fatalf("unexpected reasoning effort: %q", cfg.LLMs[0].ReasoningEffort)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
    unknown: true
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsLegacyLLMObject(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llm:
  base_url: "https://api.openai.com/v1"
  api_key: "key"
  model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRequiresAtLeastOneLLM(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms: []
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveStartupPathUsesExecutableDir(t *testing.T) {
	path, usedDefault, err := resolveStartupPath(nil, func() (string, error) {
		return "/tmp/grimoire/bin/grimoire-bot", nil
	})
	if err != nil {
		t.Fatalf("resolve startup path: %v", err)
	}
	if !usedDefault {
		t.Fatal("expected default path")
	}
	expected := filepath.Join("/tmp/grimoire/bin", "config", "config.yaml")
	if path != expected {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestResolveStartupPathUsesExplicitPath(t *testing.T) {
	path, usedDefault, err := resolveStartupPath([]string{"./custom.yaml"}, func() (string, error) {
		t.Fatal("executable path should not be used")
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolve startup path: %v", err)
	}
	if usedDefault {
		t.Fatal("expected explicit path")
	}
	if path != "custom.yaml" {
		t.Fatalf("unexpected path: %q", path)
	}
}

func TestResolveStartupPathRejectsTooManyArgs(t *testing.T) {
	if _, _, err := resolveStartupPath([]string{"a", "b"}, func() (string, error) {
		return "", nil
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureDefaultConfigWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin", "config.yaml")
	if err := EnsureDefaultConfig(path); err != nil {
		t.Fatalf("ensure default config: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	content := string(data)
	for _, section := range []string{"telegram:", "storage:", "conversation:", "recovery:", "llms:", "reasoning_effort:", "nai:"} {
		if !strings.Contains(content, section) {
			t.Fatalf("missing section %q in template", section)
		}
	}
	if !strings.Contains(content, "low, medium, high, xhigh") {
		t.Fatalf("missing reasoning effort examples in template")
	}
}

func TestLoadGeneratedTemplateValidationFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "config.yaml")
	if err := EnsureDefaultConfig(path); err != nil {
		t.Fatalf("ensure default config: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadRejectsUnsupportedNAIModel(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-3"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestExplicitMissingConfigReturnsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadRejectsNegativeRecentMessageLimit(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
conversation:
  recent_message_limit: -1
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadDefaultsRecoveryEnabledWhenUnset(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Recovery.EnabledValue() {
		t.Fatal("expected recovery to default to enabled")
	}
}

func TestLoadAllowsExplicitRecoveryDisable(t *testing.T) {
	path := writeTestConfig(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
recovery:
  enabled: false
llms:
  - base_url: "https://api.openai.com/v1"
    api_key: "key"
    model: "gpt-4o-mini"
nai:
  base_url: "https://image.novelai.net"
  api_key: "key"
  model: "nai-diffusion-4-5-full"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Recovery.EnabledValue() {
		t.Fatal("expected recovery to remain disabled")
	}
}

func TestConfigResolveSQLiteLayoutUsesStorageOverrides(t *testing.T) {
	cfg := normalize(Config{
		Storage: Storage{
			DataDir:    "var/data",
			SQLitePath: "var/db/grimoire.sqlite",
			ImageDir:   "var/images",
		},
	})

	layout, err := cfg.ResolveSQLiteLayout("/tmp/grimoire/config/config.yaml")
	if err != nil {
		t.Fatalf("resolve sqlite layout: %v", err)
	}

	if layout.DataDir != "/tmp/grimoire/var/data" {
		t.Fatalf("unexpected data dir: %q", layout.DataDir)
	}
	if layout.DatabasePath != "/tmp/grimoire/var/db/grimoire.sqlite" {
		t.Fatalf("unexpected database path: %q", layout.DatabasePath)
	}
	if layout.ImageDir != "/tmp/grimoire/var/images" {
		t.Fatalf("unexpected image dir: %q", layout.ImageDir)
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
