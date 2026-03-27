package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"grimoire/internal/bootstrap"
	"grimoire/internal/config"
)

type appRunner interface {
	Run(ctx context.Context) error
}

func main() {
	os.Exit(run(
		os.Args[0],
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		config.ResolveStartupPath,
		config.Load,
		config.EnsureDefaultConfig,
		func(cfg config.Config, configPath string, logger *slog.Logger) (appRunner, error) {
			return bootstrap.NewApp(cfg, configPath, logger)
		},
	))
}

func run(
	programName string,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	resolveStartupPath func(args []string) (path string, usedDefault bool, err error),
	loadConfig func(path string) (config.Config, error),
	ensureDefaultConfig func(path string) error,
	buildApp func(cfg config.Config, configPath string, logger *slog.Logger) (appRunner, error),
) int {
	logger := slog.New(slog.NewTextHandler(stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	configPath, usedDefault, err := resolveStartupPath(args)
	if err != nil {
		fmt.Fprintf(stderr, "用法: %s [config-path]\n", filepath.Base(programName))
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 1
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if usedDefault {
				if err := ensureDefaultConfig(configPath); err != nil {
					logger.Error("generate default config failed", "path", configPath, "error", err)
					fmt.Fprintf(stderr, "生成默认配置失败: %v\n", err)
					return 1
				}
				fmt.Fprintf(stdout, "已生成模板配置文件: %s\n请填写后重新启动。\n", configPath)
				return 0
			}
			fmt.Fprintf(stderr, "配置文件不存在: %s\n", configPath)
			return 1
		}
		logger.Error("load config failed", "path", configPath, "error", err)
		fmt.Fprintf(stderr, "加载配置失败: %v\n", err)
		return 1
	}

	app, err := buildApp(cfg, configPath, logger)
	if err != nil {
		logger.Error("bootstrap app failed", "error", err)
		fmt.Fprintf(stderr, "应用初始化失败: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		logger.Error("app run failed", "error", err)
		fmt.Fprintf(stderr, "运行失败: %v\n", err)
		return 1
	}
	return 0
}
