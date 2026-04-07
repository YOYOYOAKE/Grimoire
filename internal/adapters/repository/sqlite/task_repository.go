package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	domaintask "grimoire/internal/domain/task"
)

type TaskRepository struct {
	db *sql.DB
}

type taskRecord struct {
	ID                string
	UserID            string
	SessionID         string
	SourceTask        sql.NullString
	Request           string
	Prompt            sql.NullString
	Image             sql.NullString
	Status            string
	Error             sql.NullString
	Timeline          string
	Context           string
	ProgressMessageID sql.NullString
	ResultMessageID   sql.NullString
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) Create(ctx context.Context, task domaintask.Task) error {
	record, err := encodeTask(task)
	if err != nil {
		return err
	}

	_, err = ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO tasks(
			id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.UserID,
		record.SessionID,
		record.SourceTask,
		record.Request,
		record.Prompt,
		record.Image,
		record.Status,
		record.Error,
		record.Timeline,
		record.Context,
		record.ProgressMessageID,
		record.ResultMessageID,
	)
	if err != nil {
		return fmt.Errorf("create task %s: %w", record.ID, err)
	}
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id string) (domaintask.Task, error) {
	record, err := r.getRecord(ctx, id)
	if err != nil {
		return domaintask.Task{}, err
	}
	return decodeTask(record)
}

func (r *TaskRepository) Update(ctx context.Context, task domaintask.Task) error {
	record, err := encodeTask(task)
	if err != nil {
		return err
	}

	existing, err := r.getRecord(ctx, record.ID)
	if err != nil {
		return err
	}
	if err := ensureTaskImmutableFields(existing, record); err != nil {
		return err
	}

	_, err = ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`UPDATE tasks
		SET prompt = ?, image = ?, status = ?, error = ?, timeline = ?, progress_message_id = ?, result_message_id = ?
		WHERE id = ?`,
		record.Prompt,
		record.Image,
		record.Status,
		record.Error,
		record.Timeline,
		record.ProgressMessageID,
		record.ResultMessageID,
		record.ID,
	)
	if err != nil {
		return fmt.Errorf("update task %s: %w", record.ID, err)
	}
	return nil
}

