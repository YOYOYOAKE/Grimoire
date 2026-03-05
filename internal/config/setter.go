package config

import (
	"fmt"
	"strings"
)

func (m *Manager) SetByPath(path string, value string) error {
	path = strings.TrimSpace(strings.ToLower(path))
	value = strings.TrimSpace(value)

	switch path {
	case "generation.shape_default", "generation.artist":
		// keep configurable at runtime via /img menu
	default:
		if strings.HasPrefix(path, "llm.") || strings.HasPrefix(path, "nai.") {
			return fmt.Errorf("%s 已迁移到 %s，请修改配置文件后重启", path, DefaultConfigPath)
		}
		return fmt.Errorf("不支持的配置键: %s", path)
	}

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	if err := applyManagedValue(&cfg, path, value); err != nil {
		return fmt.Errorf("不支持的配置键: %s", path)
	}

	return m.Save(cfg)
}
