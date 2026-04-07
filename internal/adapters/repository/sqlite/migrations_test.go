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

	if got := countRows(t, db, "schema_migrations"); got != 1 {
		t.Fatalf("unexpected migration count: %d", got)
	}

	for _, table := range []string{"users", "sessions", "session_messages", "tasks"} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected %s table to exist after migration", table)
		}
	}
	if !indexExists(t, db, "idx_tasks_recoverable_status") {
		t.Fatal("expected idx_tasks_recoverable_status index to exist after migration")
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
