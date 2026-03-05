package sqlite

import (
	"context"
	"database/sql"
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
