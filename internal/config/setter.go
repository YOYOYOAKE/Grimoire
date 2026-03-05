package config

import (
	"fmt"
	"strconv"
	"strings"
)

func (m *Manager) SetByPath(path string, value string) error {
	path = strings.TrimSpace(strings.ToLower(path))
	value = strings.TrimSpace(value)

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	switch path {
	case "telegram.bot_token":
		cfg.Telegram.BotToken = value
	case "telegram.admin_user_id":
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("telegram.admin_user_id 必须是整数: %w", err)
		}
		cfg.Telegram.AdminUserID = v
	case "telegram.proxy_url":
		cfg.Telegram.ProxyURL = value
	case "llm.base_url":
		cfg.LLM.BaseURL = value
	case "llm.api_key":
		cfg.LLM.APIKey = value
	case "llm.model":
		cfg.LLM.Model = value
	case "nai.base_url":
		cfg.NAI.BaseURL = value
	case "nai.api_key":
		cfg.NAI.APIKey = value
	case "nai.model":
		cfg.NAI.Model = value
	case "generation.shape_default":
		cfg.Generation.ShapeDefault = strings.ToLower(value)
	case "generation.artist":
		cfg.Generation.Artist = value
	case "runtime.sqlite_path":
		cfg.Runtime.SQLitePath = value
	default:
		return fmt.Errorf("不支持的配置键: %s", path)
	}

	return m.Save(cfg)
}
