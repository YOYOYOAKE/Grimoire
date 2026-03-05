package sqlite

import (
	"context"
	"time"
)

func (s *TaskStore) CreateInboundMessage(ctx context.Context, chatID, userID, messageID int64, text string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages(chat_id, user_id, telegram_message_id, direction, text, created_at)
		VALUES(?, ?, ?, 'inbound', ?, ?)
	`, chatID, userID, messageID, text, createdAt.UTC())
	return err
}
