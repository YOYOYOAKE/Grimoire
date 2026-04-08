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

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()

	var count int
	query := "SELECT COUNT(*) FROM " + table
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("count rows in %s: %v", table, err)
	}
	return count
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var count int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table,
	).Scan(&count); err != nil {
		t.Fatalf("check table %s existence: %v", table, err)
	}
	return count == 1
}

func indexExists(t *testing.T, db *sql.DB, index string) bool {
	t.Helper()

	var count int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`,
		index,
	).Scan(&count); err != nil {
		t.Fatalf("check index %s existence: %v", index, err)
	}
	return count == 1
}

func columnExists(t *testing.T, db *sql.DB, table string, column string) bool {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), "PRAGMA table_info("+table+")")
	if err != nil {
		t.Fatalf("query table info for %s: %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("scan table info for %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info for %s: %v", table, err)
	}
	return false
}

func queryString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()

	var value string
	if err := db.QueryRowContext(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatalf("query string %q: %v", query, err)
	}
	return value
}
