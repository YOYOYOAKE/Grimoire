package chat

import (
	"context"
	"fmt"
	"strings"

	conversationapp "grimoire/internal/app/conversation"
	sessionapp "grimoire/internal/app/session"
)

type Service struct {
	users         UserRepository
	sessions      SessionService
	conversations ConversationService
}

func NewService(
	users UserRepository,
	sessions SessionService,
	conversations ConversationService,
) *Service {
	return &Service{
		users:         users,
		sessions:      sessions,
		conversations: conversations,
	}
}

func (s *Service) HandleText(ctx context.Context, command HandleTextCommand) (HandleTextResult, error) {
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return HandleTextResult{}, fmt.Errorf("user id is required")
	}
	messageID := strings.TrimSpace(command.MessageID)
	if messageID == "" {
		return HandleTextResult{}, fmt.Errorf("message id is required")
	}
	text := strings.TrimSpace(command.Text)
	if text == "" {
		return HandleTextResult{}, fmt.Errorf("text is required")
	}
	if command.CreatedAt.IsZero() {
		return HandleTextResult{}, fmt.Errorf("created at is required")
	}

	user, err := s.users.GetByTelegramID(ctx, userID)
	if err != nil {
		return HandleTextResult{}, err
	}

	currentSession, err := s.sessions.GetOrCreate(ctx, sessionapp.GetOrCreateCommand{UserID: userID})
	if err != nil {
		return HandleTextResult{}, err
	}

	if _, err := s.sessions.AppendUserMessage(ctx, sessionapp.AppendMessageCommand{
		SessionID: currentSession.ID,
		MessageID: messageID,
		Content:   text,
		CreatedAt: command.CreatedAt,
	}); err != nil {
		return HandleTextResult{}, err
	}

	result, err := s.conversations.Converse(ctx, conversationapp.ConverseCommand{
		SessionID:  currentSession.ID,
		Preference: user.Preference,
	})
	if err != nil {
		return HandleTextResult{}, err
	}

	return HandleTextResult{
		SessionID: currentSession.ID,
		Reply:     result.Reply,
	}, nil
}
