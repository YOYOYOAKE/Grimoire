package types

import (
	"context"
	"time"

	"grimoire/internal/config"
)

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type GenerateRequest struct {
	PositivePrompt string
	NegativePrompt string
	Shape          string
	Seed           *int64
	Characters     []CharacterPrompt
}

type CharacterPrompt struct {
	PositivePrompt string
	NegativePrompt string
	CenterX        float64
	CenterY        float64
}

type TranslationResult struct {
	PositivePrompt string
	NegativePrompt string
	Characters     []CharacterPrompt
}

type JobResult struct {
	Status        string
	QueuePosition int
	ImageBase64   string
	Error         string
}

type DrawTask struct {
	TaskID          string
	ChatID          int64
	UserID          int64
	StatusMessageID int64
	Prompt          string
	Shape           string
	Seed            *int64
	ResumeJobID     string
	RetryOfTaskID   string
	CreatedAt       time.Time
	StartedAt       time.Time
	FinishedAt      time.Time
}

type QueueStats struct {
	Pending       int
	Running       bool
	CurrentTaskID string
}

type Translator interface {
	Translate(ctx context.Context, naturalText string, shape string) (TranslationResult, error)
}

type ImageGenerator interface {
	Submit(ctx context.Context, req GenerateRequest) (jobID string, err error)
	Poll(ctx context.Context, jobID string) (JobResult, error)
}

type TaskQueue interface {
	Enqueue(task DrawTask) (taskID string, queuePos int)
}

type ConfigStore interface {
	Load() (config.Config, error)
	Save(cfg config.Config) error
}

type Notifier interface {
	NotifyText(ctx context.Context, chatID int64, text string) (int64, error)
	EditText(ctx context.Context, chatID int64, messageID int64, text string) error
	EditPhoto(ctx context.Context, chatID int64, messageID int64, filePath string, caption string) error
	NotifyPhoto(ctx context.Context, chatID int64, filePath string, caption string) error
}
