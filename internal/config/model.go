package config

type Config struct {
	Telegram Telegram `yaml:"telegram"`
	LLMs     []LLM    `yaml:"llms"`
	NAI      NAI      `yaml:"nai"`
}

type Telegram struct {
	BotToken    string `yaml:"bot_token"`
	AdminUserID int64  `yaml:"admin_user_id"`
	Proxy       string `yaml:"proxy"`
	TimeoutSec  int    `yaml:"timeout_sec"`
}

type LLM struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	Proxy      string `yaml:"proxy"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

type NAI struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	Proxy      string `yaml:"proxy"`
	TimeoutSec int    `yaml:"timeout_sec"`
}
