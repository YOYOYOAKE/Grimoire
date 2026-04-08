package session

import (
	"time"

	domainsession "grimoire/internal/domain/session"
)

type GetOrCreateCommand struct {
	UserID string
}

type CreateNewCommand struct {
	UserID string
}

type AppendMessageCommand struct {
	SessionID string
	MessageID string
	Content   string
	CreatedAt time.Time
}

type ListRecentMessagesCommand struct {
	SessionID string
	Limit     int
}

type AppendMessageResult struct {
	Session domainsession.Session
	Message domainsession.Message
}

type RecentMessagesResult struct {
	Messages []domainsession.Message
}
