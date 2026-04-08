package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domaindraw "grimoire/internal/domain/draw"
	domainsession "grimoire/internal/domain/session"
	domainuser "grimoire/internal/domain/user"
	platformid "grimoire/internal/platform/id"
)

func TestSessionRepositoryGetOrCreateActiveByUserIDCreatesOnce(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	createSessionUser(t, db, "user-1")

	first, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get or create session: %v", err)
	}
	second, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get existing session: %v", err)
	}

	if first.ID != "session-1" {
		t.Fatalf("unexpected session id: %s", first.ID)
	}
	if first.Length != 0 {
		t.Fatalf("unexpected initial length: %d", first.Length)
	}
	if !first.Summary.IsEmpty() {
		t.Fatalf("expected empty summary, got %q", first.Summary.Content())
	}
	if second.ID != first.ID {
		t.Fatalf("expected existing session id %s, got %s", first.ID, second.ID)
	}
	if got := countRows(t, db, "sessions"); got != 1 {
		t.Fatalf("expected exactly one session row, got %d", got)
	}
	if got := activeSessionID(t, db, "user-1"); got != "session-1" {
		t.Fatalf("unexpected active session id: %q", got)
	}
}

func TestSessionRepositorySavePersistsLengthAndSummary(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	createSessionUser(t, db, "user-1")

	session, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get or create session: %v", err)
	}
	message, err := domainsession.NewMessage(
		"message-1",
		"session-1",
		domainsession.MessageRoleAssistant,
		"reply",
		time.Unix(1, 0),
	)
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := session.RecordMessage(message); err != nil {
		t.Fatalf("record message: %v", err)
	}
	session.UpdateSummary(domainsession.NewSummary(`{"topic":"castle"}`))

	if err := repository.Save(context.Background(), session); err != nil {
		t.Fatalf("save session: %v", err)
	}

	got, err := repository.Get(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.ID != "session-1" {
		t.Fatalf("unexpected session id: %s", got.ID)
	}
	if got.Length != 1 {
		t.Fatalf("unexpected length: %d", got.Length)
	}
	if got.Summary.Content() != `{"topic":"castle"}` {
		t.Fatalf("unexpected summary: %q", got.Summary.Content())
	}
}

func TestSessionRepositoryGetLoadsSessionByID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	createSessionUser(t, db, "user-1")

	session, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get or create session: %v", err)
	}
	session.UpdateSummary(domainsession.NewSummary(`{"topic":"moon"}`))
	if err := repository.Save(context.Background(), session); err != nil {
		t.Fatalf("save session: %v", err)
	}

	got, err := repository.Get(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.UserID != "user-1" {
		t.Fatalf("unexpected user id: %s", got.UserID)
	}
	if got.Summary.Content() != `{"topic":"moon"}` {
		t.Fatalf("unexpected summary: %q", got.Summary.Content())
	}
}

func TestSessionRepositoryGetOrCreateActiveByUserIDDoesNotRequireGeneratorForExistingSession(t *testing.T) {
	db := openMigratedTestDB(t)
	createSessionUser(t, db, "user-1")

	seedRepository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	if _, err := seedRepository.GetOrCreateActiveByUserID(context.Background(), "user-1"); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	repository := NewSessionRepository(db, platformid.NewStaticGenerator(""))
	got, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get existing session: %v", err)
	}
	if got.ID != "session-1" {
		t.Fatalf("unexpected session id: %s", got.ID)
	}
}

func TestSessionRepositoryCreateNewActiveByUserIDSwitchesActiveSession(t *testing.T) {
	db := openMigratedTestDB(t)
	createSessionUser(t, db, "user-1")

	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	first, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	repository = NewSessionRepository(db, platformid.NewStaticGenerator("session-2"))
	second, err := repository.CreateNewActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("create new active session: %v", err)
	}

	active, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	if first.ID != "session-1" {
		t.Fatalf("unexpected first session id: %q", first.ID)
	}
	if second.ID != "session-2" {
		t.Fatalf("unexpected second session id: %q", second.ID)
	}
	if active.ID != "session-2" {
		t.Fatalf("expected active session-2, got %q", active.ID)
	}
	if got := countRows(t, db, "sessions"); got != 2 {
		t.Fatalf("expected two session rows, got %d", got)
	}
	if got := activeSessionID(t, db, "user-1"); got != "session-2" {
		t.Fatalf("unexpected active session id: %q", got)
	}
}

