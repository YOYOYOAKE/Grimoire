package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type TxRunner struct {
	db *sql.DB
}

type txContextKey struct{}

func NewTxRunner(db *sql.DB) *TxRunner {
	return &TxRunner{db: db}
}

func (r *TxRunner) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if r == nil || r.db == nil {
		return fmt.Errorf("sqlite tx runner requires a database")
	}
	if fn == nil {
		return fmt.Errorf("sqlite tx runner requires a callback")
	}

	if _, ok := txFromContext(ctx); ok {
		return fn(ctx)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite tx: %w", err)
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
		return fmt.Errorf("commit sqlite tx: %w", err)
	}
	return nil
}

func ConnFromContext(ctx context.Context, db *sql.DB) DBTX {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return db
}

func contextWithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx, ok
}
