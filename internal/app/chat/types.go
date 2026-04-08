package chat

import "time"

type HandleTextCommand struct {
	UserID    string
	MessageID string
	Text      string
	CreatedAt time.Time
}

type HandleTextResult struct {
	SessionID     string
	Reply         string
	CreatedTaskID string
}