func (r *TaskRepository) ListRecoverable(ctx context.Context) ([]domaintask.Task, error) {
	rows, err := ConnFromContext(ctx, r.db).QueryContext(
		ctx,
		`SELECT id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id
		FROM tasks
		WHERE status IN (?, ?, ?)`,
		string(domaintask.StatusQueued),
		string(domaintask.StatusTranslating),
		string(domaintask.StatusDrawing),
	)
	if err != nil {
		return nil, fmt.Errorf("list recoverable tasks: %w", err)
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	sortTasksByCreatedAt(tasks)
	return tasks, nil
}

func (r *TaskRepository) ListBySourceTask(ctx context.Context, sourceTaskID string) ([]domaintask.Task, error) {
	sourceTaskID = strings.TrimSpace(sourceTaskID)
	if sourceTaskID == "" {
		return nil, fmt.Errorf("source task id is required")
	}

	rows, err := ConnFromContext(ctx, r.db).QueryContext(
		ctx,
		`SELECT id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id
		FROM tasks
		WHERE source_task = ?`,
		sourceTaskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tasks by source task %s: %w", sourceTaskID, err)
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	sortTasksByCreatedAt(tasks)
	return tasks, nil
}

func (r *TaskRepository) getRecord(ctx context.Context, id string) (taskRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return taskRecord{}, fmt.Errorf("task id is required")
	}

	row := ConnFromContext(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT id, user_id, session_id, source_task, request, prompt, image, status, error, timeline, context, progress_message_id, result_message_id
		FROM tasks
		WHERE id = ?`,
		id,
	)

	var record taskRecord
	if err := row.Scan(
		&record.ID,
		&record.UserID,
		&record.SessionID,
		&record.SourceTask,
		&record.Request,
		&record.Prompt,
		&record.Image,
		&record.Status,
		&record.Error,
		&record.Timeline,
		&record.Context,
		&record.ProgressMessageID,
		&record.ResultMessageID,
	); err != nil {
		return taskRecord{}, err
	}
	return record, nil
}

func encodeTask(task domaintask.Task) (taskRecord, error) {
	normalized, err := domaintask.Restore(
		task.ID,
		task.UserID,
		task.SessionID,
		task.SourceTaskID,
		task.Request,
		task.Prompt,
		task.Image,
		task.Status,
		task.Error,
		task.Timeline,
		task.Context,
		task.ProgressMessageID,
		task.ResultMessageID,
	)
	if err != nil {
		return taskRecord{}, err
	}

	timelineJSON, err := json.Marshal(normalized.Timeline)
	if err != nil {
		return taskRecord{}, fmt.Errorf("encode task %s timeline: %w", normalized.ID, err)
	}

	var taskErrorJSON sql.NullString
	if normalized.Error != nil {
		payload, err := json.Marshal(normalized.Error)
		if err != nil {
			return taskRecord{}, fmt.Errorf("encode task %s error: %w", normalized.ID, err)
		}
		taskErrorJSON = nullableTaskString(string(payload))
	}

	return taskRecord{
		ID:                normalized.ID,
		UserID:            normalized.UserID,
		SessionID:         normalized.SessionID,
		SourceTask:        nullableTaskString(normalized.SourceTaskID),
		Request:           normalized.Request,
		Prompt:            nullableTaskString(normalized.Prompt),
		Image:             nullableTaskString(normalized.Image),
		Status:            string(normalized.Status),
		Error:             taskErrorJSON,
		Timeline:          string(timelineJSON),
		Context:           normalized.Context.Raw(),
		ProgressMessageID: nullableTaskString(normalized.ProgressMessageID),
		ResultMessageID:   nullableTaskString(normalized.ResultMessageID),
	}, nil
}

func decodeTask(record taskRecord) (domaintask.Task, error) {
	context, err := domaintask.NewContext(record.Context)
	if err != nil {
		return domaintask.Task{}, fmt.Errorf("decode task %s context: %w", record.ID, err)
	}

	var timeline domaintask.Timeline
	if err := json.Unmarshal([]byte(record.Timeline), &timeline); err != nil {
		return domaintask.Task{}, fmt.Errorf("decode task %s timeline: %w", record.ID, err)
	}

	var taskError *domaintask.TaskError
	if record.Error.Valid {
		var decoded domaintask.TaskError
		if err := json.Unmarshal([]byte(record.Error.String), &decoded); err != nil {
			return domaintask.Task{}, fmt.Errorf("decode task %s error: %w", record.ID, err)
		}
		taskError = &decoded
	}

	task, err := domaintask.Restore(
		record.ID,
		record.UserID,
		record.SessionID,
		taskString(record.SourceTask),
		record.Request,
		taskString(record.Prompt),
		taskString(record.Image),
		domaintask.Status(record.Status),
		taskError,
		timeline,
		context,
		taskString(record.ProgressMessageID),
		taskString(record.ResultMessageID),
	)
	if err != nil {
		return domaintask.Task{}, fmt.Errorf("decode task %s: %w", record.ID, err)
	}
	return task, nil
}

func scanTasks(rows *sql.Rows) ([]domaintask.Task, error) {
	var tasks []domaintask.Task
	for rows.Next() {
		var record taskRecord
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.SessionID,
			&record.SourceTask,
			&record.Request,
			&record.Prompt,
			&record.Image,
			&record.Status,
			&record.Error,
			&record.Timeline,
			&record.Context,
			&record.ProgressMessageID,
			&record.ResultMessageID,
		); err != nil {
			return nil, err
		}

		task, err := decodeTask(record)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func ensureTaskImmutableFields(existing taskRecord, updated taskRecord) error {
	switch {
	case existing.UserID != updated.UserID:
		return fmt.Errorf("update task %s: task user_id is immutable", updated.ID)
	case existing.SessionID != updated.SessionID:
		return fmt.Errorf("update task %s: task session_id is immutable", updated.ID)
	case taskString(existing.SourceTask) != taskString(updated.SourceTask):
		return fmt.Errorf("update task %s: task source_task is immutable", updated.ID)
	case existing.Request != updated.Request:
		return fmt.Errorf("update task %s: task request is immutable", updated.ID)
	case existing.Context != updated.Context:
		return fmt.Errorf("update task %s: task context is immutable", updated.ID)
	default:
		return nil
	}
}

func sortTasksByCreatedAt(tasks []domaintask.Task) {
	sort.Slice(tasks, func(i int, j int) bool {
		if tasks[i].Timeline.CreatedAt.Equal(tasks[j].Timeline.CreatedAt) {
			return tasks[i].ID < tasks[j].ID
		}
		return tasks[i].Timeline.CreatedAt.Before(tasks[j].Timeline.CreatedAt)
	})
}

func nullableTaskString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: value,
		Valid:  true,
	}
}

func taskString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
