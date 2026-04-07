package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	domainsession "grimoire/internal/domain/session"
)

type SessionMessageRepository struct {
	db *sql.DB
}

const sqliteTimestampFormat = "2006-01-02T15:04:05.000000000Z07:00"

type sessionMessageRecord struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	CreatedAt string
}

func NewSessionMessageRepository(db *sql.DB) *SessionMessageRepository {
	return &SessionMessageRepository{db: db}
}

func (r *SessionMessageRepository) Append(ctx context.Context, message domainsession.Message) error {
	if err := message.Validate(); err != nil {
		return err
	}

	_, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO session_messages(id, session_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		message.ID,
		message.SessionID,
		string(message.Role),
		message.Content,
		message.CreatedAt.UTC().Format(sqliteTimestampFormat),
	)
	if err != nil {
		return fmt.Errorf("append session message %s: %w", message.ID, err)
	}
	return nil
}

func (r *SessionMessageRepository) ListRecent(ctx context.Context, sessionID string, limit int) ([]domainsession.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if limit <= 0 {
		return []domainsession.Message{}, nil
	}

	rows, err := ConnFromContext(ctx, r.db).QueryContext(
		ctx,
		`SELECT id, session_id, role, content, created_at
		FROM (
			SELECT id, session_id, role, content, created_at
			FROM session_messages
			WHERE session_id = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		)
		ORDER BY created_at ASC, id ASC`,
		sessionID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list recent messages for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	return scanSessionMessages(rows)
}

func (r *SessionMessageRepository) ListAll(ctx context.Context, sessionID string) ([]domainsession.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}

	rows, err := ConnFromContext(ctx, r.db).QueryContext(
		ctx,
		`SELECT id, session_id, role, content, created_at
		FROM session_messages
		WHERE session_id = ?
		ORDER BY created_at ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list all messages for session %s: %w", sessionID, err)
	}
	defer rows.Close()

	return scanSessionMessages(rows)
}

func scanSessionMessages(rows *sql.Rows) ([]domainsession.Message, error) {
	var messages []domainsession.Message
	for rows.Next() {
		var record sessionMessageRecord
		if err := rows.Scan(
			&record.ID,
			&record.SessionID,
			&record.Role,
			&record.Content,
			&record.CreatedAt,
		); err != nil {
			return nil, err
		}

		message, err := restoreSessionMessage(record)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func restoreSessionMessage(record sessionMessageRecord) (domainsession.Message, error) {
	createdAt, err := time.Parse(sqliteTimestampFormat, record.CreatedAt)
	if err != nil {
		return domainsession.Message{}, fmt.Errorf("parse session message %s created_at: %w", record.ID, err)
	}

	message, err := domainsession.NewMessage(
		record.ID,
		record.SessionID,
		domainsession.MessageRole(record.Role),
		record.Content,
		createdAt,
	)
	if err != nil {
		return domainsession.Message{}, fmt.Errorf("restore session message %s: %w", record.ID, err)
	}
	return message, nil
}
