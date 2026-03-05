package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (o *Orchestrator) saveImage(imageB64 string, taskID string, jobID string) (string, error) {
	if strings.TrimSpace(imageB64) == "" {
		return "", fmt.Errorf("空图片数据")
	}
	decoded, err := base64.StdEncoding.DecodeString(imageB64)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}

	cfg := o.cfg.Snapshot()
	day := time.Now().Format("20060102")
	dir := filepath.Join(cfg.Runtime.SaveDir, day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	name := fmt.Sprintf("%s_%s_%s.png", time.Now().Format("150405"), sanitize(taskID), sanitize(jobID))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, decoded, 0o644); err != nil {
		return "", fmt.Errorf("写入图片失败: %w", err)
	}
	return path, nil
}

func sanitize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")
	v = strings.ReplaceAll(v, " ", "_")
	if v == "" {
		return "unknown"
	}
	return v
}
