package request

import (
	"context"

	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type GenerateInput struct {
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type RequestGenerator interface {
	Generate(ctx context.Context, input GenerateInput) (string, error)
}
