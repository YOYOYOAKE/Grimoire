package config

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
