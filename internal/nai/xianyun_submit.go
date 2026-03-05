package nai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"grimoire/internal/types"
)

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
