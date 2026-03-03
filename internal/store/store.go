package store

import (
	"context"
	"errors"
	"time"

	"grimoire/internal/types"
)

var ErrNotFound = errors.New("not found")

type GalleryItem struct {
	ID        int64
	ChatID    int64
	MessageID int64
	TaskID    string
	JobID     string
	FilePath  string
	Caption   string
	CreatedAt time.Time
}

type TaskStore interface {
	Init(ctx context.Context) error
	NextTaskID(ctx context.Context) (string, error)
	CreateInboundMessage(ctx context.Context, chatID, userID, messageID int64, text string, createdAt time.Time) error
	CreateTask(ctx context.Context, task types.DrawTask) error
	UpdateTaskStatus(ctx context.Context, taskID string, status string, stage string, errMsg string) error
	SetTaskJobID(ctx context.Context, taskID string, jobID string) error
	SaveTaskResult(ctx context.Context, taskID string, jobID string, filePath string, completedAt time.Time) error
	GetTaskByID(ctx context.Context, taskID string) (types.DrawTask, error)
	ListRecoverableTasks(ctx context.Context) ([]types.DrawTask, error)
	AppendGalleryItem(ctx context.Context, chatID, messageID int64, taskID, jobID, filePath, caption string, createdAt time.Time) error
	ListGalleryItems(ctx context.Context, chatID, messageID int64) ([]GalleryItem, error)
}
