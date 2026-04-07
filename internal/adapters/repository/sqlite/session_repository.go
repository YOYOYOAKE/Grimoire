package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	domainsession "grimoire/internal/domain/session"
	platformid "grimoire/internal/platform/id"
)

type SessionRepository struct {
	db        *sql.DB
	generator platformid.Generator
}

type sessionRecord struct {
	ID      string
	UserID  string
	Length  int
	Summary string
}

func NewSessionRepository(db *sql.DB, generator platformid.Generator) *SessionRepository {
	if generator == nil {
		defaultGenerator := platformid.NewUUIDGenerator()
		generator = defaultGenerator
	}

	return &SessionRepository{
		db:        db,
		generator: generator,
	}
}

func (r *SessionRepository) GetOrCreateActiveByUserID(ctx context.Context, userID string) (domainsession.Session, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domainsession.Session{}, fmt.Errorf("user id is required")
	}

	session, err := r.getByUserID(ctx, userID)
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domainsession.Session{}, err
	}

	session, err = domainsession.New(r.generator.NewString(), userID)
	if err != nil {
		return domainsession.Session{}, fmt.Errorf("create session for user %s: %w", userID, err)
	}

	_, err = ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO sessions(id, user_id, length, summary) VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id) DO NOTHING`,
		session.ID,
		session.UserID,
		session.Length,
		session.Summary.Content(),
	)
	if err != nil {
		return domainsession.Session{}, fmt.Errorf("ensure active session for user %s: %w", userID, err)
	}

	return r.getByUserID(ctx, userID)
}

func (r *SessionRepository) Save(ctx context.Context, session domainsession.Session) error {
	normalized, err := domainsession.Restore(session.ID, session.UserID, session.Length, session.Summary)
	if err != nil {
		return err
	}

	result, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO sessions(id, user_id, length, summary) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET length = excluded.length, summary = excluded.summary
		WHERE sessions.user_id = excluded.user_id`,
		normalized.ID,
		normalized.UserID,
		normalized.Length,
		normalized.Summary.Content(),
	)
	if err != nil {
		return fmt.Errorf("save session %s: %w", normalized.ID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("save session %s rows affected: %w", normalized.ID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("save session %s: session owner is immutable", normalized.ID)
	}
	return nil
}

func (r *SessionRepository) getByUserID(ctx context.Context, userID string) (domainsession.Session, error) {
	row := ConnFromContext(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT id, user_id, length, summary FROM sessions WHERE user_id = ?`,
		userID,
	)

	var record sessionRecord
	if err := row.Scan(&record.ID, &record.UserID, &record.Length, &record.Summary); err != nil {
		return domainsession.Session{}, err
	}

	session, err := domainsession.Restore(
		record.ID,
		record.UserID,
		record.Length,
		domainsession.NewSummary(record.Summary),
	)
	if err != nil {
		return domainsession.Session{}, fmt.Errorf("restore session %s: %w", record.ID, err)
	}
	return session, nil
}
