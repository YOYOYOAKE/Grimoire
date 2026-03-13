package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, fmt.Errorf("config path is required")
	}

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return Config{}, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %s: %w", path, err)
	}

	cfg = normalize(cfg)
	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

func normalize(cfg Config) Config {
	cfg.Telegram.BotToken = strings.TrimSpace(cfg.Telegram.BotToken)
	cfg.Telegram.Proxy = strings.TrimSpace(cfg.Telegram.Proxy)
	cfg.Telegram.TimeoutSec = defaultInt(cfg.Telegram.TimeoutSec, 60)

	for i, llm := range cfg.LLMs {
		normalizeProvider(&llm.BaseURL, &llm.APIKey, &llm.Model, &llm.Proxy, true, &llm.TimeoutSec, 180)
		cfg.LLMs[i] = llm
	}

	normalizeProvider(&cfg.NAI.BaseURL, &cfg.NAI.APIKey, &cfg.NAI.Model, &cfg.NAI.Proxy, false, &cfg.NAI.TimeoutSec, 180)

	return cfg
}

func normalizeProvider(baseURL, apiKey, model, proxy *string, ensureV1 bool, timeout *int, defaultTimeout int) {
	*baseURL = ensureBaseURL(strings.TrimSpace(*baseURL), ensureV1)
	*apiKey = strings.TrimSpace(*apiKey)
	*model = strings.TrimSpace(*model)
	*proxy = strings.TrimSpace(*proxy)
	*timeout = defaultInt(*timeout, defaultTimeout)
}

func defaultInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func ResolveStartupPath(args []string) (string, bool, error) {
	return resolveStartupPath(args, os.Executable)
}

func EnsureDefaultConfig(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("config path is required")
	}

	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	file, err := os.OpenFile(clean, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create config file %s: %w", clean, err)
	}
	defer file.Close()

	if _, err := file.WriteString(DefaultTemplate()); err != nil {
		return fmt.Errorf("write config file %s: %w", clean, err)
	}
	return nil
}

func DefaultTemplate() string {
	return `telegram:
  bot_token: ""
  admin_user_id: 123456789
  proxy: ""
  timeout_sec: 60

llms:
  - base_url: "https://api.openai.com/v1"
    api_key: ""
    model: "gpt-4o-mini"
    proxy: ""
    timeout_sec: 180

nai:
  base_url: "https://image.novelai.net"
  api_key: ""
  model: "nai-diffusion-4-5-full"
  proxy: ""
  timeout_sec: 180
`
}

func resolveStartupPath(args []string, executablePath func() (string, error)) (string, bool, error) {
	switch len(args) {
	case 0:
		executable, err := executablePath()
		if err != nil {
			return "", false, fmt.Errorf("resolve executable path: %w", err)
		}
		return filepath.Join(filepath.Dir(executable), "config.yaml"), true, nil
	case 1:
		return filepath.Clean(args[0]), false, nil
	default:
		return "", false, fmt.Errorf("expected at most one config path argument")
	}
}
