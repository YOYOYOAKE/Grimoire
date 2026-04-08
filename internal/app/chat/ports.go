package chat

import (
	"context"

	conversationapp "grimoire/internal/app/conversation"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
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

type TaskService interface {
	Create(ctx context.Context, command taskapp.CreateCommand) (domaintask.Task, error)
}
