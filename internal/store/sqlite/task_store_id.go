package sqlite

import (
	"context"
	"fmt"
)

func (s *TaskStore) NextTaskID(ctx context.Context) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `INSERT INTO task_counter(id, value) VALUES(1, 0) ON CONFLICT(id) DO NOTHING`); err != nil {
		return "", err
	}

	var current int64
	if err = tx.QueryRowContext(ctx, `SELECT value FROM task_counter WHERE id = 1`).Scan(&current); err != nil {
		return "", err
	}
	current++
	if _, err = tx.ExecContext(ctx, `UPDATE task_counter SET value = ? WHERE id = 1`, current); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return fmt.Sprintf("task-%06d", current), nil
}
