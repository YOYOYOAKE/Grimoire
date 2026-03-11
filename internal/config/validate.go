package config

import (
	"fmt"
	"net/url"
	"strings"
)

func validate(cfg Config) error {
	if err := validateRequiredString("telegram.bot_token", cfg.Telegram.BotToken); err != nil {
		return err
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
		if err := validateProvider(fmt.Sprintf("llms[%d]", i), llm.BaseURL, llm.APIKey, llm.Model, llm.Proxy); err != nil {
			return err
		}
	}
	if err := validateProvider("nai", cfg.NAI.BaseURL, cfg.NAI.APIKey, cfg.NAI.Model, cfg.NAI.Proxy); err != nil {
		return err
	}
	if cfg.NAI.Model != "nai-diffusion-4-5-full" {
		return fmt.Errorf("nai.model must be nai-diffusion-4-5-full")
	}
	return nil
}

func validateRequiredString(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validateProvider(prefix string, baseURL string, apiKey string, model string, proxy string) error {
	requiredFields := []struct {
		name  string
		value string
	}{
		{name: prefix + ".base_url", value: baseURL},
		{name: prefix + ".api_key", value: apiKey},
		{name: prefix + ".model", value: model},
	}

	for _, field := range requiredFields {
		if err := validateRequiredString(field.name, field.value); err != nil {
			return err
		}
	}
	if err := validateURL(prefix+".base_url", baseURL); err != nil {
		return err
	}
	return validateOptionalURL(prefix+".proxy", proxy)
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
