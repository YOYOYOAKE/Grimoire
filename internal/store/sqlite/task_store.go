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

type TaskStore struct {
	db *sql.DB
}

func NewTaskStore(path string) (*TaskStore, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	return &TaskStore{db: db}, nil
}

func (s *TaskStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *TaskStore) Init(ctx context.Context) error {
	return initSchema(ctx, s.db)
}

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

func (s *TaskStore) CreateInboundMessage(ctx context.Context, chatID, userID, messageID int64, text string, createdAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages(chat_id, user_id, telegram_message_id, direction, text, created_at)
		VALUES(?, ?, ?, 'inbound', ?, ?)
	`, chatID, userID, messageID, text, createdAt.UTC())
	return err
}

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

func (s *TaskStore) AppendGalleryItem(ctx context.Context, chatID, messageID int64, taskID, jobID, filePath, caption string, createdAt time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_items(chat_id, message_id, task_id, job_id, file_path, caption, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)
	`, chatID, messageID, taskID, jobID, filePath, caption, createdAt.UTC())
	return err
}

func (s *TaskStore) ListGalleryItems(ctx context.Context, chatID, messageID int64) ([]store.GalleryItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, chat_id, message_id, task_id, job_id, file_path, caption, created_at
		FROM gallery_items
		WHERE chat_id = ? AND message_id = ?
		ORDER BY id ASC
	`, chatID, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]store.GalleryItem, 0)
	for rows.Next() {
		var item store.GalleryItem
		if err := rows.Scan(
			&item.ID,
			&item.ChatID,
			&item.MessageID,
			&item.TaskID,
			&item.JobID,
			&item.FilePath,
			&item.Caption,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func scanTask(row *sql.Row) (types.DrawTask, error) {
	var (
		task        types.DrawTask
		seed        sql.NullInt64
		createdAt   time.Time
		startedAt   sql.NullTime
		finishedAt  sql.NullTime
		jobID       sql.NullString
		retryOfTask sql.NullString
	)
	if err := row.Scan(
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
		&retryOfTask,
	); err != nil {
		if err == sql.ErrNoRows {
			return types.DrawTask{}, store.ErrNotFound
		}
		return types.DrawTask{}, err
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
	if jobID.Valid {
		task.ResumeJobID = jobID.String
	}
	if retryOfTask.Valid {
		task.RetryOfTaskID = retryOfTask.String
	}
	return task, nil
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func nullableString(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func isTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case types.StatusCompleted, types.StatusFailed, "cancelled":
		return true
	default:
		return false
	}
}
