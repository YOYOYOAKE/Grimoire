package config

import (
	"errors"
	"fmt"
	neturl "net/url"
)

func validate(cfg Config) error {
	if cfg.Telegram.BotToken == "" {
		return fmt.Errorf("环境变量 %s 不能为空", EnvTelegramBotToken)
	}
	if cfg.Telegram.AdminUserID <= 0 {
		return fmt.Errorf("环境变量 %s 必须 > 0", EnvTelegramAdminUserID)
	}
	if cfg.Telegram.ProxyURL != "" {
		parsed, err := neturl.Parse(cfg.Telegram.ProxyURL)
		if err != nil {
			return fmt.Errorf("环境变量 %s 非法: %w", EnvTelegramProxyURL, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("环境变量 %s 必须包含 scheme 和 host", EnvTelegramProxyURL)
		}
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
