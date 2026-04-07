package request

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	generator          RequestGenerator
	sessions           SessionRepository
	messages           SessionMessageRepository
	recentMessageLimit int
}

func NewService(generator RequestGenerator, sessions SessionRepository, messages SessionMessageRepository, recentMessageLimit int) *Service {
	return &Service{
		generator:          generator,
		sessions:           sessions,
		messages:           messages,
		recentMessageLimit: recentMessageLimit,
	}
}

func (s *Service) Generate(ctx context.Context, command GenerateCommand) (PendingRequest, error) {
	if s.recentMessageLimit <= 0 {
		return PendingRequest{}, fmt.Errorf("recent message limit must be > 0")
	}

	sessionID := strings.TrimSpace(command.SessionID)
	if sessionID == "" {
		return PendingRequest{}, fmt.Errorf("session id is required")
	}
	if err := command.Preference.Validate(); err != nil {
		return PendingRequest{}, err
	}

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return PendingRequest{}, err
	}
	recentMessages, err := s.messages.ListRecent(ctx, sessionID, s.recentMessageLimit)
	if err != nil {
		return PendingRequest{}, err
	}

	requestText, err := s.generator.Generate(ctx, GenerateInput{
		Summary:    session.Summary,
		Messages:   recentMessages,
		Preference: command.Preference,
	})
	if err != nil {
		return PendingRequest{}, err
	}
	requestText = strings.TrimSpace(requestText)
	if requestText == "" {
		return PendingRequest{}, fmt.Errorf("generated request is required")
	}

	return PendingRequest{
		Request:       requestText,
		ConfirmAction: newAction(DecisionKindConfirm, sessionID),
		ReviseAction:  newAction(DecisionKindRevise, sessionID),
	}, nil
}

func (s *Service) ResolveDecision(command ResolveDecisionCommand) Decision {
	if decision, ok := resolveDecisionFromCallback(strings.TrimSpace(command.CallbackData)); ok {
		return decision
	}

	sessionID := strings.TrimSpace(command.SessionID)
	switch strings.TrimSpace(command.Text) {
	case "确认", "确认执行":
		if sessionID == "" {
			return Decision{}
		}
		return Decision{Kind: DecisionKindConfirm, SessionID: sessionID}
	case "继续修改":
		if sessionID == "" {
			return Decision{}
		}
		return Decision{Kind: DecisionKindRevise, SessionID: sessionID}
	default:
		return Decision{}
	}
}

func resolveDecisionFromCallback(callbackData string) (Decision, bool) {
	switch {
	case strings.HasPrefix(callbackData, RequestConfirmPrefix):
		sessionID := strings.TrimSpace(strings.TrimPrefix(callbackData, RequestConfirmPrefix))
		if sessionID == "" {
			return Decision{}, false
		}
		return Decision{Kind: DecisionKindConfirm, SessionID: sessionID}, true
	case strings.HasPrefix(callbackData, RequestRevisePrefix):
		sessionID := strings.TrimSpace(strings.TrimPrefix(callbackData, RequestRevisePrefix))
		if sessionID == "" {
			return Decision{}, false
		}
		return Decision{Kind: DecisionKindRevise, SessionID: sessionID}, true
	default:
		return Decision{}, false
	}
}
