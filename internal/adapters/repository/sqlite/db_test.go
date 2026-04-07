package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesDatabaseAndChecksAvailability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "grimoire.db")

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected sqlite file to exist: %v", err)
	}

	var value int
	if err := db.QueryRowContext(context.Background(), "SELECT 1").Scan(&value); err != nil {
		t.Fatalf("select 1: %v", err)
	}
	if value != 1 {
		t.Fatalf("unexpected availability result: %d", value)
	}

	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("unexpected max open connections: %d", got)
	}
}

func TestOpenAppliesRecommendedPragmas(t *testing.T) {
	db := openTestDB(t)

	if got := pragmaString(t, db, "journal_mode"); got != "wal" {
		t.Fatalf("unexpected journal mode: %q", got)
	}
	if got := pragmaInt(t, db, "foreign_keys"); got != 1 {
		t.Fatalf("unexpected foreign_keys: %d", got)
	}
	if got := pragmaInt(t, db, "busy_timeout"); got != busyTimeoutMillis {
		t.Fatalf("unexpected busy_timeout: %d", got)
	}
	if got := pragmaInt(t, db, "temp_store"); got != 2 {
		t.Fatalf("unexpected temp_store: %d", got)
	}
}

func TestOpenRejectsEmptyPath(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("expected error")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "grimoire.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func pragmaString(t *testing.T, db *sql.DB, name string) string {
	t.Helper()

	var value string
	query := "PRAGMA " + name + ";"
	if err := db.QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("query pragma %s: %v", name, err)
	}
	return value
}

func pragmaInt(t *testing.T, db *sql.DB, name string) int {
	t.Helper()

	var value int
	query := "PRAGMA " + name + ";"
	if err := db.QueryRowContext(context.Background(), query).Scan(&value); err != nil {
		t.Fatalf("query pragma %s: %v", name, err)
	}
	return value
}
