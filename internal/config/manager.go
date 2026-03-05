package config

import "sync"

type Manager struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

func NewManager(path string) (*Manager, error) {
	cfg, err := LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	return &Manager{path: path, cfg: cfg}, nil
}

func (m *Manager) Path() string {
	return m.path
}

func (m *Manager) Snapshot() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Load() (Config, error) {
	return m.Snapshot(), nil
}

func (m *Manager) Save(cfg Config) error {
	cfg = applyDefaults(cfg)
	if err := validate(cfg); err != nil {
		return err
	}
	if err := saveToFileAtomic(m.path, cfg); err != nil {
		return err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return nil
}
