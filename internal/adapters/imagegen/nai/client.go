package nai

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	"grimoire/internal/platform/httpclient"
)

const (
	supportedModel = "nai-diffusion-4-5-full"
	promptSuffix   = "location, very aesthetic, masterpiece, no text"
	baseNegative   = "nsfw, lowres, artistic error, film grain, scan artifacts, worst quality, bad quality, jpeg artifacts, very displeasing, chromatic aberration, dithering, halftone, screentone, multiple views, logo, too many watermarks, negative space, blank page"
)

type Client struct {
	cfg        config.Config
	httpClient *http.Client
	logger     *slog.Logger
	now        func() time.Time

	mu        sync.Mutex
	completed map[string][]byte
	nextJobID uint64
}

func NewClient(cfg config.Config, logger *slog.Logger) (*Client, error) {
	if strings.TrimSpace(cfg.NAI.Model) != supportedModel {
		return nil, fmt.Errorf("unsupported nai model %q", cfg.NAI.Model)
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		cfg:        cfg,
		httpClient: httpclient.New(cfg.NAI.TimeoutSec, cfg.NAI.Proxy, logger, "nai"),
		logger:     logger,
		now:        time.Now,
		completed:  make(map[string][]byte),
	}, nil
}

func (c *Client) Submit(ctx context.Context, req domaindraw.GenerateRequest) (string, error) {
	payload, err := c.buildPayload(req)
	if err != nil {
		return "", err
	}

	logSubmitRequest(c.logger, c.cfg.NAI.BaseURL, c.cfg.NAI.Model, 1, req.Shape, req.Artists)

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.NAI.TimeoutSec)*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(c.cfg.NAI.BaseURL, "/") + "/ai/generate-image"
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create nai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.NAI.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("submit nai request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read submit response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("submit nai status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	image, err := extractFirstImage(body)
	if err != nil {
		return "", err
	}

	return c.storeCompletedImage(image), nil
}

func (c *Client) Poll(_ context.Context, jobID string) (domaindraw.JobUpdate, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return domaindraw.JobUpdate{}, fmt.Errorf("job id is required")
	}

	c.mu.Lock()
	image, ok := c.completed[jobID]
	if ok {
		delete(c.completed, jobID)
	}
	c.mu.Unlock()
	if !ok {
		return domaindraw.JobUpdate{}, fmt.Errorf("unknown official nai job %q", jobID)
	}

	return domaindraw.JobUpdate{
		Status:        domaindraw.JobCompleted,
		QueuePosition: 0,
		Image:         append([]byte(nil), image...),
	}, nil
}

