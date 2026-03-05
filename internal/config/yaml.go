package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadBaseConfigFromFile(configPath string, sqlitePath string) (Config, error) {
	path := strings.TrimSpace(configPath)
	if path == "" {
		path = DefaultConfigPath
	}

	raw, err := loadYAMLConfig(path)
	if err != nil {
		return Config{}, err
	}
	cfg, err := buildBaseConfigFromYAML(raw, sqlitePath)
	if err != nil {
		return Config{}, fmt.Errorf("配置校验失败 (%s): %w", path, err)
	}
	return cfg, nil
}

func loadYAMLConfig(path string) (yamlConfig, error) {
	clean := filepath.Clean(path)
	f, err := os.Open(clean)
	if err != nil {
		return yamlConfig{}, fmt.Errorf("读取配置文件失败 (%s): %w", clean, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var cfg yamlConfig
	if err := dec.Decode(&cfg); err != nil {
		return yamlConfig{}, fmt.Errorf("解析配置文件失败 (%s): %w", clean, err)
	}
	return cfg, nil
}
