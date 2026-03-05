package config

import (
	"errors"
	"fmt"
	neturl "net/url"
)

func validate(cfg Config) error {
	if cfg.Telegram.BotToken == "" {
		return errors.New("telegram.bot_token 不能为空")
	}
	if cfg.Telegram.AdminUserID <= 0 {
		return errors.New("telegram.admin_user_id 必须 > 0")
	}
	if cfg.Telegram.ProxyURL != "" {
		parsed, err := neturl.Parse(cfg.Telegram.ProxyURL)
		if err != nil {
			return fmt.Errorf("telegram.proxy_url 非法: %w", err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("telegram.proxy_url 必须包含 scheme 和 host")
		}
	}
	if cfg.LLM.BaseURL == "" {
		return errors.New("llm.base_url 不能为空")
	}
	if cfg.LLM.APIKey == "" {
		return errors.New("llm.api_key 不能为空")
	}
	if cfg.LLM.Model == "" {
		return errors.New("llm.model 不能为空")
	}
	if cfg.NAI.BaseURL == "" {
		return errors.New("nai.base_url 不能为空")
	}
	if cfg.NAI.APIKey == "" {
		return errors.New("nai.api_key 不能为空")
	}
	if cfg.NAI.Model == "" {
		return errors.New("nai.model 不能为空")
	}
	if cfg.Generation.ShapeMap[cfg.Generation.ShapeDefault] == "" {
		return fmt.Errorf("generation.shape_default=%s 未在 shape_map 中定义", cfg.Generation.ShapeDefault)
	}
	if cfg.Generation.NSamples <= 0 {
		return errors.New("generation.n_samples 必须 > 0")
	}
	if cfg.Runtime.WorkerConcurrency <= 0 {
		return errors.New("runtime.worker_concurrency 必须 > 0")
	}
	if cfg.Runtime.SaveDir == "" {
		return errors.New("runtime.save_dir 不能为空")
	}
	if cfg.Runtime.SQLitePath == "" {
		return errors.New("runtime.sqlite_path 不能为空")
	}
	return nil
}
