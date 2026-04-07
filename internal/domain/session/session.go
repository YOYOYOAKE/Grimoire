package session

import (
	"fmt"
	"strings"
)

type Session struct {
	ID      string
	UserID  string
	Length  int
	Summary Summary
}

func New(id string, userID string) (Session, error) {
	return Restore(id, userID, 0, EmptySummary())
}

func Restore(id string, userID string, length int, summary Summary) (Session, error) {
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)

	if id == "" {
		return Session{}, fmt.Errorf("session id is required")
	}
	if userID == "" {
		return Session{}, fmt.Errorf("user id is required")
	}
	if length < 0 {
		return Session{}, fmt.Errorf("session length must be >= 0")
	}

	return Session{
		ID:      id,
		UserID:  userID,
		Length:  length,
		Summary: NewSummary(summary.Content()),
	}, nil
}

func (s *Session) RecordMessage(message Message) error {
	if err := message.Validate(); err != nil {
		return err
	}
	if message.SessionID != s.ID {
		return fmt.Errorf("message session mismatch: %s", message.SessionID)
	}
	s.Length++
	return nil
}

func (s *Session) UpdateSummary(summary Summary) {
	s.Summary = NewSummary(summary.Content())
}
