package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grimoire/internal/config"
)

type runnerStub struct {
	runErr error
}

func (r runnerStub) Run(context.Context) error {
	return r.runErr
}

func TestRunGeneratesDefaultConfigAndExitsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run(
		"grimoire-bot",
		nil,
		stdout,
		stderr,
		func([]string) (string, bool, error) { return path, true, nil },
		config.Load,
		config.EnsureDefaultConfig,
		func(config.Config, *slog.Logger) (appRunner, error) {
			t.Fatal("buildApp should not be called")
			return nil, nil
		},
	)

	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
	if !strings.Contains(stdout.String(), "已生成模板配置文件") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunMissingExplicitConfigExitsOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run(
		"grimoire-bot",
		[]string{path},
		stdout,
		stderr,
		func([]string) (string, bool, error) { return path, false, nil },
		config.Load,
		config.EnsureDefaultConfig,
		func(config.Config, *slog.Logger) (appRunner, error) {
			t.Fatal("buildApp should not be called")
			return nil, nil
		},
	)

	if code != 1 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config file should not exist: %v", err)
	}
	if !strings.Contains(stderr.String(), "配置文件不存在") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}
