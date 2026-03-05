package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewManagerMissingConfigFile(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)

	_, err := NewManager(filepath.Join(t.TempDir(), "grimoire.db"))
	if err == nil {
		t.Fatalf("expected missing config error")
	}
	if !strings.Contains(err.Error(), "读取配置文件失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "configs/config.yaml") {
		t.Fatalf("unexpected path in error: %v", err)
	}
}

func TestNewManagerRejectsInvalidLLMProviderSelection(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, `
telegram:
  bot_token: "token"
  admin_user_id: 1
  proxy: ""
  timeout_sec: 60
llm:
  timeout_sec: 180
  openai_custom:
    enable: true
    base_url: "https://api.openai.com/v1"
    api_key: "k"
    model: "gpt-4o-mini"
    proxy: ""
  openrouter:
    enable: true
    api_key: "k"
    model: "openrouter/model"
    proxy: ""
nai:
  base_url: "https://image.idlecloud.cc/api"
  api_key: "nai-k"
  model: "nai-model"
  timeout_sec: 180
  proxy: ""
`)

	_, err := NewManager(filepath.Join(t.TempDir(), "grimoire.db"))
	if err == nil {
		t.Fatalf("expected llm provider selection error")
	}
	if !strings.Contains(err.Error(), "必须且仅能启用一个") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewManagerLoadsOpenAICustom(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: true
    base_url: "https://api.openai.com"
    api_key: "llm-key"
    model: "gpt-4o-mini"
    proxy: "http://127.0.0.1:7890"
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
`))

	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	cfg := mgr.Snapshot()
	if cfg.LLM.Provider != ProviderOpenAICustom {
		t.Fatalf("unexpected provider: %s", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected llm base url: %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected llm proxy: %q", cfg.LLM.Proxy)
	}
	if cfg.Telegram.BotToken != "token" || cfg.Telegram.AdminUserID != 1 {
		t.Fatalf("unexpected telegram config: %+v", cfg.Telegram)
	}
}

func TestOpenAICustomBaseURLKeepsSingleV1Suffix(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: true
    base_url: "https://example.com/proxy/v1/"
    api_key: "llm-key"
    model: "gpt-4o-mini"
    proxy: ""
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
`))

	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	if got := mgr.Snapshot().LLM.BaseURL; got != "https://example.com/proxy/v1" {
		t.Fatalf("unexpected llm base url: %q", got)
	}
}

func TestNewManagerLoadsOpenRouter(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: false
    base_url: "https://api.openai.com/v1"
    api_key: ""
    model: ""
    proxy: ""
  openrouter:
    enable: true
    api_key: "or-key"
    model: "openai/gpt-4o-mini"
    proxy: "http://127.0.0.1:7890"
`))

	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	cfg := mgr.Snapshot()
	if cfg.LLM.Provider != ProviderOpenRouter {
		t.Fatalf("unexpected provider: %s", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("unexpected openrouter base url: %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.APIKey != "or-key" || cfg.LLM.Model != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected openrouter config: %+v", cfg.LLM)
	}
}

func TestManagerMissingDrawConfigKeysOpenAICustom(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: true
    base_url: ""
    api_key: ""
    model: ""
    proxy: ""
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
`))

	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	missing := mgr.MissingDrawConfigKeys()
	expected := []string{
		"llm.openai_custom.base_url",
		"llm.openai_custom.api_key",
		"llm.openai_custom.model",
	}
	if !reflect.DeepEqual(missing, expected) {
		t.Fatalf("unexpected missing keys: got=%v want=%v", missing, expected)
	}
}

func TestManagerSetByPathOnlyGenerationPersistsAndReloads(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: true
    base_url: "https://api.openai.com/v1"
    api_key: "llm-key"
    model: "gpt-4o-mini"
    proxy: ""
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
`))

	path := filepath.Join(t.TempDir(), "grimoire.db")
	mgr := mustNewManager(t, path)
	defer func() { _ = mgr.Close() }()

	if err := mgr.SetByPath("generation.shape_default", "portrait"); err != nil {
		t.Fatalf("set shape default: %v", err)
	}
	if err := mgr.SetByPath("generation.artist", "artist:a, artist:b"); err != nil {
		t.Fatalf("set artist: %v", err)
	}

	_ = mgr.Close()
	reloaded := mustNewManager(t, path)
	defer func() { _ = reloaded.Close() }()

	cfg := reloaded.Snapshot()
	if cfg.Generation.ShapeDefault != "portrait" {
		t.Fatalf("unexpected shape default: %q", cfg.Generation.ShapeDefault)
	}
	if cfg.Generation.Artist != "artist:a, artist:b" {
		t.Fatalf("unexpected artist: %q", cfg.Generation.Artist)
	}
}

func TestManagerSetByPathRejectsLLMAndNAI(t *testing.T) {
	wd := t.TempDir()
	chdir(t, wd)
	writeConfigFile(t, validConfigYAML(`
  openai_custom:
    enable: true
    base_url: "https://api.openai.com/v1"
    api_key: "llm-key"
    model: "gpt-4o-mini"
    proxy: ""
  openrouter:
    enable: false
    api_key: ""
    model: ""
    proxy: ""
`))

	mgr := mustNewManager(t, filepath.Join(t.TempDir(), "grimoire.db"))
	defer func() { _ = mgr.Close() }()

	if err := mgr.SetByPath("llm.model", "gpt-5-mini"); err == nil {
		t.Fatalf("expected llm set rejected")
	}
	if err := mgr.SetByPath("nai.model", "nai-x"); err == nil {
		t.Fatalf("expected nai set rejected")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

func writeConfigFile(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join("configs", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func validConfigYAML(llmSection string) string {
	return `
telegram:
  bot_token: "token"
  admin_user_id: 1
  proxy: ""
  timeout_sec: 60
llm:
  timeout_sec: 180
` + llmSection + `
nai:
  base_url: "https://image.idlecloud.cc/api"
  api_key: "nai-key"
  model: "nai-model"
  timeout_sec: 180
  proxy: ""
`
}

func mustNewManager(t *testing.T, path string) *Manager {
	t.Helper()
	mgr, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}
