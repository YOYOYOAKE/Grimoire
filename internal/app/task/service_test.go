package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
	platformid "grimoire/internal/platform/id"
)

type taskRepositoryStub struct {
	createErr  error
	getErr     error
	created    domaintask.Task
	storedTask domaintask.Task
	order      *[]string
}

func (s *taskRepositoryStub) Create(_ context.Context, task domaintask.Task) error {
	if s.order != nil {
		*s.order = append(*s.order, "repo:create")
	}
	if s.createErr != nil {
		return s.createErr
	}
	s.created = task
	s.storedTask = task
	return nil
}

func (s *taskRepositoryStub) Get(_ context.Context, id string) (domaintask.Task, error) {
	if s.getErr != nil {
		return domaintask.Task{}, s.getErr
	}
	if s.storedTask.ID != id {
		return domaintask.Task{}, fmt.Errorf("task %s not found", id)
	}
	return s.storedTask, nil
}

func (s *taskRepositoryStub) Update(context.Context, domaintask.Task) error {
	return nil
}

func (s *taskRepositoryStub) ListRecoverable(context.Context) ([]domaintask.Task, error) {
	return nil, nil
}

func (s *taskRepositoryStub) ListBySourceTask(context.Context, string) ([]domaintask.Task, error) {
	return nil, nil
}

type taskTxRunnerStub struct {
	calls int
	order *[]string
}

func (s *taskTxRunnerStub) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	s.calls++
	if s.order != nil {
		*s.order = append(*s.order, "tx:start")
	}
	if err := fn(ctx); err != nil {
		return err
	}
	if s.order != nil {
		*s.order = append(*s.order, "tx:commit")
	}
	return nil
}

type schedulerStub struct {
	taskID string
	err    error
	order  *[]string
}

func (s *schedulerStub) Enqueue(taskID string) error {
	s.taskID = taskID
	if s.order != nil {
		*s.order = append(*s.order, "schedule")
	}
	return s.err
}

func TestCreatePersistsQueuedTaskAndEnqueuesAfterCommit(t *testing.T) {
	contextSnapshot := mustTaskContext(t, `{"summary":{"topic":"moon"}}`)
	order := []string{}
	repository := &taskRepositoryStub{order: &order}
	txRunner := &taskTxRunnerStub{order: &order}
	scheduler := &schedulerStub{order: &order}
	now := func() time.Time { return time.Unix(1, 0).UTC() }
	service := NewService(repository, txRunner, scheduler, now, func() string { return "task-1" })

	task, err := service.Create(context.Background(), CreateCommand{
		UserID:    "user-1",
		SessionID: "session-1",
		Request:   "draw a moonlit girl",
		Context:   contextSnapshot,
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected one transaction, got %d", txRunner.calls)
	}
	if task.ID != "task-1" || repository.created.ID != "task-1" {
		t.Fatalf("unexpected task id: %#v", task.ID)
	}
	if task.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Context.Raw() != `{"summary":{"topic":"moon"}}` {
		t.Fatalf("unexpected context: %q", task.Context.Raw())
	}
	if task.Timeline.CreatedAt != now() || task.Timeline.UpdatedAt != now() {
		t.Fatalf("unexpected timeline: %#v", task.Timeline)
	}
	if scheduler.taskID != "task-1" {
		t.Fatalf("unexpected scheduled task id: %q", scheduler.taskID)
	}
	expectedOrder := []string{"tx:start", "repo:create", "tx:commit", "schedule"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestCreateReturnsPersistedTaskWhenSchedulerFails(t *testing.T) {
	db := openTaskTestDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	createTaskSessionFixture(t, db, "user-1", "session-1")

	contextSnapshot := mustTaskContext(t, `{"summary":{"topic":"moon"}}`)
	schedulerErr := errors.New("queue unavailable")
	service := NewService(
		taskRepo,
		txRunner,
		&schedulerStub{err: schedulerErr},
		func() time.Time { return time.Unix(1, 0).UTC() },
		func() string { return "task-1" },
	)

	task, err := service.Create(context.Background(), CreateCommand{
		UserID:    "user-1",
		SessionID: "session-1",
		Request:   "draw a moonlit girl",
		Context:   contextSnapshot,
	})
	if !errors.Is(err, schedulerErr) {
		t.Fatalf("expected scheduler error, got %v", err)
	}

	stored, err := taskRepo.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get persisted task: %v", err)
	}
	if stored.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected persisted status: %s", stored.Status)
	}
	if stored.Context.Raw() != contextSnapshot.Raw() {
		t.Fatalf("unexpected persisted context: %q", stored.Context.Raw())
	}
}

func TestCreateRequiresTxRunner(t *testing.T) {
	service := NewService(nil, nil, &schedulerStub{}, nil, nil)

	_, err := service.Create(context.Background(), CreateCommand{
		UserID:    "user-1",
		SessionID: "session-1",
		Request:   "draw a moonlit girl",
		Context:   mustTaskContext(t, `{"summary":{"topic":"moon"}}`),
	})
	if !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected tx runner required error, got %v", err)
	}
}

