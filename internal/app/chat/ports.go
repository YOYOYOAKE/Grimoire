package chat

import (
	"context"

	conversationapp "grimoire/internal/app/conversation"
	sessionapp "grimoire/internal/app/session"
	domainsession "grimoire/internal/domain/session"
	domainuser "grimoire/internal/domain/user"
)

type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID string) (domainuser.User, error)
}

type SessionService interface {
	GetOrCreate(ctx context.Context, command sessionapp.GetOrCreateCommand) (domainsession.Session, error)
	AppendUserMessage(ctx context.Context, command sessionapp.AppendMessageCommand) (sessionapp.AppendMessageResult, error)
}

type ConversationService interface {
	Converse(ctx context.Context, command conversationapp.ConverseCommand) (conversationapp.ConverseResult, error)
}