func (c *Client) GetBalance(ctx context.Context) (domainnai.AccountBalance, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(c.cfg.NAI.TimeoutSec)*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(c.cfg.NAI.BaseURL, "/") + "/user/data"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domainnai.AccountBalance{}, fmt.Errorf("create user data request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.NAI.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domainnai.AccountBalance{}, fmt.Errorf("query user data: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domainnai.AccountBalance{}, fmt.Errorf("read user data response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return domainnai.AccountBalance{}, fmt.Errorf("user data status=%d body=%s", resp.StatusCode, truncate(string(body), 400))
	}

	var out struct {
		Subscription struct {
			Tier              int  `json:"tier"`
			Active            bool `json:"active"`
			TrainingStepsLeft struct {
				FixedTrainingStepsLeft int `json:"fixedTrainingStepsLeft"`
				PurchasedTrainingSteps int `json:"purchasedTrainingSteps"`
			} `json:"trainingStepsLeft"`
		} `json:"subscription"`
		Information struct {
			TrialImagesLeft int `json:"trialImagesLeft"`
		} `json:"information"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return domainnai.AccountBalance{}, fmt.Errorf("decode user data response: %w", err)
	}

	return domainnai.AccountBalance{
		PurchasedTrainingSteps: out.Subscription.TrainingStepsLeft.PurchasedTrainingSteps,
		FixedTrainingStepsLeft: out.Subscription.TrainingStepsLeft.FixedTrainingStepsLeft,
		TrialImagesLeft:        out.Information.TrialImagesLeft,
		SubscriptionTier:       out.Subscription.Tier,
		SubscriptionActive:     out.Subscription.Active,
	}, nil
}

func (c *Client) buildPayload(req domaindraw.GenerateRequest) ([]byte, error) {
	width, height, err := resolveDimensions(req.Shape)
	if err != nil {
		return nil, err
	}

	prompt := buildPrompt(req.Prompt)
	negativePrompt := buildNegativePrompt(req.NegativePrompt)
	characterPrompts, v4Prompt, v4NegativePrompt, err := buildCharacterMetadata(req.Characters, prompt, negativePrompt)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"action": "generate",
		"input":  prompt,
		"model":  c.cfg.NAI.Model,
		"parameters": map[string]any{
			"add_original_image":                    true,
			"autoSmea":                              false,
			"cfg_rescale":                           0,
			"characterPrompts":                      characterPrompts,
			"controlnet_strength":                   1,
			"deliberate_euler_ancestral_bug":        false,
			"dynamic_thresholding":                  false,
			"height":                                height,
			"inpaintImg2ImgStrength":                1,
			"legacy":                                false,
			"legacy_uc":                             false,
			"legacy_v3_extend":                      false,
			"n_samples":                             1,
			"negative_prompt":                       negativePrompt,
			"noise_schedule":                        "karras",
			"normalize_reference_strength_multiple": true,
			"params_version":                        3,
			"prefer_brownian":                       true,
			"qualityToggle":                         true,
			"sampler":                               "k_euler_ancestral",
			"scale":                                 5,
			"seed":                                  c.now().UnixNano() & 0x7fffffff,
			"sm":                                    false,
			"sm_dyn":                                false,
			"steps":                                 23,
			"uc":                                    negativePrompt,
			"ucPreset":                              4,
			"uncond_scale":                          1,
			"use_coords":                            false,
			"v4_negative_prompt":                    v4NegativePrompt,
			"v4_prompt":                             v4Prompt,
			"width":                                 width,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal nai request: %w", err)
	}
	return data, nil
}

func (c *Client) storeCompletedImage(image []byte) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextJobID++
	jobID := fmt.Sprintf("nai-official-%d-%d", c.now().UnixNano(), c.nextJobID)
	c.completed[jobID] = append([]byte(nil), image...)
	return jobID
}

func buildPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return promptSuffix
	}
	return prompt + ", " + promptSuffix
}

func buildNegativePrompt(extra string) string {
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return baseNegative
	}
	return baseNegative + ", " + extra
}

func buildCharacterMetadata(characters []domaindraw.CharacterPrompt, prompt string, negativePrompt string) ([]map[string]any, map[string]any, map[string]any, error) {
	characterPrompts := make([]map[string]any, 0, len(characters))
	v4Chars := make([]map[string]any, 0, len(characters))
	v4NegativeChars := make([]map[string]any, 0, len(characters))

	for _, character := range characters {
		center, err := positionToCoordinates(character.Position)
		if err != nil {
			return nil, nil, nil, err
		}

		charPrompt := strings.TrimSpace(character.Prompt)
		charNegative := strings.TrimSpace(character.NegativePrompt)
		characterPrompts = append(characterPrompts, map[string]any{
			"center": center,
			"prompt": charPrompt,
			"uc":     charNegative,
		})
		v4Chars = append(v4Chars, map[string]any{
			"centers":      []map[string]any{center},
			"char_caption": charPrompt,
		})
		v4NegativeChars = append(v4NegativeChars, map[string]any{
			"centers":      []map[string]any{center},
			"char_caption": charNegative,
		})
	}

	v4Prompt := map[string]any{
		"caption": map[string]any{
			"base_caption":  prompt,
			"char_captions": v4Chars,
		},
		"use_coords": false,
		"use_order":  true,
	}
	v4NegativePrompt := map[string]any{
		"caption": map[string]any{
			"base_caption":  negativePrompt,
			"char_captions": v4NegativeChars,
		},
		"legacy_uc": false,
	}

	return characterPrompts, v4Prompt, v4NegativePrompt, nil
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

func positionToCoordinates(position string) (map[string]any, error) {
	position = strings.ToUpper(strings.TrimSpace(position))
	if len(position) != 2 {
		return nil, fmt.Errorf("invalid character position %q", position)
	}

	column := position[0]
	row := position[1]
	if column < 'A' || column > 'E' || row < '1' || row > '5' {
		return nil, fmt.Errorf("invalid character position %q", position)
	}

	return map[string]any{
		"x": roundToOneDecimal(0.5 + 0.2*float64(int(column)-int('C'))),
		"y": roundToOneDecimal(0.5 + 0.2*float64(int(row)-int('3'))),
	}, nil
}

func extractFirstImage(data []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open generated zip: %w", err)
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open generated image %s: %w", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read generated image %s: %w", file.Name, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close generated image %s: %w", file.Name, closeErr)
		}
		if len(content) == 0 {
			continue
		}
		return content, nil
	}

	return nil, fmt.Errorf("generated zip did not contain any image")
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

func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
