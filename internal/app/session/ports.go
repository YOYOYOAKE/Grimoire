package session

import (
	"context"

	domainsession "grimoire/internal/domain/session"
)

type SessionRepository interface {
	GetOrCreateActiveByUserID(ctx context.Context, userID string) (domainsession.Session, error)
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
