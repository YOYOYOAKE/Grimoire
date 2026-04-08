package runner

import (
	"context"

	domaindraw "grimoire/internal/domain/draw"
	domaintask "grimoire/internal/domain/task"
)

type TaskRepository interface {
	Get(ctx context.Context, id string) (domaintask.Task, error)
	Update(ctx context.Context, task domaintask.Task) error
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type PromptTranslator interface {
	Translate(ctx context.Context, request string, shape domaindraw.Shape) (domaindraw.Translation, error)
}

type ImageGenerator interface {
	Generate(ctx context.Context, request domaindraw.GenerateRequest) ([]byte, error)
}

type ImageStore interface {
	Save(ctx context.Context, userID string, taskID string, content []byte) (string, error)
}

type MessageOptions struct {
	ReplyToMessageID string
	TaskID           string
	Variant          MessageVariant
}

type MessageVariant string

const (
	MessageVariantNone     MessageVariant = ""
	MessageVariantProgress MessageVariant = "progress"
	MessageVariantResult   MessageVariant = "result"
)

type Notifier interface {
	SendText(ctx context.Context, userID string, text string, options MessageOptions) (string, error)
	EditText(ctx context.Context, userID string, messageID string, text string, options MessageOptions) error
	SendImage(ctx context.Context, userID string, path string, caption string, options MessageOptions) (string, error)
	DeleteMessage(ctx context.Context, userID string, messageID string) error
}
