package config

import "strings"

func applyDefaults(cfg Config) Config {
	cfg.Telegram.ProxyURL = strings.TrimSpace(cfg.Telegram.ProxyURL)

	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://api.openai.com/v1"
	}
	cfg.LLM.BaseURL = strings.TrimRight(cfg.LLM.BaseURL, "/")
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "gpt-4o-mini"
	}
	if cfg.LLM.TimeoutSec <= 0 {
		cfg.LLM.TimeoutSec = 30
	}

	if cfg.NAI.BaseURL == "" {
		cfg.NAI.BaseURL = "https://image.idlecloud.cc/api"
	}
	cfg.NAI.BaseURL = strings.TrimRight(cfg.NAI.BaseURL, "/")
	if cfg.NAI.Model == "" {
		cfg.NAI.Model = "nai-diffusion-4-5-full"
	}
	if cfg.NAI.PollIntervalSec <= 0 {
		cfg.NAI.PollIntervalSec = 5
	}

	if cfg.Generation.ShapeDefault == "" {
		cfg.Generation.ShapeDefault = "square"
	}
	cfg.Generation.Artist = strings.TrimSpace(cfg.Generation.Artist)
	if cfg.Generation.ShapeMap == nil {
		cfg.Generation.ShapeMap = map[string]string{}
	}
	if cfg.Generation.ShapeMap["square"] == "" {
		cfg.Generation.ShapeMap["square"] = "1024x1024"
	}
	if cfg.Generation.ShapeMap["landscape"] == "" {
		cfg.Generation.ShapeMap["landscape"] = "1216x832"
	}
	if cfg.Generation.ShapeMap["portrait"] == "" {
		cfg.Generation.ShapeMap["portrait"] = "832x1216"
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
	if cfg.Runtime.SaveDir == "" {
		cfg.Runtime.SaveDir = "/home/YOAKE/dev/Grimoire/data/images"
	}
	if cfg.Runtime.SQLitePath == "" {
		cfg.Runtime.SQLitePath = "/home/YOAKE/dev/Grimoire/data/grimoire.db"
	}

	return cfg
}
