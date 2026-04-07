package session

import (
	"fmt"
	"strings"
	"time"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

type Message struct {
	ID        string
	SessionID string
	Role      MessageRole
	Content   string
	CreatedAt time.Time
}

func NewMessage(id string, sessionID string, role MessageRole, content string, createdAt time.Time) (Message, error) {
	message := Message{
		ID:        id,
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: createdAt,
	}
	if err := message.Validate(); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r MessageRole) Valid() bool {
	switch r {
	case MessageRoleUser, MessageRoleAssistant:
		return true
	default:
		return false
	}
}

func (m Message) Validate() error {
	id := strings.TrimSpace(m.ID)
	sessionID := strings.TrimSpace(m.SessionID)
	content := strings.TrimSpace(m.Content)

	if id == "" {
		return fmt.Errorf("message id is required")
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if !m.Role.Valid() {
		return fmt.Errorf("invalid message role %q", m.Role)
	}
	if content == "" {
		return fmt.Errorf("message content is required")
	}
	if m.CreatedAt.IsZero() {
		return fmt.Errorf("message created_at is required")
	}
	return nil
}
