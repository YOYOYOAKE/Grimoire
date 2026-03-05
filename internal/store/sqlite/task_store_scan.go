package sqlite

import (
	"database/sql"
	"strings"
	"time"

	"grimoire/internal/store"
	"grimoire/internal/types"
)

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
