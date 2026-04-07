package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestTxRunnerCommitsSuccessfulTransaction(t *testing.T) {
	db := openMigratedTestDB(t)
	runner := NewTxRunner(db)

	if err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		_, err := ConnFromContext(ctx, db).ExecContext(
			ctx,
			`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
			"user-1",
			"normal",
			`{"shape":"square"}`,
		)
		return err
	}); err != nil {
		t.Fatalf("within tx: %v", err)
	}

	if got := countRows(t, db, "users"); got != 1 {
		t.Fatalf("unexpected user count: %d", got)
	}
}

func TestTxRunnerRollsBackFailedTransaction(t *testing.T) {
	db := openMigratedTestDB(t)
	runner := NewTxRunner(db)
	expectedErr := errors.New("stop here")

	err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		if _, err := ConnFromContext(ctx, db).ExecContext(
			ctx,
			`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
			"user-1",
			"normal",
			`{"shape":"square"}`,
		); err != nil {
			return err
		}
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected rollback error %v, got %v", expectedErr, err)
	}

	if got := countRows(t, db, "users"); got != 0 {
		t.Fatalf("expected rollback to leave user count at 0, got %d", got)
	}
}

func TestTxRunnerReusesExistingTransaction(t *testing.T) {
	db := openMigratedTestDB(t)
	runner := NewTxRunner(db)

	if err := runner.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := runner.WithinTx(ctx, func(ctx context.Context) error {
			_, err := ConnFromContext(ctx, db).ExecContext(
				ctx,
				`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
				"user-1",
				"normal",
				`{"shape":"square"}`,
			)
			return err
		}); err != nil {
			return err
		}

		_, err := ConnFromContext(ctx, db).ExecContext(
			ctx,
			`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
			"user-2",
			"banned",
			`{}`,
		)
		return err
	}); err != nil {
		t.Fatalf("within nested tx: %v", err)
	}

	if got := countRows(t, db, "users"); got != 2 {
		t.Fatalf("unexpected user count: %d", got)
	}
}

func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db := openTestDB(t)
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}
