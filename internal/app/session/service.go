package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainsession "grimoire/internal/domain/session"
)

type Service struct {
	sessions SessionRepository
	messages SessionMessageRepository
	txRunner TxRunner
}

var ErrTxRunnerRequired = errors.New("tx runner is required")

func NewService(sessions SessionRepository, messages SessionMessageRepository, txRunner TxRunner) *Service {
	return &Service{
		sessions: sessions,
		messages: messages,
		txRunner: txRunner,
	}
}

func (s *Service) GetOrCreate(ctx context.Context, command GetOrCreateCommand) (domainsession.Session, error) {
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return domainsession.Session{}, fmt.Errorf("user id is required")
	}
	return s.sessions.GetOrCreateActiveByUserID(ctx, userID)
}

func (s *Service) CreateNew(ctx context.Context, command CreateNewCommand) (domainsession.Session, error) {
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return domainsession.Session{}, fmt.Errorf("user id is required")
	}
	if s.txRunner == nil {
		return domainsession.Session{}, ErrTxRunnerRequired
	}

	var created domainsession.Session
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		session, err := s.sessions.CreateNewActiveByUserID(txCtx, userID)
		if err != nil {
			return err
		}
		created = session
		return nil
	})
	if err != nil {
		return domainsession.Session{}, err
	}
	return created, nil
}

func (s *Service) AppendUserMessage(ctx context.Context, command AppendMessageCommand) (AppendMessageResult, error) {
	return s.appendMessage(ctx, command, domainsession.MessageRoleUser)
}

func (s *Service) AppendAssistantMessage(ctx context.Context, command AppendMessageCommand) (AppendMessageResult, error) {
	return s.appendMessage(ctx, command, domainsession.MessageRoleAssistant)
}

func (s *Service) UpdateSummary(ctx context.Context, command UpdateSummaryCommand) (domainsession.Session, error) {
	if s.txRunner == nil {
		return domainsession.Session{}, ErrTxRunnerRequired
	}

	var updated domainsession.Session
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		session, err := s.loadSession(txCtx, command.SessionID)
		if err != nil {
			return err
		}
		session.UpdateSummary(command.Summary)
		if err := s.sessions.Save(txCtx, session); err != nil {
			return err
		}
		updated = session
		return nil
	})
	if err != nil {
		return domainsession.Session{}, err
	}
	return updated, nil
}

func (s *Service) ListRecentMessages(ctx context.Context, command ListRecentMessagesCommand) (RecentMessagesResult, error) {
	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return RecentMessagesResult{}, fmt.Errorf("session id is required")
	}
	if command.Limit <= 0 {
		return RecentMessagesResult{}, fmt.Errorf("recent message limit must be > 0")
	}

	messages, err := s.messages.ListRecent(ctx, sessionID, command.Limit)
	if err != nil {
		return RecentMessagesResult{}, err
	}
	return RecentMessagesResult{
		Messages: messages,
	}, nil
}

func (s *Service) appendMessage(
	ctx context.Context,
	command AppendMessageCommand,
	role domainsession.MessageRole,
) (AppendMessageResult, error) {
	if s.txRunner == nil {
		return AppendMessageResult{}, ErrTxRunnerRequired
	}

	var result AppendMessageResult
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		session, err := s.loadSession(txCtx, command.SessionID)
		if err != nil {
			return err
		}

		message, err := domainsession.NewMessage(
			command.MessageID,
			session.ID,
			role,
			command.Content,
			command.CreatedAt,
		)
		if err != nil {
			return err
		}
		if err := session.RecordMessage(message); err != nil {
			return err
		}
		if err := s.messages.Append(txCtx, message); err != nil {
			return err
		}
		if err := s.sessions.Save(txCtx, session); err != nil {
			return err
		}

		result = AppendMessageResult{
			Session: session,
			Message: message,
		}
		return nil
	})
	if err != nil {
		return AppendMessageResult{}, err
	}
	return result, nil
}

func (s *Service) loadSession(ctx context.Context, sessionID string) (domainsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return domainsession.Session{}, fmt.Errorf("session id is required")
	}
	return s.sessions.Get(ctx, sessionID)
}
