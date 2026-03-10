package draw

import (
	"context"
	"errors"
	"testing"
	"time"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type taskRepoStub struct {
	tasks map[string]domaindraw.Task
}

func (s *taskRepoStub) Create(_ context.Context, task domaindraw.Task) error {
	s.tasks[task.ID] = task
	return nil
}

func (s *taskRepoStub) Get(_ context.Context, taskID string) (domaindraw.Task, error) {
	task, ok := s.tasks[taskID]
	if !ok {
		return domaindraw.Task{}, ErrTaskNotFound
	}
	return task, nil
}

func (s *taskRepoStub) Update(_ context.Context, task domaindraw.Task) error {
	s.tasks[task.ID] = task
	return nil
}

func (s *taskRepoStub) Delete(_ context.Context, taskID string) error {
	delete(s.tasks, taskID)
	return nil
}

type preferenceRepoStub struct {
	preference domainpreferences.Preference
	err        error
}

func (s *preferenceRepoStub) Get() (domainpreferences.Preference, error) {
	if s.err != nil {
		return domainpreferences.Preference{}, s.err
	}
	if !s.preference.Shape.Valid() {
		return domainpreferences.DefaultPreference(), nil
	}
	return s.preference, nil
}

type schedulerStub struct {
	taskIDs   []string
	onEnqueue func(taskID string) int
}

func (s *schedulerStub) Enqueue(taskID string) int {
	s.taskIDs = append(s.taskIDs, taskID)
	if s.onEnqueue != nil {
		return s.onEnqueue(taskID)
	}
	return len(s.taskIDs)
}

type translatorStub struct {
	result domaindraw.Translation
	err    error
}

func (s *translatorStub) Translate(_ context.Context, _ string, _ domaindraw.Shape) (domaindraw.Translation, error) {
	return s.result, s.err
}

type generatorStub struct {
	jobID   string
	updates []domaindraw.JobUpdate
	submit  error
	poll    error
}

func (s *generatorStub) Submit(_ context.Context, _ domaindraw.GenerateRequest) (string, error) {
	return s.jobID, s.submit
}

func (s *generatorStub) Poll(_ context.Context, _ string) (domaindraw.JobUpdate, error) {
	if s.poll != nil {
		return domaindraw.JobUpdate{}, s.poll
	}
	if len(s.updates) == 0 {
		return domaindraw.JobUpdate{}, errors.New("missing update")
	}
	update := s.updates[0]
	s.updates = s.updates[1:]
	return update, nil
}

type notifierStub struct {
	sentTexts    []string
	editedTexts  []string
	sendTextErr  error
	editTextErr  error
	sendPhotos   int
	sendReplyTo  []int64
	deleted      []int64
	sendPhotoErr error
	deleteErr    error
}

func (s *notifierStub) SendText(_ context.Context, _ int64, _ int64, text string) (int64, error) {
	if s.sendTextErr != nil {
		return 0, s.sendTextErr
	}
	s.sentTexts = append(s.sentTexts, text)
	return int64(len(s.sentTexts)), nil
}

func (s *notifierStub) EditText(_ context.Context, _ int64, _ int64, text string) error {
	if s.editTextErr != nil {
		return s.editTextErr
	}
	s.editedTexts = append(s.editedTexts, text)
	return nil
}

func (s *notifierStub) SendPhoto(_ context.Context, _ int64, replyToMessageID int64, _ string, _ string, _ []byte) error {
	if s.sendPhotoErr != nil {
		return s.sendPhotoErr
	}
	s.sendPhotos++
	s.sendReplyTo = append(s.sendReplyTo, replyToMessageID)
	return nil
}

func (s *notifierStub) DeleteMessage(_ context.Context, _ int64, messageID int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, messageID)
	return nil
}

