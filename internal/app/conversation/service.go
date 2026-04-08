package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainsession "grimoire/internal/domain/session"
)

var ErrTxRunnerRequired = errors.New("tx runner is required")

type Service struct {
	model              ConversationModel
	sessions           SessionRepository
	messages           SessionMessageRepository
	txRunner           TxRunner
	recentMessageLimit int
	now                func() time.Time
	idGenerator        func() string
}

func NewService(
	model ConversationModel,
	sessions SessionRepository,
	messages SessionMessageRepository,
	txRunner TxRunner,
	recentMessageLimit int,
	now func() time.Time,
	idGenerator func() string,
) *Service {
	if now == nil {
		now = time.Now
	}
	if idGenerator == nil {
		idGenerator = func() string {
			return fmt.Sprintf("assistant-%d", now().UnixNano())
		}
	}
	return &Service{
		model:              model,
		sessions:           sessions,
		messages:           messages,
		txRunner:           txRunner,
		recentMessageLimit: recentMessageLimit,
		now:                now,
		idGenerator:        idGenerator,
	}
}

func (s *Service) Converse(ctx context.Context, command ConverseCommand) (ConverseResult, error) {
	if s.txRunner == nil {
		return ConverseResult{}, ErrTxRunnerRequired
	}
	if s.recentMessageLimit <= 0 {
		return ConverseResult{}, fmt.Errorf("recent message limit must be > 0")
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return ConverseResult{}, fmt.Errorf("session id is required")
	}
	if err := command.Preference.Validate(); err != nil {
		return ConverseResult{}, err
	}

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return ConverseResult{}, err
	}
	recentMessages, err := s.messages.ListRecent(ctx, sessionID, s.recentMessageLimit)
	if err != nil {
		return ConverseResult{}, err
	}

	output, err := s.model.Converse(ctx, ConversationInput{
		Summary:    session.Summary,
		Messages:   recentMessages,
		Preference: command.Preference,
	})
	if err != nil {
		return ConverseResult{}, err
	}

	reply := strings.TrimSpace(output.Reply)
	summary := domainsession.NewSummary(output.Summary.Content())
	var createDrawingTask *CreateDrawingTask
	if output.CreateDrawingTask != nil {
		request := strings.TrimSpace(output.CreateDrawingTask.Request)
		if request == "" {
			return ConverseResult{}, fmt.Errorf("conversation create drawing task request is required")
		}
		createDrawingTask = &CreateDrawingTask{Request: request}
	}
	switch {
	case reply == "" && createDrawingTask == nil:
		return ConverseResult{}, fmt.Errorf("conversation reply or create drawing task is required")
	case reply != "" && createDrawingTask != nil:
		return ConverseResult{}, fmt.Errorf("conversation reply and create drawing task are mutually exclusive")
	}

	err = s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		latestSession, err := s.sessions.Get(txCtx, sessionID)
		if err != nil {
			return err
		}

		latestSession.UpdateSummary(summary)

		if reply != "" {
			message, err := domainsession.NewMessage(
				s.idGenerator(),
				latestSession.ID,
				domainsession.MessageRoleAssistant,
				reply,
				s.now(),
			)
			if err != nil {
				return err
			}
			if err := latestSession.RecordMessage(message); err != nil {
				return err
			}
			if err := s.messages.Append(txCtx, message); err != nil {
				return err
			}
		}
		if err := s.sessions.Save(txCtx, latestSession); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ConverseResult{}, err
	}

	return ConverseResult{
		Reply:             reply,
		Summary:           summary,
		CreateDrawingTask: createDrawingTask,
	}, nil
}