func TestCreateRequiresScheduler(t *testing.T) {
	service := NewService(nil, &taskTxRunnerStub{}, nil, nil, nil)

	_, err := service.Create(context.Background(), CreateCommand{
		UserID:    "user-1",
		SessionID: "session-1",
		Request:   "draw a moonlit girl",
		Context:   mustTaskContext(t, `{"summary":{"topic":"moon"}}`),
	})
	if !errors.Is(err, ErrSchedulerRequired) {
		t.Fatalf("expected scheduler required error, got %v", err)
	}
}

func TestCreateReturnsRepositoryErrorWithoutScheduling(t *testing.T) {
	repositoryErr := errors.New("write failed")
	order := []string{}
	service := NewService(
		&taskRepositoryStub{createErr: repositoryErr, order: &order},
		&taskTxRunnerStub{order: &order},
		&schedulerStub{order: &order},
		func() time.Time { return time.Unix(1, 0).UTC() },
		func() string { return "task-1" },
	)

	_, err := service.Create(context.Background(), CreateCommand{
		UserID:    "user-1",
		SessionID: "session-1",
		Request:   "draw a moonlit girl",
		Context:   mustTaskContext(t, `{"summary":{"topic":"moon"}}`),
	})
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("expected repository error, got %v", err)
	}

	expectedOrder := []string{"tx:start", "repo:create"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestGetReturnsTask(t *testing.T) {
	taskFixture := mustTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl", time.Unix(1, 0).UTC())
	service := NewService(
		&taskRepositoryStub{storedTask: taskFixture},
		nil,
		nil,
		nil,
		nil,
	)

	task, err := service.Get(context.Background(), " task-1 ")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.ID != "task-1" {
		t.Fatalf("unexpected task id: %q", task.ID)
	}
}

func TestGetRejectsBlankTaskID(t *testing.T) {
	service := NewService(&taskRepositoryStub{}, nil, nil, nil, nil)

	_, err := service.Get(context.Background(), " \t ")
	if err == nil {
		t.Fatal("expected error for blank task id")
	}
}

func TestGetPromptReturnsPrompt(t *testing.T) {
	taskFixture := mustTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl", time.Unix(1, 0).UTC())
	if err := taskFixture.SetPrompt("masterpiece, moonlit_girl"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}

	service := NewService(
		&taskRepositoryStub{storedTask: taskFixture},
		nil,
		nil,
		nil,
		nil,
	)

	prompt, err := service.GetPrompt(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func openTaskTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sqliterepo.Open(context.Background(), filepath.Join(t.TempDir(), "grimoire.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := sqliterepo.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func createTaskSessionFixture(t *testing.T, db *sql.DB, userID string, sessionID string) {
	t.Helper()

	userRepo := sqliterepo.NewUserRepository(db)
	user, err := domainuser.New(userID, domainuser.RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator(sessionID))
	session, err := sessionRepo.GetOrCreateActiveByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session.ID != sessionID {
		t.Fatalf("unexpected session id: %q", session.ID)
	}
}

func mustTaskContext(t *testing.T, raw string) domaintask.Context {
	t.Helper()
	contextSnapshot, err := domaintask.NewContext(raw)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	return contextSnapshot
}

func mustTaskFixture(t *testing.T, id string, userID string, sessionID string, request string, createdAt time.Time) domaintask.Task {
	t.Helper()
	task, err := domaintask.New(id, userID, sessionID, request, mustTaskContext(t, `{"summary":{"topic":"moon"}}`), createdAt)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
