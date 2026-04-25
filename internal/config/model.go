package config

type Config struct {
	Telegram     Telegram     `yaml:"telegram"`
	Storage      Storage      `yaml:"storage"`
	Conversation Conversation `yaml:"conversation"`
	Recovery     Recovery     `yaml:"recovery"`
	LLMs         []LLM        `yaml:"llms"`
	NAI          NAI          `yaml:"nai"`
}

type Telegram struct {
	BotToken    string `yaml:"bot_token"`
	AdminUserID int64  `yaml:"admin_user_id"`
	Proxy       string `yaml:"proxy"`
	TimeoutSec  int    `yaml:"timeout_sec"`
}

type Storage struct {
	DataDir    string `yaml:"data_dir"`
	SQLitePath string `yaml:"sqlite_path"`
	ImageDir   string `yaml:"image_dir"`
}

type Conversation struct {
	RecentMessageLimit int `yaml:"recent_message_limit"`
}

type Recovery struct {
	Enabled *bool `yaml:"enabled"`
}

func (r Recovery) EnabledValue() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

type LLM struct {
	BaseURL         string `yaml:"base_url"`
	APIKey          string `yaml:"api_key"`
	Model           string `yaml:"model"`
	ReasoningEffort string `yaml:"reasoning_effort"`
	Proxy           string `yaml:"proxy"`
	TimeoutSec      int    `yaml:"timeout_sec"`
}

type NAI struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	Proxy      string `yaml:"proxy"`
	TimeoutSec int    `yaml:"timeout_sec"`
}
