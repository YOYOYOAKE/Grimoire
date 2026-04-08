package task

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

type taskRepositoryStub struct {
	createErr  error
	getErr     error
	updateErr  error
	created    domaintask.Task
	updated    domaintask.Task
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
	if s.order != nil {
		*s.order = append(*s.order, "repo:get")
	}
	if s.getErr != nil {
		return domaintask.Task{}, s.getErr
	}
	if s.storedTask.ID != id {
		return domaintask.Task{}, fmt.Errorf("task %s not found", id)
	}
	return s.storedTask, nil
}

func (s *taskRepositoryStub) Update(_ context.Context, task domaintask.Task) error {
	if s.order != nil {
		*s.order = append(*s.order, "repo:update")
	}
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = task
	s.storedTask = task
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
	db := sqlitefixture.OpenDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	sqlitefixture.CreateUserAndSession(t, db, "user-1", "session-1", domainpreferences.DefaultPreference())

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

func TestStopMarksTaskStoppedWithinTransaction(t *testing.T) {
	order := []string{}
	repository := &taskRepositoryStub{
		storedTask: mustDrawingTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl"),
		order:      &order,
	}
	txRunner := &taskTxRunnerStub{order: &order}
	service := NewService(repository, txRunner, nil, func() time.Time { return time.Unix(4, 0).UTC() }, nil)

	task, err := service.Stop(context.Background(), StopCommand{TaskID: " task-1 ", UserID: " user-1 "})
	if err != nil {
		t.Fatalf("stop task: %v", err)
	}

	if task.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if task.Timeline.StoppedAt == nil || !task.Timeline.StoppedAt.Equal(time.Unix(4, 0).UTC()) {
		t.Fatalf("unexpected stopped timeline: %#v", task.Timeline)
	}
	if repository.updated.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected persisted status: %s", repository.updated.Status)
	}
	if repository.updated.Prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected persisted prompt: %q", repository.updated.Prompt)
	}
	expectedOrder := []string{"tx:start", "repo:get", "repo:update", "tx:commit"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestStopReturnsExistingStoppedTaskWithoutExtraUpdate(t *testing.T) {
	order := []string{}
	taskFixture := mustDrawingTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl")
	if err := taskFixture.MarkStopped(time.Unix(4, 0).UTC()); err != nil {
		t.Fatalf("mark stopped: %v", err)
	}

	service := NewService(
		&taskRepositoryStub{storedTask: taskFixture, order: &order},
		&taskTxRunnerStub{order: &order},
		nil,
		func() time.Time { return time.Unix(5, 0).UTC() },
		nil,
	)

	task, err := service.Stop(context.Background(), StopCommand{TaskID: "task-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("stop task: %v", err)
	}
	if task.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	expectedOrder := []string{"tx:start", "repo:get", "tx:commit"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestStopRequiresTxRunner(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	_, err := service.Stop(context.Background(), StopCommand{TaskID: "task-1", UserID: "user-1"})
	if !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected tx runner required error, got %v", err)
	}
}

func TestStopRejectsTaskOwnedByAnotherUser(t *testing.T) {
	order := []string{}
	service := NewService(
		&taskRepositoryStub{
			storedTask: mustDrawingTaskFixture(t, "task-1", "user-2", "session-1", "draw a moonlit girl"),
			order:      &order,
		},
		&taskTxRunnerStub{order: &order},
		nil,
		func() time.Time { return time.Unix(4, 0).UTC() },
		nil,
	)

	_, err := service.Stop(context.Background(), StopCommand{TaskID: "task-1", UserID: "user-1"})
	if !errors.Is(err, ErrTaskAccessDenied) {
		t.Fatalf("expected task access denied error, got %v", err)
	}
	expectedOrder := []string{"tx:start", "repo:get"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestRetryTranslateCreatesChildTaskAndClearsPrompt(t *testing.T) {
	order := []string{}
	source := mustDrawingTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl")
	repository := &taskRepositoryStub{
		storedTask: source,
		order:      &order,
	}
	txRunner := &taskTxRunnerStub{order: &order}
	scheduler := &schedulerStub{order: &order}
	now := func() time.Time { return time.Unix(5, 0).UTC() }
	service := NewService(repository, txRunner, scheduler, now, func() string { return "task-2" })

	task, err := service.RetryTranslate(context.Background(), RetryCommand{TaskID: " task-1 ", UserID: " user-1 "})
	if err != nil {
		t.Fatalf("retry translate: %v", err)
	}

	if task.ID != "task-2" {
		t.Fatalf("unexpected task id: %q", task.ID)
	}
	if task.SourceTaskID != "task-1" {
		t.Fatalf("unexpected source task id: %q", task.SourceTaskID)
	}
	if task.Prompt != "" {
		t.Fatalf("expected prompt to be cleared, got %q", task.Prompt)
	}
	if task.Request != source.Request || task.Context.Raw() != source.Context.Raw() {
		t.Fatalf("retry task did not preserve source snapshot: %#v", task)
	}
	if task.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if !task.Timeline.CreatedAt.Equal(now()) {
		t.Fatalf("unexpected created at: %v", task.Timeline.CreatedAt)
	}
	if scheduler.taskID != "task-2" {
		t.Fatalf("unexpected scheduled task id: %q", scheduler.taskID)
	}
	expectedOrder := []string{"tx:start", "repo:get", "repo:create", "tx:commit", "schedule"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", expectedOrder) {
		t.Fatalf("unexpected execution order: got %v want %v", order, expectedOrder)
	}
}

func TestRetryDrawCreatesChildTaskAndReusesPrompt(t *testing.T) {
	source := mustDrawingTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl")
	service := NewService(
		&taskRepositoryStub{storedTask: source},
		&taskTxRunnerStub{},
		&schedulerStub{},
		func() time.Time { return time.Unix(5, 0).UTC() },
		func() string { return "task-2" },
	)

	task, err := service.RetryDraw(context.Background(), RetryCommand{TaskID: "task-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("retry draw: %v", err)
	}
	if task.SourceTaskID != "task-1" {
		t.Fatalf("unexpected source task id: %q", task.SourceTaskID)
	}
	if task.Prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", task.Prompt)
	}
}

func TestRetryDrawReturnsPersistedTaskWhenSchedulerFails(t *testing.T) {
	db := sqlitefixture.OpenDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	sqlitefixture.CreateUserAndSession(t, db, "user-1", "session-1", domainpreferences.DefaultPreference())

	source := mustTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl", time.Unix(1, 0).UTC())
	if err := source.SetPrompt("masterpiece, moonlit_girl"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := taskRepo.Create(context.Background(), source); err != nil {
		t.Fatalf("create source task: %v", err)
	}

	schedulerErr := errors.New("queue unavailable")
	service := NewService(
		taskRepo,
		txRunner,
		&schedulerStub{err: schedulerErr},
		func() time.Time { return time.Unix(5, 0).UTC() },
		func() string { return "task-2" },
	)

	task, err := service.RetryDraw(context.Background(), RetryCommand{TaskID: "task-1", UserID: "user-1"})
	if !errors.Is(err, schedulerErr) {
		t.Fatalf("expected scheduler error, got %v", err)
	}

	stored, err := taskRepo.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get persisted task: %v", err)
	}
	if stored.SourceTaskID != "task-1" {
		t.Fatalf("unexpected persisted source task: %q", stored.SourceTaskID)
	}
	if stored.Prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected persisted prompt: %q", stored.Prompt)
	}
}

func TestRetryDrawRejectsSourceWithoutPrompt(t *testing.T) {
	repository := &taskRepositoryStub{
		storedTask: mustTaskFixture(t, "task-1", "user-1", "session-1", "draw a moonlit girl", time.Unix(1, 0).UTC()),
	}
	scheduler := &schedulerStub{}
	service := NewService(repository, &taskTxRunnerStub{}, scheduler, nil, func() string { return "task-2" })

	_, err := service.RetryDraw(context.Background(), RetryCommand{TaskID: "task-1", UserID: "user-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if repository.created.ID != "" {
		t.Fatalf("expected no retry task to be created, got %#v", repository.created)
	}
	if scheduler.taskID != "" {
		t.Fatalf("expected no enqueue, got %q", scheduler.taskID)
	}
}

func TestRetryTranslateRejectsTaskOwnedByAnotherUser(t *testing.T) {
	repository := &taskRepositoryStub{
		storedTask: mustDrawingTaskFixture(t, "task-1", "user-2", "session-1", "draw a moonlit girl"),
	}
	scheduler := &schedulerStub{}
	service := NewService(repository, &taskTxRunnerStub{}, scheduler, nil, func() string { return "task-2" })

	_, err := service.RetryTranslate(context.Background(), RetryCommand{TaskID: "task-1", UserID: "user-1"})
	if !errors.Is(err, ErrTaskAccessDenied) {
		t.Fatalf("expected task access denied error, got %v", err)
	}
	if repository.created.ID != "" {
		t.Fatalf("expected no retry task to be created, got %#v", repository.created)
	}
	if scheduler.taskID != "" {
		t.Fatalf("expected no enqueue, got %q", scheduler.taskID)
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

	prompt, err := service.GetPrompt(context.Background(), GetPromptCommand{TaskID: "task-1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if prompt != "masterpiece, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestGetPromptRejectsTaskOwnedByAnotherUser(t *testing.T) {
	taskFixture := mustTaskFixture(t, "task-1", "user-2", "session-1", "draw a moonlit girl", time.Unix(1, 0).UTC())
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

	_, err := service.GetPrompt(context.Background(), GetPromptCommand{TaskID: "task-1", UserID: "user-1"})
	if !errors.Is(err, ErrTaskAccessDenied) {
		t.Fatalf("expected task access denied error, got %v", err)
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

func mustDrawingTaskFixture(t *testing.T, id string, userID string, sessionID string, request string) domaintask.Task {
	t.Helper()
	task := mustTaskFixture(t, id, userID, sessionID, request, time.Unix(1, 0).UTC())
	if err := task.SetPrompt("masterpiece, moonlit_girl"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := task.MarkTranslating(time.Unix(2, 0).UTC()); err != nil {
		t.Fatalf("mark translating: %v", err)
	}
	if err := task.MarkDrawing(time.Unix(3, 0).UTC()); err != nil {
		t.Fatalf("mark drawing: %v", err)
	}
	return task
}
