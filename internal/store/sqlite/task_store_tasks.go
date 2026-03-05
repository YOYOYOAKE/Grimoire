package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/store"
	"grimoire/internal/types"
)

func (s *TaskStore) CreateTask(ctx context.Context, task types.DrawTask) error {
	if strings.TrimSpace(task.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}

	var seed any
	if task.Seed != nil {
		seed = *task.Seed
	}

	createdAt := task.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks(
			task_id, chat_id, user_id, status_message_id, prompt, shape, seed,
			status, stage, job_id, error_text, created_at, started_at, finished_at, retry_of_task_id
		) VALUES(?, ?, ?, ?, ?, ?, ?, 'queued', 'queued', NULL, NULL, ?, NULL, NULL, ?)
	`,
		task.TaskID,
		task.ChatID,
		task.UserID,
		task.StatusMessageID,
		task.Prompt,
		task.Shape,
		seed,
		createdAt.UTC(),
		nullIfEmpty(task.RetryOfTaskID),
	)
	return err
}

func (s *TaskStore) UpdateTaskStatus(ctx context.Context, taskID string, status string, stage string, errMsg string) error {
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task_id is required")
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, stage = ?, error_text = ?
		WHERE task_id = ?
	`, status, stage, nullableString(errMsg), taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}

	now := time.Now().UTC()
	if status == types.StatusProcessing {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE tasks SET started_at = COALESCE(started_at, ?) WHERE task_id = ?
		`, now, taskID); err != nil {
			return err
		}
	}
	if isTerminalStatus(status) {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE tasks SET finished_at = ? WHERE task_id = ?
		`, now, taskID); err != nil {
			return err
		}
	}
	return nil
}

func (s *TaskStore) SetTaskJobID(ctx context.Context, taskID string, jobID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET job_id = ?
		WHERE task_id = ?
	`, nullIfEmpty(jobID), taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *TaskStore) SaveTaskResult(ctx context.Context, taskID string, jobID string, filePath string, completedAt time.Time) error {
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE tasks
		SET status = ?, stage = ?, job_id = ?, error_text = NULL, finished_at = ?
		WHERE task_id = ?
	`, types.StatusCompleted, types.StatusCompleted, nullIfEmpty(jobID), completedAt.UTC(), taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *TaskStore) GetTaskByID(ctx context.Context, taskID string) (types.DrawTask, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT task_id, chat_id, user_id, status_message_id, prompt, shape, seed,
		       created_at, started_at, finished_at, job_id, retry_of_task_id
		FROM tasks
		WHERE task_id = ?
	`, taskID)
	return scanTask(row)
}

func (s *TaskStore) ListRecoverableTasks(ctx context.Context) ([]types.DrawTask, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_id, chat_id, user_id, status_message_id, prompt, shape, seed,
		       created_at, started_at, finished_at, job_id, retry_of_task_id, stage
		FROM tasks
		WHERE stage IN ('queued', 'translating', 'submitting', 'polling')
		  AND status NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]types.DrawTask, 0)
	for rows.Next() {
		var (
			task       types.DrawTask
			seed       sql.NullInt64
			startedAt  sql.NullTime
			finishedAt sql.NullTime
			createdAt  time.Time
			jobID      sql.NullString
			retryOf    sql.NullString
			stage      string
		)
		if err := rows.Scan(
			&task.TaskID,
			&task.ChatID,
			&task.UserID,
			&task.StatusMessageID,
			&task.Prompt,
			&task.Shape,
			&seed,
			&createdAt,
			&startedAt,
			&finishedAt,
			&jobID,
			&retryOf,
			&stage,
		); err != nil {
			return nil, err
		}
		task.CreatedAt = createdAt
		if seed.Valid {
			v := seed.Int64
			task.Seed = &v
		}
		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = finishedAt.Time
		}
		if retryOf.Valid {
			task.RetryOfTaskID = retryOf.String
		}
		if jobID.Valid && strings.EqualFold(stage, "polling") {
			task.ResumeJobID = jobID.String
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}
