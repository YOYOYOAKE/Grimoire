package request

import (
	"context"

	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type SessionRepository interface {
	Get(ctx context.Context, sessionID string) (domainsession.Session, error)
}

type SessionMessageRepository interface {
	ListRecent(ctx context.Context, sessionID string, limit int) ([]domainsession.Message, error)
}

type GenerateInput struct {
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type RequestGenerator interface {
	Generate(ctx context.Context, input GenerateInput) (string, error)
}
