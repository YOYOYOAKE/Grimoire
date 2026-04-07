package session

import (
	"context"

	domainsession "grimoire/internal/domain/session"
)

type SessionRepository interface {
	GetOrCreateActiveByUserID(ctx context.Context, userID string) (domainsession.Session, error)
	Save(ctx context.Context, session domainsession.Session) error
}

type SessionMessageRepository interface {
	Append(ctx context.Context, message domainsession.Message) error
	ListRecent(ctx context.Context, sessionID string, limit int) ([]domainsession.Message, error)
}
