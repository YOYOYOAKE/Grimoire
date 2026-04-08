package sqlite

import (
	"context"
	"testing"
)

func TestMigrateAppliesFullEmbeddedSchemaOnlyOnce(t *testing.T) {
	db := openTestDB(t)

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	if got := countRows(t, db, "schema_migrations"); got != 2 {
		t.Fatalf("unexpected migration count: %d", got)
	}

	for _, table := range []string{"users", "sessions", "active_sessions", "session_messages", "tasks"} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected %s table to exist after migration", table)
		}
	}
	if !indexExists(t, db, "idx_tasks_recoverable_status") {
		t.Fatal("expected idx_tasks_recoverable_status index to exist after migration")
	}
	if !indexExists(t, db, "idx_sessions_user_id") {
		t.Fatal("expected idx_sessions_user_id index to exist after migration")
	}
}

func TestApplyMigrationsSkipsAlreadyRecordedMigration(t *testing.T) {
	db := openTestDB(t)

	migrations := []migration{
		{
			name: "001_test",
			sql: `
CREATE TABLE sample (
    id TEXT PRIMARY KEY
);`,
		},
	}

	if err := applyMigrations(context.Background(), db, migrations); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := applyMigrations(context.Background(), db, migrations); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	if got := countRows(t, db, "schema_migrations"); got != 1 {
		t.Fatalf("unexpected migration count: %d", got)
	}
	if !tableExists(t, db, "sample") {
		t.Fatal("expected sample table to exist")
	}
}

func TestMigrateUpgradesLegacySingleSessionSchema(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.ExecContext(context.Background(), legacyInitSQL); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if err := ensureSchemaMigrationsTable(context.Background(), db); err != nil {
		t.Fatalf("ensure schema migrations: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO schema_migrations(name) VALUES ('001_init')`); err != nil {
		t.Fatalf("record legacy migration: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
		"user-1",
		"normal",
		`{"shape":"square"}`,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO sessions(id, user_id, length, summary) VALUES (?, ?, ?, ?)`,
		"session-1",
		"user-1",
		2,
		`{"topic":"moon"}`,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO session_messages(id, session_id, role, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		"message-1",
		"session-1",
		"user",
		"hello",
		"2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seed session message: %v", err)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO tasks(id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"task-1",
		"user-1",
		"session-1",
		nil,
		"draw moon",
		nil,
		nil,
		"queued",
		nil,
		`[]`,
		`{"version":1,"shape":"square"}`,
		nil,
		nil,
	); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("upgrade migrate: %v", err)
	}

	if got := countRows(t, db, "schema_migrations"); got != 2 {
		t.Fatalf("unexpected migration count after upgrade: %d", got)
	}
	if !tableExists(t, db, "active_sessions") {
		t.Fatal("expected active_sessions table after upgrade")
	}
	if got := queryString(t, db, `SELECT session_id FROM active_sessions WHERE user_id = ?`, "user-1"); got != "session-1" {
		t.Fatalf("unexpected active session id after upgrade: %q", got)
	}
	if got := countRows(t, db, "session_messages"); got != 1 {
		t.Fatalf("expected session message to survive upgrade, got %d", got)
	}
	if got := countRows(t, db, "tasks"); got != 1 {
		t.Fatalf("expected task to survive upgrade, got %d", got)
	}
	if _, err := db.ExecContext(
		context.Background(),
		`INSERT INTO sessions(id, user_id, length, summary) VALUES (?, ?, ?, ?)`,
		"session-2",
		"user-1",
		0,
		`{}`,
	); err != nil {
		t.Fatalf("expected duplicate user sessions to be allowed after upgrade: %v", err)
	}
}

const legacyInitSQL = `
CREATE TABLE users (
    telegram_id TEXT PRIMARY KEY,
    role TEXT NOT NULL CHECK (role IN ('normal', 'banned')),
    preference TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    length INTEGER NOT NULL DEFAULT 0 CHECK (length >= 0),
    summary TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (user_id) REFERENCES users(telegram_id) ON DELETE CASCADE
);

CREATE TABLE session_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_session_messages_session_created_at
    ON session_messages(session_id, created_at DESC, id DESC);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    source_task TEXT,
    request TEXT NOT NULL CHECK (length(trim(request)) > 0),
    prompt TEXT,
    image TEXT,
    status TEXT NOT NULL CHECK (status IN ('queued', 'translating', 'drawing', 'completed', 'failed', 'stopped')),
    error TEXT,
    timeline TEXT NOT NULL,
    context TEXT NOT NULL,
    progress_message_id TEXT,
    result_message_id TEXT,
    FOREIGN KEY (user_id) REFERENCES users(telegram_id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    FOREIGN KEY (source_task) REFERENCES tasks(id) ON DELETE SET NULL
);

CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_session_id ON tasks(session_id);
CREATE INDEX idx_tasks_source_task ON tasks(source_task);
CREATE INDEX idx_tasks_recoverable_status
    ON tasks(status)
    WHERE status IN ('queued', 'translating', 'drawing');
`
