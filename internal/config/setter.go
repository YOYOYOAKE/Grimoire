package config

import (
	"fmt"
	"strings"
)

func (m *Manager) SetByPath(path string, value string) error {
	path = strings.TrimSpace(strings.ToLower(path))
	value = strings.TrimSpace(value)

	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	if err := applyManagedValue(&cfg, path, value); err != nil {
		return fmt.Errorf("不支持的配置键: %s", path)
	}

	return m.Save(cfg)
}
