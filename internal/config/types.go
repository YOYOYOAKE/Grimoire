package config

type Config struct {
	Telegram   TelegramConfig
	LLM        LLMConfig
	NAI        NAIConfig
	Generation GenerationConfig
	Runtime    RuntimeConfig
}

type TelegramConfig struct {
	BotToken    string
	AdminUserID int64
	ProxyURL    string
	TimeoutSec  int
}

type LLMConfig struct {
	Provider   string
	BaseURL    string
	APIKey     string
	Model      string
	TimeoutSec int
	Proxy      string
}

type NAIConfig struct {
	BaseURL         string
	APIKey          string
	Model           string
	TimeoutSec      int
	Proxy           string
	PollIntervalSec int
}

type GenerationConfig struct {
	ShapeDefault string
	Artist       string
	ShapeMap     map[string]string
	Steps        int
	Scale        int
	Sampler      string
	NSamples     int
}

type RuntimeConfig struct {
	WorkerConcurrency int
	SaveDir           string
	SQLitePath        string
}

type yamlConfig struct {
	Telegram yamlTelegramConfig `yaml:"telegram"`
	LLM      yamlLLMConfig      `yaml:"llm"`
	NAI      yamlNAIConfig      `yaml:"nai"`
}

type yamlTelegramConfig struct {
	BotToken    string `yaml:"bot_token"`
	AdminUserID int64  `yaml:"admin_user_id"`
	Proxy       string `yaml:"proxy"`
	ProxyURL    string `yaml:"proxy_url"`
	TimeoutSec  int    `yaml:"timeout_sec"`
}

type yamlLLMConfig struct {
	TimeoutSec   int                 `yaml:"timeout_sec"`
	OpenAICustom yamlOpenAICustomLLM `yaml:"openai_custom"`
	OpenRouter   yamlOpenRouterLLM   `yaml:"openrouter"`
}

type yamlOpenAICustomLLM struct {
	Enable  bool   `yaml:"enable"`
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	Proxy   string `yaml:"proxy"`
}

type yamlOpenRouterLLM struct {
	Enable bool   `yaml:"enable"`
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
	Proxy  string `yaml:"proxy"`
}

type yamlNAIConfig struct {
	BaseURL    string `yaml:"base_url"`
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	TimeoutSec int    `yaml:"timeout_sec"`
	Proxy      string `yaml:"proxy"`
}