func TestProcessSuccessDeletesTask(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	preferences := &preferenceRepoStub{}
	notifier := &notifierStub{}
	service := NewService(
		taskRepo,
		preferences,
		&translatorStub{result: domaindraw.Translation{PositivePrompt: "pos", NegativePrompt: "neg"}},
		&generatorStub{
			jobID: "job-1",
			updates: []domaindraw.JobUpdate{
				{Status: domaindraw.JobQueued, QueuePosition: 1},
				{Status: domaindraw.JobCompleted, Image: []byte("png")},
			},
		},
		notifier,
		func() time.Time { return time.Unix(100, 0) },
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	scheduler := &schedulerStub{}
	service.SetScheduler(scheduler)

	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID:           1,
		Prompt:           "moon",
		RequestMessageID: 3,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := service.Process(context.Background(), task.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if _, ok := taskRepo.tasks[task.ID]; ok {
		t.Fatal("expected task deleted")
	}
	if len(notifier.sentTexts) != 1 {
		t.Fatalf("expected exactly 1 status text message, got %d", len(notifier.sentTexts))
	}
	if notifier.sendPhotos != 1 {
		t.Fatalf("expected 1 sent photo, got %d", notifier.sendPhotos)
	}
	if len(notifier.sendReplyTo) != 1 || notifier.sendReplyTo[0] != 3 {
		t.Fatalf("expected photo reply to request message 3, got %#v", notifier.sendReplyTo)
	}
	if len(notifier.deleted) != 1 || notifier.deleted[0] != 1 {
		t.Fatalf("expected status message 1 deleted, got %#v", notifier.deleted)
	}
}

func TestProcessFailureDeletesTask(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	service := NewService(
		taskRepo,
		&preferenceRepoStub{},
		&translatorStub{err: errors.New("boom")},
		&generatorStub{},
		&notifierStub{},
		time.Now,
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	service.SetScheduler(&schedulerStub{})
	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID: 1,
		Prompt: "moon",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := service.Process(context.Background(), task.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if _, ok := taskRepo.tasks[task.ID]; ok {
		t.Fatal("expected task deleted")
	}
}

func TestProcessSendPhotoFailureDeletesTask(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	notifier := &notifierStub{sendPhotoErr: errors.New("send failed")}
	service := NewService(
		taskRepo,
		&preferenceRepoStub{},
		&translatorStub{result: domaindraw.Translation{PositivePrompt: "pos", NegativePrompt: "neg"}},
		&generatorStub{
			jobID: "job-1",
			updates: []domaindraw.JobUpdate{
				{Status: domaindraw.JobCompleted, Image: []byte("png")},
			},
		},
		notifier,
		time.Now,
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	service.SetScheduler(&schedulerStub{})
	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID: 1,
		Prompt: "moon",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := service.Process(context.Background(), task.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if _, ok := taskRepo.tasks[task.ID]; ok {
		t.Fatal("expected task deleted")
	}
	if len(notifier.deleted) != 0 {
		t.Fatalf("expected status message kept on photo failure, got %#v", notifier.deleted)
	}
}

func TestProcessSendsPhotoWhenStatusMessageMissing(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	notifier := &notifierStub{sendTextErr: errors.New("send text failed")}
	service := NewService(
		taskRepo,
		&preferenceRepoStub{},
		&translatorStub{result: domaindraw.Translation{PositivePrompt: "pos", NegativePrompt: "neg"}},
		&generatorStub{
			jobID: "job-1",
			updates: []domaindraw.JobUpdate{
				{Status: domaindraw.JobCompleted, Image: []byte("png")},
			},
		},
		notifier,
		time.Now,
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	service.SetScheduler(&schedulerStub{})
	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID: 1,
		Prompt: "moon",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if task.StatusMessageID != 0 {
		t.Fatalf("expected missing status message id, got %d", task.StatusMessageID)
	}
	if err := service.Process(context.Background(), task.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if notifier.sendPhotos != 1 {
		t.Fatalf("expected 1 sent photo, got %d", notifier.sendPhotos)
	}
	if len(notifier.sendReplyTo) != 1 || notifier.sendReplyTo[0] != 0 {
		t.Fatalf("expected direct photo send without reply target, got %#v", notifier.sendReplyTo)
	}
	if len(notifier.deleted) != 0 {
		t.Fatalf("expected no delete call without status message, got %#v", notifier.deleted)
	}
}

func TestProcessDoesNotSendReplacementStatusMessageOnEditFailure(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	notifier := &notifierStub{editTextErr: errors.New("edit failed")}
	service := NewService(
		taskRepo,
		&preferenceRepoStub{},
		&translatorStub{result: domaindraw.Translation{PositivePrompt: "pos", NegativePrompt: "neg"}},
		&generatorStub{
			jobID: "job-1",
			updates: []domaindraw.JobUpdate{
				{Status: domaindraw.JobQueued, QueuePosition: 1},
				{Status: domaindraw.JobCompleted, Image: []byte("png")},
			},
		},
		notifier,
		time.Now,
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	service.SetScheduler(&schedulerStub{})
	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID:           1,
		Prompt:           "moon",
		RequestMessageID: 3,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := service.Process(context.Background(), task.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(notifier.sentTexts) != 1 {
		t.Fatalf("expected only the initial status message, got %d", len(notifier.sentTexts))
	}
	if notifier.sendPhotos != 1 {
		t.Fatalf("expected completion photo send, got %d", notifier.sendPhotos)
	}
	if len(notifier.deleted) != 1 || notifier.deleted[0] != 1 {
		t.Fatalf("expected original status message deleted, got %#v", notifier.deleted)
	}
}

func TestSubmitStoresStatusMessageBeforeEnqueue(t *testing.T) {
	taskRepo := &taskRepoStub{tasks: map[string]domaindraw.Task{}}
	notifier := &notifierStub{}
	scheduler := &schedulerStub{}
	scheduler.onEnqueue = func(taskID string) int {
		task, err := taskRepo.Get(context.Background(), taskID)
		if err != nil {
			t.Fatalf("get task during enqueue: %v", err)
		}
		if task.StatusMessageID == 0 {
			t.Fatal("expected status message id stored before enqueue")
		}
		return len(scheduler.taskIDs)
	}

	service := NewService(
		taskRepo,
		&preferenceRepoStub{},
		&translatorStub{},
		&generatorStub{},
		notifier,
		time.Now,
		func() string { return "task-1" },
		time.Millisecond,
		nil,
	)
	service.SetScheduler(scheduler)

	task, err := service.Submit(context.Background(), SubmitCommand{
		ChatID:           1,
		Prompt:           "moon",
		RequestMessageID: 3,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if task.StatusMessageID != 1 {
		t.Fatalf("expected returned task to include status message id 1, got %d", task.StatusMessageID)
	}
	if len(notifier.sentTexts) != 1 || notifier.sentTexts[0] != "已入队" {
		t.Fatalf("unexpected queued text: %#v", notifier.sentTexts)
	}
}
