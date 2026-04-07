package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domainsession "grimoire/internal/domain/session"
)

func TestSessionMessageRepositoryAppendAndListRecent(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	messages := []domainsession.Message{
		newTestSessionMessage(t, "message-1", "session-1", domainsession.MessageRoleUser, "first", time.Unix(1, 0)),
		newTestSessionMessage(t, "message-2", "session-1", domainsession.MessageRoleAssistant, "second", time.Unix(2, 0)),
		newTestSessionMessage(t, "message-3", "session-1", domainsession.MessageRoleUser, "third", time.Unix(3, 0)),
	}
	for _, message := range messages {
		if err := repository.Append(context.Background(), message); err != nil {
			t.Fatalf("append message %s: %v", message.ID, err)
		}
	}

	got, err := repository.ListRecent(context.Background(), "session-1", 2)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected message count: %d", len(got))
	}
	if got[0].ID != "message-2" || got[1].ID != "message-3" {
		t.Fatalf("unexpected recent message order: %#v", []string{got[0].ID, got[1].ID})
	}
}

func TestSessionMessageRepositoryListRecentOrdersMessagesWithinSameSecond(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	base := time.Unix(10, 0)
	for _, message := range []domainsession.Message{
		newTestSessionMessage(t, "message-1", "session-1", domainsession.MessageRoleUser, "first", base),
		newTestSessionMessage(t, "message-2", "session-1", domainsession.MessageRoleAssistant, "second", base.Add(time.Nanosecond)),
	} {
		if err := repository.Append(context.Background(), message); err != nil {
			t.Fatalf("append message %s: %v", message.ID, err)
		}
	}

	got, err := repository.ListRecent(context.Background(), "session-1", 2)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected message count: %d", len(got))
	}
	if got[0].ID != "message-1" || got[1].ID != "message-2" {
		t.Fatalf("unexpected same-second message order: %#v", []string{got[0].ID, got[1].ID})
	}
}

func TestSessionMessageRepositoryListAllReturnsCompleteHistory(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")
	createTestSessionRecord(t, db, "session-2", "user-2")

	for _, message := range []domainsession.Message{
		newTestSessionMessage(t, "message-1", "session-1", domainsession.MessageRoleUser, "first", time.Unix(1, 0)),
		newTestSessionMessage(t, "message-2", "session-2", domainsession.MessageRoleUser, "other", time.Unix(2, 0)),
		newTestSessionMessage(t, "message-3", "session-1", domainsession.MessageRoleAssistant, "second", time.Unix(3, 0)),
	} {
		if err := repository.Append(context.Background(), message); err != nil {
			t.Fatalf("append message %s: %v", message.ID, err)
		}
	}

	got, err := repository.ListAll(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected message count: %d", len(got))
	}
	if got[0].ID != "message-1" || got[1].ID != "message-3" {
		t.Fatalf("unexpected full history order: %#v", []string{got[0].ID, got[1].ID})
	}
}

func TestSessionMessageRepositoryAppendRejectsInvalidMessage(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)

	err := repository.Append(context.Background(), domainsession.Message{
		ID:        "message-1",
		SessionID: "session-1",
		Role:      domainsession.MessageRoleUser,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionMessageRepositoryAppendNormalizesCreatedAtToUTC(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)
	createTestSessionRecord(t, db, "session-1", "user-1")

	createdAt := time.Date(2026, time.April, 7, 21, 30, 0, 123, time.FixedZone("CST", 8*60*60))
	message := newTestSessionMessage(t, "message-1", "session-1", domainsession.MessageRoleUser, "hello", createdAt)
	if err := repository.Append(context.Background(), message); err != nil {
		t.Fatalf("append message: %v", err)
	}

	var raw string
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT created_at FROM session_messages WHERE id = ?`,
		"message-1",
	).Scan(&raw); err != nil {
		t.Fatalf("query created_at: %v", err)
	}

	if raw != createdAt.UTC().Format(sqliteTimestampFormat) {
		t.Fatalf("unexpected stored created_at: %q", raw)
	}
}

func TestSessionMessageRepositoryListRecentRejectsBlankSessionID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)

	if _, err := repository.ListRecent(context.Background(), "   ", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionMessageRepositoryListRecentReturnsEmptyForNonPositiveLimit(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)

	got, err := repository.ListRecent(context.Background(), "session-1", 0)
	if err != nil {
		t.Fatalf("list recent with zero limit: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no messages, got %d", len(got))
	}
}

func TestSessionMessageRepositoryUsesTransactionConnection(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionMessageRepository(db)
	runner := NewTxRunner(db)
	createTestSessionRecord(t, db, "session-1", "user-1")
	expectedErr := errors.New("rollback")

	err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := repository.Append(ctx, newTestSessionMessage(t, "message-1", "session-1", domainsession.MessageRoleUser, "hello", time.Unix(1, 0))); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if got := countRows(t, db, "session_messages"); got != 0 {
		t.Fatalf("expected rollback to leave message count at 0, got %d", got)
	}
}

func createTestSessionRecord(t *testing.T, db *sql.DB, sessionID string, userID string) {
	t.Helper()

	createSessionUser(t, db, userID)

	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO sessions(id, user_id, length, summary) VALUES (?, ?, ?, ?)`,
		sessionID,
		userID,
		0,
		domainsession.EmptySummary().Content(),
	)
	if err != nil {
		t.Fatalf("create session record: %v", err)
	}
}

func newTestSessionMessage(
	t *testing.T,
	id string,
	sessionID string,
	role domainsession.MessageRole,
	content string,
	createdAt time.Time,
) domainsession.Message {
	t.Helper()

	message, err := domainsession.NewMessage(id, sessionID, role, content, createdAt)
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return message
}
