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
	ID     string
	UserID string
	Length int
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

	var active domainsession.Session
	err := r.withinTx(ctx, func(txCtx context.Context) error {
		session, err := r.getActiveByUserID(txCtx, userID)
		if err == nil {
			active = session
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		created, err := r.insertSession(txCtx, userID)
		if err != nil {
			return err
		}
		if err := r.upsertActiveSession(txCtx, userID, created.ID); err != nil {
			return err
		}
		active = created
		return nil
	})
	if err != nil {
		return domainsession.Session{}, err
	}
	return active, nil
}

func (r *SessionRepository) CreateNewActiveByUserID(ctx context.Context, userID string) (domainsession.Session, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domainsession.Session{}, fmt.Errorf("user id is required")
	}

	var created domainsession.Session
	err := r.withinTx(ctx, func(txCtx context.Context) error {
		session, err := r.insertSession(txCtx, userID)
		if err != nil {
			return err
		}
		if err := r.upsertActiveSession(txCtx, userID, session.ID); err != nil {
			return err
		}
		created = session
		return nil
	})
	if err != nil {
		return domainsession.Session{}, err
	}
	return created, nil
}

func (r *SessionRepository) Save(ctx context.Context, session domainsession.Session) error {
	normalized, err := domainsession.Restore(session.ID, session.UserID, session.Length)
	if err != nil {
		return err
	}

	result, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO sessions(id, user_id, length) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET length = excluded.length
		WHERE sessions.user_id = excluded.user_id`,
		normalized.ID,
		normalized.UserID,
		normalized.Length,
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

func (r *SessionRepository) Get(ctx context.Context, sessionID string) (domainsession.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return domainsession.Session{}, fmt.Errorf("session id is required")
	}

	row := ConnFromContext(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT id, user_id, length FROM sessions WHERE id = ?`,
		sessionID,
	)
	return scanSession(row)
}

func (r *SessionRepository) getActiveByUserID(ctx context.Context, userID string) (domainsession.Session, error) {
	row := ConnFromContext(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT s.id, s.user_id, s.length
		FROM active_sessions AS a
		JOIN sessions AS s ON s.id = a.session_id
		WHERE a.user_id = ?`,
		userID,
	)
	return scanSession(row)
}

func (r *SessionRepository) insertSession(ctx context.Context, userID string) (domainsession.Session, error) {
	session, err := domainsession.New(r.generator.NewString(), userID)
	if err != nil {
		return domainsession.Session{}, fmt.Errorf("create session for user %s: %w", userID, err)
	}

	if _, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO sessions(id, user_id, length) VALUES (?, ?, ?)`,
		session.ID,
		session.UserID,
		session.Length,
	); err != nil {
		return domainsession.Session{}, fmt.Errorf("insert session %s: %w", session.ID, err)
	}
	return session, nil
}

func (r *SessionRepository) upsertActiveSession(ctx context.Context, userID string, sessionID string) error {
	_, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO active_sessions(user_id, session_id) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET session_id = excluded.session_id`,
		userID,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("upsert active session for user %s: %w", userID, err)
	}
	return nil
}

func (r *SessionRepository) withinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session repository tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = fn(contextWithTx(ctx, tx)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit session repository tx: %w", err)
	}
	return nil
}

func scanSession(row *sql.Row) (domainsession.Session, error) {
	var record sessionRecord
	if err := row.Scan(&record.ID, &record.UserID, &record.Length); err != nil {
		return domainsession.Session{}, err
	}

	session, err := domainsession.Restore(
		record.ID,
		record.UserID,
		record.Length,
	)
	if err != nil {
		return domainsession.Session{}, fmt.Errorf("restore session %s: %w", record.ID, err)
	}
	return session, nil
}
