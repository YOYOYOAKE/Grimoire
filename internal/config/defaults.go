package config

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	DefaultConfigPath = "./configs/config.yaml"
	DefaultSQLitePath = "./data/grimoire.db"

	ProviderOpenAICustom = "openai_custom"
	ProviderOpenRouter   = "openrouter"

	defaultSaveDir            = "./data/images"
	defaultOpenRouterBaseURL  = "https://openrouter.ai/api/v1"
	defaultNAIBaseURL         = "https://image.idlecloud.cc/api"
	defaultTelegramTimeoutSec = 60
	defaultLLMTimeoutSec      = 180
	defaultNAITimeoutSec      = 180
	defaultNAIPollSec         = 5
)

func buildBaseConfigFromYAML(raw yamlConfig, sqlitePath string) (Config, error) {
	sqlitePath = strings.TrimSpace(sqlitePath)
	if sqlitePath == "" {
		sqlitePath = DefaultSQLitePath
	}

	cfg := Config{
		Telegram: TelegramConfig{
			BotToken:    raw.Telegram.BotToken,
			AdminUserID: raw.Telegram.AdminUserID,
			ProxyURL:    firstNonEmpty(raw.Telegram.Proxy, raw.Telegram.ProxyURL),
			TimeoutSec:  raw.Telegram.TimeoutSec,
		},
		NAI: NAIConfig{
			BaseURL:         raw.NAI.BaseURL,
			APIKey:          raw.NAI.APIKey,
			Model:           raw.NAI.Model,
			TimeoutSec:      raw.NAI.TimeoutSec,
			Proxy:           raw.NAI.Proxy,
			PollIntervalSec: defaultNAIPollSec,
		},
		Generation: GenerationConfig{
			ShapeDefault: "square",
			Artist:       "",
			ShapeMap: map[string]string{
				"square":    "1024x1024",
				"landscape": "1216x832",
				"portrait":  "832x1216",
			},
			Steps:    28,
			Scale:    5,
			Sampler:  "k_euler",
			NSamples: 1,
		},
		Runtime: RuntimeConfig{
			WorkerConcurrency: 1,
			SaveDir:           defaultSaveDir,
			SQLitePath:        sqlitePath,
		},
	}

	openaiEnabled := raw.LLM.OpenAICustom.Enable
	openrouterEnabled := raw.LLM.OpenRouter.Enable
	switch {
	case openaiEnabled && !openrouterEnabled:
		cfg.LLM = LLMConfig{
			Provider:   ProviderOpenAICustom,
			BaseURL:    raw.LLM.OpenAICustom.BaseURL,
			APIKey:     raw.LLM.OpenAICustom.APIKey,
			Model:      raw.LLM.OpenAICustom.Model,
			TimeoutSec: raw.LLM.TimeoutSec,
			Proxy:      raw.LLM.OpenAICustom.Proxy,
		}
	case !openaiEnabled && openrouterEnabled:
		cfg.LLM = LLMConfig{
			Provider:   ProviderOpenRouter,
			BaseURL:    defaultOpenRouterBaseURL,
			APIKey:     raw.LLM.OpenRouter.APIKey,
			Model:      raw.LLM.OpenRouter.Model,
			TimeoutSec: raw.LLM.TimeoutSec,
			Proxy:      raw.LLM.OpenRouter.Proxy,
		}
	default:
		return Config{}, fmt.Errorf("llm.openai_custom.enable 与 llm.openrouter.enable 必须且仅能启用一个")
	}

	cfg = normalizeConfig(cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalizeConfig(cfg Config) Config {
	cfg.Telegram.BotToken = strings.TrimSpace(cfg.Telegram.BotToken)
	cfg.Telegram.ProxyURL = strings.TrimSpace(cfg.Telegram.ProxyURL)
	if cfg.Telegram.TimeoutSec <= 0 {
		cfg.Telegram.TimeoutSec = defaultTelegramTimeoutSec
	}

	cfg.LLM.Provider = strings.TrimSpace(cfg.LLM.Provider)
	cfg.LLM.BaseURL = strings.TrimSpace(cfg.LLM.BaseURL)
	if cfg.LLM.Provider == ProviderOpenAICustom {
		cfg.LLM.BaseURL = ensureOpenAICustomV1BaseURL(cfg.LLM.BaseURL)
	} else {
		cfg.LLM.BaseURL = strings.TrimRight(cfg.LLM.BaseURL, "/")
	}
	cfg.LLM.APIKey = strings.TrimSpace(cfg.LLM.APIKey)
	cfg.LLM.Model = strings.TrimSpace(cfg.LLM.Model)
	cfg.LLM.Proxy = strings.TrimSpace(cfg.LLM.Proxy)
	if cfg.LLM.TimeoutSec <= 0 {
		cfg.LLM.TimeoutSec = defaultLLMTimeoutSec
	}

	cfg.NAI.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.NAI.BaseURL), "/")
	cfg.NAI.APIKey = strings.TrimSpace(cfg.NAI.APIKey)
	cfg.NAI.Model = strings.TrimSpace(cfg.NAI.Model)
	cfg.NAI.Proxy = strings.TrimSpace(cfg.NAI.Proxy)
	if cfg.NAI.TimeoutSec <= 0 {
		cfg.NAI.TimeoutSec = defaultNAITimeoutSec
	}
	if cfg.NAI.PollIntervalSec <= 0 {
		cfg.NAI.PollIntervalSec = defaultNAIPollSec
	}

	cfg.Generation.ShapeDefault = strings.ToLower(strings.TrimSpace(cfg.Generation.ShapeDefault))
	cfg.Generation.Artist = strings.TrimSpace(cfg.Generation.Artist)
	if cfg.Generation.ShapeMap == nil {
		cfg.Generation.ShapeMap = map[string]string{
			"square":    "1024x1024",
			"landscape": "1216x832",
			"portrait":  "832x1216",
		}
	}
	if cfg.Generation.Steps <= 0 {
		cfg.Generation.Steps = 28
	}
	if cfg.Generation.Scale <= 0 {
		cfg.Generation.Scale = 5
	}
	if strings.TrimSpace(cfg.Generation.Sampler) == "" {
		cfg.Generation.Sampler = "k_euler"
	}
	if cfg.Generation.NSamples <= 0 {
		cfg.Generation.NSamples = 1
	}

	if cfg.Runtime.WorkerConcurrency <= 0 {
		cfg.Runtime.WorkerConcurrency = 1
	}
	if strings.TrimSpace(cfg.Runtime.SaveDir) == "" {
		cfg.Runtime.SaveDir = defaultSaveDir
	}
	if strings.TrimSpace(cfg.Runtime.SQLitePath) == "" {
		cfg.Runtime.SQLitePath = DefaultSQLitePath
	}

	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ensureOpenAICustomV1BaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/v1"
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path
	default:
		parsed.Path = path + "/v1"
	}
	return parsed.String()
}
