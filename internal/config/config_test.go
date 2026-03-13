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
	expected := filepath.Join("/tmp/grimoire/bin", "config.yaml")
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
	for _, section := range []string{"telegram:", "llms:", "nai:"} {
		if !strings.Contains(content, section) {
			t.Fatalf("missing section %q in template", section)
		}
	}
}

func TestLoadGeneratedTemplateValidationFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
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

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
