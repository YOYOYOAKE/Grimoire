package draw

import (
	"context"
	"errors"
	"testing"
	"time"

	preferencesapp "grimoire/internal/app/preferences"
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
	preference domainpreferences.UserPreference
	err        error
}

func (s *preferenceRepoStub) GetByUserID(_ context.Context, userID int64) (domainpreferences.UserPreference, error) {
	if s.err != nil {
		return domainpreferences.UserPreference{}, s.err
	}
	if s.preference.UserID == 0 {
		return domainpreferences.UserPreference{}, preferencesapp.ErrPreferenceNotFound
	}
	return s.preference, nil
}

type schedulerStub struct {
	taskIDs []string
}

func (s *schedulerStub) Enqueue(taskID string) int {
	s.taskIDs = append(s.taskIDs, taskID)
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
	texts    []string
	photos   int
	photoErr error
}

func (s *notifierStub) SendText(_ context.Context, _ int64, _ int64, text string) (int64, error) {
	s.texts = append(s.texts, text)
	return int64(len(s.texts)), nil
}

func (s *notifierStub) EditText(_ context.Context, _ int64, _ int64, text string) error {
	s.texts = append(s.texts, text)
	return nil
}

func (s *notifierStub) SendPhoto(_ context.Context, _ int64, _ string, _ string, _ []byte) error {
	if s.photoErr != nil {
		return s.photoErr
	}
	s.photos++
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
		UserID:           2,
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
	if notifier.photos != 1 {
		t.Fatalf("expected 1 photo, got %d", notifier.photos)
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
		UserID: 2,
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
	notifier := &notifierStub{photoErr: errors.New("send failed")}
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
		UserID: 2,
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