func TestSessionRepositorySaveRejectsInvalidSession(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))

	err := repository.Save(context.Background(), domainsession.Session{
		ID:      "session-1",
		UserID:  "user-1",
		Length:  -1,
		Summary: domainsession.EmptySummary(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionRepositorySaveRejectsOwnerChange(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-seed"))
	createSessionUser(t, db, "user-1")
	createSessionUser(t, db, "user-2")

	original, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new original session: %v", err)
	}
	if err := repository.Save(context.Background(), original); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	mutated := original
	mutated.UserID = "user-2"
	mutated.UpdateSummary(domainsession.NewSummary(`{"topic":"forbidden"}`))

	if err := repository.Save(context.Background(), mutated); err == nil {
		t.Fatal("expected error")
	}

	got, err := repository.Get(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("reload original session: %v", err)
	}
	if got.UserID != "user-1" {
		t.Fatalf("unexpected user id after rejected owner change: %s", got.UserID)
	}
	if got.Summary.Content() != domainsession.EmptySummary().Content() {
		t.Fatalf("unexpected summary after rejected owner change: %q", got.Summary.Content())
	}
	if got := countRows(t, db, "sessions"); got != 1 {
		t.Fatalf("expected one session row, got %d", got)
	}
}

func TestSessionRepositoryGetOrCreateActiveByUserIDRejectsBlankUserID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))

	if _, err := repository.GetOrCreateActiveByUserID(context.Background(), "   "); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionRepositoryGetRejectsBlankSessionID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))

	if _, err := repository.Get(context.Background(), "   "); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionRepositoryCreateNewActiveByUserIDRejectsBlankUserID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))

	if _, err := repository.CreateNewActiveByUserID(context.Background(), "   "); err == nil {
		t.Fatal("expected error")
	}
}

func TestSessionRepositoryUsesTransactionConnection(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	runner := NewTxRunner(db)
	createSessionUser(t, db, "user-1")
	expectedErr := errors.New("rollback")

	err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		session, err := repository.GetOrCreateActiveByUserID(ctx, "user-1")
		if err != nil {
			return err
		}
		session.UpdateSummary(domainsession.NewSummary(`{"state":"pending"}`))
		if err := repository.Save(ctx, session); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if got := countRows(t, db, "sessions"); got != 0 {
		t.Fatalf("expected rollback to leave session count at 0, got %d", got)
	}
}

func TestSessionRepositoryCreateNewActiveByUserIDRollsBackWithTransaction(t *testing.T) {
	db := openMigratedTestDB(t)
	createSessionUser(t, db, "user-1")

	repository := NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	first, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	repository = NewSessionRepository(db, platformid.NewStaticGenerator("session-2"))
	runner := NewTxRunner(db)
	expectedErr := errors.New("rollback")

	err = runner.WithinTx(context.Background(), func(ctx context.Context) error {
		created, err := repository.CreateNewActiveByUserID(ctx, "user-1")
		if err != nil {
			return err
		}
		if created.ID != "session-2" {
			t.Fatalf("unexpected created session id in tx: %q", created.ID)
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	active, err := repository.GetOrCreateActiveByUserID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get active session after rollback: %v", err)
	}
	if active.ID != first.ID {
		t.Fatalf("expected active session %q after rollback, got %q", first.ID, active.ID)
	}
	if got := countRows(t, db, "sessions"); got != 1 {
		t.Fatalf("expected one session row after rollback, got %d", got)
	}
}

func createSessionUser(t *testing.T, db *sql.DB, telegramID string) {
	t.Helper()

	repository := NewUserRepository(db)
	user := newTestUser(t, telegramID, domainuser.RoleNormal, domaindraw.ShapeSquare, "")
	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func activeSessionID(t *testing.T, db *sql.DB, userID string) string {
	t.Helper()

	var sessionID string
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT session_id FROM active_sessions WHERE user_id = ?`,
		userID,
	).Scan(&sessionID); err != nil {
		t.Fatalf("query active session id: %v", err)
	}
	return sessionID
}
