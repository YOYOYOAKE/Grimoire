package config

import (
	"fmt"
	"net/url"
	"strings"
)

func validate(cfg Config) error {
	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("telegram.bot_token is required")
	}
	if cfg.Telegram.AdminUserID <= 0 {
		return fmt.Errorf("telegram.admin_user_id must be > 0")
	}
	if err := validateOptionalURL("telegram.proxy", cfg.Telegram.Proxy); err != nil {
		return err
	}

	if len(cfg.LLMs) == 0 {
		return fmt.Errorf("llms must contain at least one entry")
	}
	for i, llm := range cfg.LLMs {
		prefix := fmt.Sprintf("llms[%d]", i)
		if llm.BaseURL == "" {
			return fmt.Errorf("%s.base_url is required", prefix)
		}
		if llm.APIKey == "" {
			return fmt.Errorf("%s.api_key is required", prefix)
		}
		if llm.Model == "" {
			return fmt.Errorf("%s.model is required", prefix)
		}
		if err := validateURL(prefix+".base_url", llm.BaseURL); err != nil {
			return err
		}
		if err := validateOptionalURL(prefix+".proxy", llm.Proxy); err != nil {
			return err
		}
	}

	if cfg.NAI.BaseURL == "" {
		return fmt.Errorf("nai.base_url is required")
	}
	if cfg.NAI.APIKey == "" {
		return fmt.Errorf("nai.api_key is required")
	}
	if cfg.NAI.Model == "" {
		return fmt.Errorf("nai.model is required")
	}
	if err := validateURL("nai.base_url", cfg.NAI.BaseURL); err != nil {
		return err
	}
	if err := validateOptionalURL("nai.proxy", cfg.NAI.Proxy); err != nil {
		return err
	}
	return nil
}

func validateURL(name string, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s invalid: %w", name, err)
	}
	if strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("%s must include scheme and host", name)
	}
	return nil
}

func validateOptionalURL(name string, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return validateURL(name, raw)
}

func ensureBaseURL(raw string, ensureV1 bool) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if !ensureV1 || raw == "" {
		return raw
	}
	if strings.HasSuffix(raw, "/v1") {
		return raw
	}
	return raw + "/v1"
}
