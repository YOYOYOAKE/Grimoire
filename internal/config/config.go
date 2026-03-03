package config

import (
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram   TelegramConfig   `yaml:"telegram"`
	LLM        LLMConfig        `yaml:"llm"`
	NAI        NAIConfig        `yaml:"nai"`
	Generation GenerationConfig `yaml:"generation"`
	Runtime    RuntimeConfig    `yaml:"runtime"`
}

type TelegramConfig struct {
	BotToken    string `yaml:"bot_token"`
	AdminUserID int64  `yaml:"admin_user_id"`
	ProxyURL    string `yaml:"proxy_url"`
}

type LLMConfig struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

type NAIConfig struct {
	BaseURL         string `yaml:"base_url"`
	APIKey          string `yaml:"api_key"`
	Model           string `yaml:"model"`
	PollIntervalSec int    `yaml:"poll_interval_sec"`
}

type GenerationConfig struct {
	ShapeDefault string            `yaml:"shape_default"`
	Artist       string            `yaml:"artist"`
	ShapeMap     map[string]string `yaml:"shape_map"`
	Steps        int               `yaml:"steps"`
	Scale        int               `yaml:"scale"`
	Sampler      string            `yaml:"sampler"`
	NSamples     int               `yaml:"n_samples"`
}

type RuntimeConfig struct {
	WorkerConcurrency int    `yaml:"worker_concurrency"`
	SaveDir           string `yaml:"save_dir"`
	SQLitePath        string `yaml:"sqlite_path"`
}

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

func LoadFromFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置失败: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析 YAML 失败: %w", err)
	}
	cfg = applyDefaults(cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

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

func saveToFileAtomic(path string, cfg Config) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("序列化 YAML 失败: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpName)
	}
	defer cleanup()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync 临时文件失败: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("设置配置权限失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换配置失败: %w", err)
	}

	d, err := os.Open(dir)
	if err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func MaskSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "****" + v[len(v)-4:]
}
