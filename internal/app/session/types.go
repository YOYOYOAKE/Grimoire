package session

import domainsession "grimoire/internal/domain/session"

type GetOrCreateCommand struct {
	UserID string
}

type AppendMessageCommand struct {
	SessionID string
	Message   domainsession.Message
}

type RecentMessagesResult struct {
	Messages []domainsession.Message
}
