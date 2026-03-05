package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/types"
)

type OpenAIClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

type parseOutputError struct {
	cause error
}

func (e *parseOutputError) Error() string {
	if e == nil || e.cause == nil {
		return "parse output error"
	}
	return e.cause.Error()
}

func NewOpenAIClient(cfg *config.Manager, logger *slog.Logger) *OpenAIClient {
	return &OpenAIClient{
		cfg:        cfg,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

func (c *OpenAIClient) Translate(ctx context.Context, naturalText string, shape string) (types.TranslationResult, error) {
	cfg := c.cfg.Snapshot()
	requestTimeout := time.Duration(cfg.LLM.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	systemPrompt := `
	You translate Chinese natural language image requests into NovelAI-friendly English tag prompts.

	Output rules:
	1) Output JSON only, no extra text.
	2) Schema:
	{
	  "positivePrompt":"...",
	  "negativePrompt":"...",
	  "characterPrompts":[
	    {
	      "charPositivePrompt":"...",
	      "charUnconcentPrompt":"...",
	      "centers":{"x":1,"y":"A"}
	    }
	  ]
	}
	3) positivePrompt and negativePrompt are required.
	4) characterPrompts can be empty.
	5) If characterPrompts is not empty, each item must include non-empty:
	   - charPositivePrompt
	   - charUnconcentPrompt
	   - centers.x
	   - centers.y
	6) centers.x must be integer 1..5 where 1=top and 5=bottom.
	7) centers.y must be letter A..E where A=left and E=right.
	`
	userPrompt := fmt.Sprintf("shape=%s\nrequest=%s", shape, naturalText)

	result, err := c.translateAttempt(ctx, cfg, shape, naturalText, systemPrompt, userPrompt, 1)
	if err == nil {
		return result, nil
	}

	var parseErr *parseOutputError
	if !errors.As(err, &parseErr) {
		return types.TranslationResult{}, err
	}

	c.logger.Warn("llm parse failed, retry once",
		"shape", shape,
		"error", parseErr.cause,
	)
	retryUserPrompt := userPrompt + "\n\nYour previous output was invalid. Return valid JSON only and follow the schema strictly."

	result, retryErr := c.translateAttempt(ctx, cfg, shape, naturalText, systemPrompt, retryUserPrompt, 2)
	if retryErr == nil {
		c.logger.Info("llm parse retry succeeded", "shape", shape, "character_count", len(result.Characters))
		return result, nil
	}

	if errors.As(retryErr, &parseErr) {
		return types.TranslationResult{}, parseErr.cause
	}
	return types.TranslationResult{}, retryErr
}
