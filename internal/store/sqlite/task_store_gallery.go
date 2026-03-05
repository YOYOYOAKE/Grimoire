package sqlite

import (
	"context"
	"time"

	"grimoire/internal/store"
)

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
