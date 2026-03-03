package nai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/types"
)

type XianyunClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

func NewXianyunClient(cfg *config.Manager, logger *slog.Logger) *XianyunClient {
	return &XianyunClient{
		cfg:        cfg,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

func (c *XianyunClient) Submit(ctx context.Context, req types.GenerateRequest) (string, error) {
	cfg := c.cfg.Snapshot()
	width, height, err := resolveDimensions(cfg.Generation.ShapeMap, req.Shape, cfg.Generation.ShapeDefault)
	if err != nil {
		return "", err
	}
	start := time.Now()
	c.logger.Info("nai submit request start",
		"model", cfg.NAI.Model,
		"shape", req.Shape,
		"width", width,
		"height", height,
		"positive_len", len(req.PositivePrompt),
		"negative_len", len(req.NegativePrompt),
		"character_count", len(req.Characters),
	)

	seed := time.Now().UnixNano() & 0x7fffffff
	if req.Seed != nil {
		seed = *req.Seed
	}

	payload := map[string]any{
		"model":                          cfg.NAI.Model,
		"positivePrompt":                 req.PositivePrompt,
		"negativePrompt":                 req.NegativePrompt,
		"qualityToggle":                  false,
		"scale":                          cfg.Generation.Scale,
		"steps":                          cfg.Generation.Steps,
		"width":                          width,
		"height":                         height,
		"promptGuidanceRescale":          0,
		"noise_schedule":                 "karras",
		"seed":                           seed,
		"sampler":                        cfg.Generation.Sampler,
		"sm":                             false,
		"sm_dyn":                         false,
		"decrisp":                        false,
		"variety":                        false,
		"n_samples":                      cfg.Generation.NSamples,
		"prefer_brownian":                true,
		"deliberate_euler_ancestral_bug": false,
		"legacy":                         false,
		"legacy_uc":                      false,
		"legacy_v3_extend":               false,
		"ucPreset":                       1,
		"autoSmea":                       false,
		"use_coords":                     false,
		"use_upscale_credits":            false,
	}
	if len(req.Characters) > 0 {
		characterPrompts, v4Prompts, v4NegativePrompts := buildCharacterPayloads(req.Characters)
		payload["characterPrompts"] = characterPrompts
		payload["v4_prompt_char_captions"] = v4Prompts
		payload["v4_negative_prompt_char_captions"] = v4NegativePrompts
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化绘图请求失败: %w", err)
	}

	url := strings.TrimRight(cfg.NAI.BaseURL, "/") + "/generate_image"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("创建绘图请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.NAI.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("nai submit request failed",
			"model", cfg.NAI.Model,
			"shape", req.Shape,
			"duration_ms", time.Since(start).Milliseconds(),
			"error", err,
		)
		return "", fmt.Errorf("提交绘图任务失败: %w", err)
	}
	defer resp.Body.Close()
	c.logger.Info("nai submit response received",
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取提交响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("提交任务失败: status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("解析提交响应失败: %w", err)
	}
	if strings.TrimSpace(out.JobID) == "" {
		return "", fmt.Errorf("提交响应缺少 job_id")
	}

	c.logger.Info("nai submit ok",
		"job_id", out.JobID,
		"shape", req.Shape,
		"width", width,
		"height", height,
		"character_count", len(req.Characters),
		"has_v4_character_payload", len(req.Characters) > 0,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return out.JobID, nil
}

func buildCharacterPayloads(chars []types.CharacterPrompt) ([]map[string]any, []map[string]any, []map[string]any) {
	characterPrompts := make([]map[string]any, 0, len(chars))
	v4Prompts := make([]map[string]any, 0, len(chars))
	v4NegativePrompts := make([]map[string]any, 0, len(chars))

	for _, ch := range chars {
		center := map[string]any{
			"x": ch.CenterX,
			"y": ch.CenterY,
		}
		characterPrompts = append(characterPrompts, map[string]any{
			"prompt": ch.PositivePrompt,
			"uc":     ch.NegativePrompt,
			"center": center,
		})
		v4Prompts = append(v4Prompts, map[string]any{
			"char_caption": ch.PositivePrompt,
			"centers": []map[string]any{
				{"x": ch.CenterX, "y": ch.CenterY},
			},
		})
		v4NegativePrompts = append(v4NegativePrompts, map[string]any{
			"char_caption": ch.NegativePrompt,
			"centers": []map[string]any{
				{"x": ch.CenterX, "y": ch.CenterY},
			},
		})
	}

	return characterPrompts, v4Prompts, v4NegativePrompts
}

func (c *XianyunClient) Poll(ctx context.Context, jobID string) (types.JobResult, error) {
	cfg := c.cfg.Snapshot()
	url := strings.TrimRight(cfg.NAI.BaseURL, "/") + "/get_result/" + jobID

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("创建轮询请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.NAI.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("轮询请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.JobResult{}, fmt.Errorf("读取轮询响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return types.JobResult{}, fmt.Errorf("轮询失败: status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return types.JobResult{}, fmt.Errorf("解析轮询响应失败: %w", err)
	}

	result := types.JobResult{}
	result.Status = toString(raw["status"])
	result.ImageBase64 = toString(raw["image_base64"])
	result.Error = toString(raw["error"])

	if q, ok := raw["queue_position"]; ok {
		result.QueuePosition = toInt(q)
	}

	if result.Status == "" {
		return types.JobResult{}, fmt.Errorf("轮询响应缺少 status")
	}

	return result, nil
}

func resolveDimensions(shapeMap map[string]string, shape string, defaultShape string) (int, int, error) {
	if shape == "" {
		shape = defaultShape
	}
	dims := shapeMap[shape]
	if dims == "" {
		return 0, 0, fmt.Errorf("未配置 shape=%s 的尺寸", shape)
	}
	parts := strings.Split(dims, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("非法尺寸格式: %s", dims)
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("非法宽度: %w", err)
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("非法高度: %w", err)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("宽高必须 > 0")
	}
	return w, h, nil
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}
