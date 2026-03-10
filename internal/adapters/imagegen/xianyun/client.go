package xianyun

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	"grimoire/internal/platform/httpclient"
)

const (
	submitAttempts   = 3
	submitRetryDelay = time.Second
)

type Client struct {
	cfg        config.Config
	httpClient *http.Client
	logger     *slog.Logger
	wait       func(ctx context.Context, delay time.Duration) error
}

func NewClient(cfg config.Config, logger *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.NAI.TimeoutSec, cfg.NAI.Proxy, logger, "nai"),
		logger:     logger,
		wait:       waitWithContext,
	}
}

func (c *Client) Submit(ctx context.Context, req domaindraw.GenerateRequest) (string, error) {
	width, height, err := resolveDimensions(req.Shape)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"model":                          c.cfg.NAI.Model,
		"positivePrompt":                 req.PositivePrompt,
		"negativePrompt":                 req.NegativePrompt,
		"qualityToggle":                  false,
		"scale":                          5,
		"steps":                          28,
		"width":                          width,
		"height":                         height,
		"promptGuidanceRescale":          0,
		"noise_schedule":                 "karras",
		"seed":                           time.Now().UnixNano() & 0x7fffffff,
		"sampler":                        "k_euler",
		"sm":                             false,
		"sm_dyn":                         false,
		"decrisp":                        false,
		"variety":                        false,
		"n_samples":                      1,
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

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal nai request: %w", err)
	}

	endpoint := strings.TrimRight(c.cfg.NAI.BaseURL, "/") + "/generate_image"
	var lastErr error
	wait := c.wait
	if wait == nil {
		wait = waitWithContext
	}
	for attempt := 1; attempt <= submitAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		logSubmitRequest(c.logger, c.cfg.NAI.BaseURL, c.cfg.NAI.Model, attempt, req.Shape, req.Artists)

		jobID, retryable, err := c.submitOnce(ctx, endpoint, data)
		if err == nil {
			if c.logger != nil {
				c.logger.Info("nai submitted", "job_id", jobID, "shape", req.Shape)
			}
			return jobID, nil
		}

		lastErr = err
		if !retryable || attempt == submitAttempts {
			return "", err
		}

		if c.logger != nil {
			c.logger.Warn(
				"nai submit attempt failed",
				"base_url", strings.TrimSpace(c.cfg.NAI.BaseURL),
				"model", c.cfg.NAI.Model,
				"attempt", attempt,
				"shape", req.Shape,
				"artists", strings.TrimSpace(req.Artists),
				"error", err,
			)
		}

		if err := wait(ctx, submitRetryDelay); err != nil {
			return "", err
		}
	}

	return "", lastErr
}

func (c *Client) Poll(ctx context.Context, jobID string) (domaindraw.JobUpdate, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.NAI.TimeoutSec)*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(c.cfg.NAI.BaseURL, "/") + "/get_result/" + strings.TrimSpace(jobID)
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domaindraw.JobUpdate{}, fmt.Errorf("create poll request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.NAI.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return domaindraw.JobUpdate{}, fmt.Errorf("poll nai request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domaindraw.JobUpdate{}, fmt.Errorf("read poll response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domaindraw.JobUpdate{}, fmt.Errorf("poll nai status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var raw struct {
		Status        string `json:"status"`
		QueuePosition int    `json:"queue_position"`
		ImageBase64   string `json:"image_base64"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return domaindraw.JobUpdate{}, fmt.Errorf("decode poll response: %w", err)
	}

	update := domaindraw.JobUpdate{
		Status:        domaindraw.JobStatus(strings.ToLower(strings.TrimSpace(raw.Status))),
		QueuePosition: raw.QueuePosition,
		Error:         strings.TrimSpace(raw.Error),
	}
	if strings.TrimSpace(raw.ImageBase64) != "" {
		update.Image, err = base64.StdEncoding.DecodeString(raw.ImageBase64)
		if err != nil {
			return domaindraw.JobUpdate{}, fmt.Errorf("decode image base64: %w", err)
		}
	}
	if update.Status == "" {
		return domaindraw.JobUpdate{}, fmt.Errorf("poll response missing status")
	}
	return update, nil
}

func resolveDimensions(shape domaindraw.Shape) (int, int, error) {
	switch shape {
	case domaindraw.ShapeSquare:
		return 1024, 1024, nil
	case domaindraw.ShapeLandscape:
		return 1216, 832, nil
	case domaindraw.ShapePortrait:
		return 832, 1216, nil
	default:
		return 0, 0, fmt.Errorf("unsupported shape %q", shape)
	}
}

func truncate(v string, limit int) string {
	if len(v) <= limit {
		return v
	}
	return v[:limit] + "..."
}

func (c *Client) submitOnce(ctx context.Context, endpoint string, data []byte) (string, bool, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.NAI.TimeoutSec)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", false, fmt.Errorf("create nai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.NAI.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", true, fmt.Errorf("submit nai request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, fmt.Errorf("read submit response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", true, fmt.Errorf("submit nai status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var out struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", true, fmt.Errorf("decode submit response: %w", err)
	}
	if strings.TrimSpace(out.JobID) == "" {
		return "", true, fmt.Errorf("submit response missing job_id")
	}
	return out.JobID, true, nil
}

func logSubmitRequest(logger *slog.Logger, baseURL string, model string, attempt int, shape domaindraw.Shape, artists string) {
	if logger == nil {
		return
	}

	logger.Info(
		"nai request started",
		"base_url", strings.TrimSpace(baseURL),
		"model", model,
		"attempt", attempt,
		"shape", shape,
		"artists", strings.TrimSpace(artists),
	)
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
