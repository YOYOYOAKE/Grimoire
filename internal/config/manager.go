package config

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

const createAppConfigTableSQL = `
CREATE TABLE IF NOT EXISTS app_config (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL,
	updated_at DATETIME NOT NULL
);`

var managedConfigPaths = []string{
	"llm.base_url",
	"llm.api_key",
	"llm.model",
	"nai.api_key",
	"nai.model",
	"generation.shape_default",
	"generation.artist",
}

type Manager struct {
	path string
	db   *sql.DB

	mu  sync.RWMutex
	cfg Config
}

func NewManager(path string) (*Manager, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultSQLitePath
	}
	db, err := openConfigDB(path)
	if err != nil {
		return nil, err
	}

	m := &Manager{path: path, db: db}
	ctx := context.Background()
	if err := m.initConfigStore(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := m.bootstrapConfig(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	cfg, err := m.loadConfig(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	m.cfg = cfg
	return m, nil
}

func (m *Manager) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	return m.db.Close()
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
	cfg = normalizeConfig(cfg)
	if err := validate(cfg); err != nil {
		return err
	}

	ctx := context.Background()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin config tx failed: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, key := range managedConfigPaths {
		value, err := managedValue(cfg, key)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO app_config(key, value, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
	value = excluded.value,
	updated_at = excluded.updated_at;
`, key, value); err != nil {
			return fmt.Errorf("save config key %s failed: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit config tx failed: %w", err)
	}

	latest, err := m.loadConfig(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.cfg = latest
	m.mu.Unlock()
	return nil
}

func (m *Manager) MissingDrawConfigKeys() []string {
	cfg := m.Snapshot()
	missing := make([]string, 0, 5)
	if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
		missing = append(missing, "llm.base_url")
	}
	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		missing = append(missing, "llm.api_key")
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		missing = append(missing, "llm.model")
	}
	if strings.TrimSpace(cfg.NAI.APIKey) == "" {
		missing = append(missing, "nai.api_key")
	}
	if strings.TrimSpace(cfg.NAI.Model) == "" {
		missing = append(missing, "nai.model")
	}
	return missing
}

func (m *Manager) loadConfig(ctx context.Context) (Config, error) {
	cfg, err := buildBaseConfig(m.path)
	if err != nil {
		return Config{}, err
	}

	rows, err := m.db.QueryContext(ctx, `SELECT key, value FROM app_config`)
	if err != nil {
		return Config{}, fmt.Errorf("query app_config failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return Config{}, fmt.Errorf("scan app_config failed: %w", err)
		}
		if err := applyManagedValue(&cfg, key, value); err != nil {
			// Ignore unknown keys for forward compatibility.
			continue
		}
	}
	if err := rows.Err(); err != nil {
		return Config{}, fmt.Errorf("iterate app_config failed: %w", err)
	}

	cfg = normalizeConfig(cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (m *Manager) initConfigStore(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, createAppConfigTableSQL); err != nil {
		return fmt.Errorf("create app_config table failed: %w", err)
	}
	return nil
}

func (m *Manager) bootstrapConfig(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, `
INSERT INTO app_config(key, value, updated_at)
VALUES
	('generation.shape_default', 'square', CURRENT_TIMESTAMP),
	('generation.artist', '', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO NOTHING;
`); err != nil {
		return fmt.Errorf("bootstrap app_config failed: %w", err)
	}
	return nil
}

func openConfigDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite dir failed: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_busy_timeout=10000&_journal_mode=WAL", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func managedValue(cfg Config, path string) (string, error) {
	switch path {
	case "llm.base_url":
		return cfg.LLM.BaseURL, nil
	case "llm.api_key":
		return cfg.LLM.APIKey, nil
	case "llm.model":
		return cfg.LLM.Model, nil
	case "nai.api_key":
		return cfg.NAI.APIKey, nil
	case "nai.model":
		return cfg.NAI.Model, nil
	case "generation.shape_default":
		return cfg.Generation.ShapeDefault, nil
	case "generation.artist":
		return cfg.Generation.Artist, nil
	default:
		return "", fmt.Errorf("不支持的配置键: %s", path)
	}
}

func applyManagedValue(cfg *Config, path string, value string) error {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "llm.base_url":
		cfg.LLM.BaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
	case "llm.api_key":
		cfg.LLM.APIKey = strings.TrimSpace(value)
	case "llm.model":
		cfg.LLM.Model = strings.TrimSpace(value)
	case "nai.api_key":
		cfg.NAI.APIKey = strings.TrimSpace(value)
	case "nai.model":
		cfg.NAI.Model = strings.TrimSpace(value)
	case "generation.shape_default":
		cfg.Generation.ShapeDefault = strings.ToLower(strings.TrimSpace(value))
	case "generation.artist":
		cfg.Generation.Artist = strings.TrimSpace(value)
	default:
		return fmt.Errorf("不支持的配置键: %s", path)
	}
	return nil
}
