package conversation

import (
	"context"

	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type ConversationInput struct {
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type ConversationOutput struct {
	Reply   string
	Summary domainsession.Summary
}

type ConversationModel interface {
	Converse(ctx context.Context, input ConversationInput) (ConversationOutput, error)
}
