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
		if err := validateProxyURL("telegram.proxy_url", cfg.Telegram.ProxyURL); err != nil {
			return err
		}
	}

	switch cfg.LLM.Provider {
	case ProviderOpenAICustom:
	case ProviderOpenRouter:
	default:
		return fmt.Errorf("不支持的 llm provider: %s", cfg.LLM.Provider)
	}
	if cfg.LLM.BaseURL != "" {
		if err := validateBaseURL("llm.base_url", cfg.LLM.BaseURL); err != nil {
			return err
		}
	}
	if cfg.LLM.Proxy != "" {
		if err := validateProxyURL("llm.proxy", cfg.LLM.Proxy); err != nil {
			return err
		}
	}

	if cfg.NAI.BaseURL != "" {
		if err := validateBaseURL("nai.base_url", cfg.NAI.BaseURL); err != nil {
			return err
		}
	}
	if cfg.NAI.Proxy != "" {
		if err := validateProxyURL("nai.proxy", cfg.NAI.Proxy); err != nil {
			return err
		}
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

func validateProxyURL(key string, raw string) error {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s 非法: %w", key, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s 必须包含 scheme 和 host", key)
	}
	return nil
}

func validateBaseURL(key string, raw string) error {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s 非法: %w", key, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s 必须包含 scheme 和 host", key)
	}
	return nil
}
