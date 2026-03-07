package draw

import (
	"context"
	"errors"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

var ErrTaskNotFound = errors.New("task not found")

type TaskRepository interface {
	Create(ctx context.Context, task domaindraw.Task) error
	Get(ctx context.Context, taskID string) (domaindraw.Task, error)
	Update(ctx context.Context, task domaindraw.Task) error
	Delete(ctx context.Context, taskID string) error
}

type PreferenceRepository interface {
	GetByUserID(ctx context.Context, userID int64) (domainpreferences.UserPreference, error)
}

type Scheduler interface {
	Enqueue(taskID string) int
}

type PromptTranslator interface {
	Translate(ctx context.Context, prompt string, shape domaindraw.Shape) (domaindraw.Translation, error)
}

type ImageGenerator interface {
	Submit(ctx context.Context, req domaindraw.GenerateRequest) (string, error)
	Poll(ctx context.Context, jobID string) (domaindraw.JobUpdate, error)
}

type Notifier interface {
	SendText(ctx context.Context, chatID int64, replyToMessageID int64, text string) (int64, error)
	EditText(ctx context.Context, chatID int64, messageID int64, text string) error
	SendPhoto(ctx context.Context, chatID int64, filename string, caption string, content []byte) error
}
