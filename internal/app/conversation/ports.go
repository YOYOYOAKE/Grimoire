package conversation

import (
	"context"

	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type SessionRepository interface {
	Get(ctx context.Context, sessionID string) (domainsession.Session, error)
	Save(ctx context.Context, session domainsession.Session) error
}

type SessionMessageRepository interface {
	Append(ctx context.Context, message domainsession.Message) error
	ListRecent(ctx context.Context, sessionID string, limit int) ([]domainsession.Message, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type ConversationInput struct {
	SessionID  string
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type CreateDrawingTask struct {
	Request string
}

type ConversationOutput struct {
	Reply             string
	Summary           domainsession.Summary
	CreateDrawingTask *CreateDrawingTask
}

type ConversationModel interface {
	Converse(ctx context.Context, input ConversationInput) (ConversationOutput, error)
}
