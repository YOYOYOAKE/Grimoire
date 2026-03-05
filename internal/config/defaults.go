package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	EnvTelegramBotToken    = "GRIMOIRE_TELEGRAM_BOT_TOKEN"
	EnvTelegramAdminUserID = "GRIMOIRE_TELEGRAM_ADMIN_USER_ID"
	EnvTelegramProxyURL    = "GRIMOIRE_TELEGRAM_PROXY_URL"

	DefaultSQLitePath = "./data/grimoire.db"
	defaultSaveDir    = "./data/images"
	defaultNAIBaseURL = "https://image.idlecloud.cc/api"
)

func buildBaseConfig(sqlitePath string) (Config, error) {
	sqlitePath = strings.TrimSpace(sqlitePath)
	if sqlitePath == "" {
		sqlitePath = DefaultSQLitePath
	}

	adminRaw := strings.TrimSpace(os.Getenv(EnvTelegramAdminUserID))
	adminID := int64(0)
	if adminRaw != "" {
		parsed, err := strconv.ParseInt(adminRaw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("%s 必须是整数: %w", EnvTelegramAdminUserID, err)
		}
		adminID = parsed
	}

	cfg := Config{
		Telegram: TelegramConfig{
			BotToken:    strings.TrimSpace(os.Getenv(EnvTelegramBotToken)),
			AdminUserID: adminID,
			ProxyURL:    strings.TrimSpace(os.Getenv(EnvTelegramProxyURL)),
		},
		LLM: LLMConfig{
			TimeoutSec: 180,
		},
		NAI: NAIConfig{
			BaseURL:         defaultNAIBaseURL,
			PollIntervalSec: 5,
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
	return normalizeConfig(cfg), nil
}

func normalizeConfig(cfg Config) Config {
	cfg.Telegram.BotToken = strings.TrimSpace(cfg.Telegram.BotToken)
	cfg.Telegram.ProxyURL = strings.TrimSpace(cfg.Telegram.ProxyURL)

	cfg.LLM.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.LLM.BaseURL), "/")
	cfg.LLM.APIKey = strings.TrimSpace(cfg.LLM.APIKey)
	cfg.LLM.Model = strings.TrimSpace(cfg.LLM.Model)
	if cfg.LLM.TimeoutSec <= 0 {
		cfg.LLM.TimeoutSec = 180
	}

	cfg.NAI.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.NAI.BaseURL), "/")
	cfg.NAI.APIKey = strings.TrimSpace(cfg.NAI.APIKey)
	cfg.NAI.Model = strings.TrimSpace(cfg.NAI.Model)
	if cfg.NAI.PollIntervalSec <= 0 {
		cfg.NAI.PollIntervalSec = 5
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
	if cfg.Generation.Sampler == "" {
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
