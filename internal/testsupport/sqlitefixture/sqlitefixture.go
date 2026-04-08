package sqlitefixture

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domainuser "grimoire/internal/domain/user"
	platformid "grimoire/internal/platform/id"
)

func OpenDB(t testing.TB) *sql.DB {
	t.Helper()

	db, err := sqliterepo.Open(context.Background(), filepath.Join(t.TempDir(), "grimoire.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := sqliterepo.Migrate(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatalf("migrate sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func createUser(t testing.TB, db *sql.DB, userID string, preference domainpreferences.Preference) {
	t.Helper()

	userRepo := sqliterepo.NewUserRepository(db)
	user, err := domainuser.New(userID, domainuser.RoleNormal, preference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func createSession(t testing.TB, db *sql.DB, userID string, sessionID string) domainsession.Session {
	t.Helper()

	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator(sessionID))
	session, err := sessionRepo.GetOrCreateActiveByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID != sessionID {
		t.Fatalf("unexpected session id: %q", session.ID)
	}
	return session
}

func CreateUserAndSession(
	t testing.TB,
	db *sql.DB,
	userID string,
	sessionID string,
	preference domainpreferences.Preference,
) domainsession.Session {
	t.Helper()

	createUser(t, db, userID, preference)
	return createSession(t, db, userID, sessionID)
}

func AppendMessage(
	t testing.TB,
	db *sql.DB,
	sessionID string,
	messageID string,
	role domainsession.MessageRole,
	content string,
	createdAt time.Time,
) {
	t.Helper()

	sessionRepo := sqliterepo.NewSessionRepository(db, nil)
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	session, err := sessionRepo.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	message, err := domainsession.NewMessage(messageID, sessionID, role, content, createdAt)
	if err != nil {
		t.Fatalf("new session message: %v", err)
	}
	if err := messageRepo.Append(context.Background(), message); err != nil {
		t.Fatalf("append session message: %v", err)
	}
	if err := session.RecordMessage(message); err != nil {
		t.Fatalf("record session message: %v", err)
	}
	if err := sessionRepo.Save(context.Background(), session); err != nil {
		t.Fatalf("save session: %v", err)
	}
}
