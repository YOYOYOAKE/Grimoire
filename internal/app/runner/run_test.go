package runner

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domaindraw "grimoire/internal/domain/draw"
	domaintask "grimoire/internal/domain/task"
)

type translatorStub struct {
	result    domaindraw.Translation
	err       error
	request   string
	shape     domaindraw.Shape
	callCount int
}

func (s *translatorStub) Translate(_ context.Context, request string, shape domaindraw.Shape) (domaindraw.Translation, error) {
	s.callCount++
	s.request = request
	s.shape = shape
	if s.err != nil {
		return domaindraw.Translation{}, s.err
	}
	return s.result, nil
}

type imageGeneratorStub struct {
	image     []byte
	err       error
	lastReq   domaindraw.GenerateRequest
	callCount int
	onCall    func()
}

func (s *imageGeneratorStub) Generate(_ context.Context, req domaindraw.GenerateRequest) ([]byte, error) {
	s.callCount++
	s.lastReq = req
	if s.onCall != nil {
		s.onCall()
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.image, nil
}

type imageStoreStub struct {
	path      string
	err       error
	userID    string
	taskID    string
	content   []byte
	callCount int
}

func (s *imageStoreStub) Save(_ context.Context, userID string, taskID string, content []byte) (string, error) {
	s.callCount++
	s.userID = userID
	s.taskID = taskID
	s.content = append([]byte(nil), content...)
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

type notifierStub struct {
	sendTextID    string
	sendTextErr   error
	sendImageID   string
	sendImageErr  error
	editTextErr   error
	deleteErr     error
	sentTexts     []string
	sentOptions   []MessageOptions
	editedTexts   []string
	editedOptions []MessageOptions
	deletedIDs    []string
	sentImagePath string
	sentImageOpts []MessageOptions
	callSendImage int
}

func (s *notifierStub) SendText(_ context.Context, _ string, text string, options MessageOptions) (string, error) {
	s.sentTexts = append(s.sentTexts, text)
	s.sentOptions = append(s.sentOptions, options)
	if s.sendTextErr != nil {
		return "", s.sendTextErr
	}
	if s.sendTextID == "" {
		return "progress-1", nil
	}
	return s.sendTextID, nil
}

func (s *notifierStub) EditText(_ context.Context, _ string, _ string, text string, options MessageOptions) error {
	s.editedTexts = append(s.editedTexts, text)
	s.editedOptions = append(s.editedOptions, options)
	return s.editTextErr
}

func (s *notifierStub) SendImage(_ context.Context, _ string, path string, _ string, options MessageOptions) (string, error) {
	s.callSendImage++
	s.sentImagePath = path
	s.sentImageOpts = append(s.sentImageOpts, options)
	if s.sendImageErr != nil {
		return "", s.sendImageErr
	}
	if s.sendImageID == "" {
		return "result-1", nil
	}
	return s.sendImageID, nil
}

func (s *notifierStub) DeleteMessage(_ context.Context, _ string, messageID string) error {
	s.deletedIDs = append(s.deletedIDs, messageID)
	return s.deleteErr
}

func TestRunSuccessPathPersistsImageAndMessages(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerQueuedTaskWithContext(t, "task-1", `{"version":1,"shape":"square","artists":"artist:foo"}`),
	}
	translator := &translatorStub{
		result: domaindraw.Translation{
			Prompt:         "moonlit_girl",
			NegativePrompt: "blurry",
		},
	}
	generator := &imageGeneratorStub{image: []byte("png")}
	store := &imageStoreStub{path: "data/images/user-1/task-1.jpg"}
	notifier := &notifierStub{sendTextID: "progress-1", sendImageID: "result-1"}
	service := NewService(repository, &runnerTxRunnerStub{}, translator, generator, store, notifier, func() time.Time { return time.Unix(10, 0).UTC() })

	if err := service.Run(context.Background(), RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	if translator.callCount != 1 || translator.request != "draw a moonlit girl" || translator.shape != domaindraw.ShapeSquare {
		t.Fatalf("unexpected translate input: count=%d request=%q shape=%s", translator.callCount, translator.request, translator.shape)
	}
	if generator.callCount != 1 {
		t.Fatalf("expected one generate call, got %d", generator.callCount)
	}
	if generator.lastReq.Prompt != "artist:foo, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", generator.lastReq.Prompt)
	}
	if generator.lastReq.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", generator.lastReq.NegativePrompt)
	}
	if generator.lastReq.Shape != domaindraw.ShapeSquare {
		t.Fatalf("unexpected shape: %s", generator.lastReq.Shape)
	}
	if store.callCount != 1 || store.userID != "user-1" || store.taskID != "task-1" || string(store.content) != "png" {
		t.Fatalf("unexpected image store call: %#v", store)
	}
	if notifier.callSendImage != 1 || notifier.sentImagePath != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected send image call: %#v", notifier)
	}
	if len(notifier.sentImageOpts) != 1 || notifier.sentImageOpts[0].TaskID != "task-1" || notifier.sentImageOpts[0].Variant != MessageVariantResult {
		t.Fatalf("unexpected send image options: %#v", notifier.sentImageOpts)
	}
	if len(notifier.sentTexts) != 1 || notifier.sentTexts[0] != "已入队" {
		t.Fatalf("unexpected sent texts: %#v", notifier.sentTexts)
	}
	if len(notifier.sentOptions) != 1 || notifier.sentOptions[0].TaskID != "task-1" || notifier.sentOptions[0].Variant != MessageVariantProgress {
		t.Fatalf("unexpected sent text options: %#v", notifier.sentOptions)
	}
	if len(notifier.editedTexts) != 2 || notifier.editedTexts[0] != "正在翻译提示词" || notifier.editedTexts[1] != "正在绘图" {
		t.Fatalf("unexpected edited texts: %#v", notifier.editedTexts)
	}
	if len(notifier.editedOptions) != 2 || notifier.editedOptions[0].TaskID != "task-1" || notifier.editedOptions[1].TaskID != "task-1" {
		t.Fatalf("unexpected edited options: %#v", notifier.editedOptions)
	}
	if len(notifier.deletedIDs) != 1 || notifier.deletedIDs[0] != "progress-1" {
		t.Fatalf("unexpected deleted ids: %#v", notifier.deletedIDs)
	}

	stored := repository.storedTask
	if stored.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected final status: %s", stored.Status)
	}
	if stored.Image != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected image path: %q", stored.Image)
	}
	if stored.ProgressMessageID != "progress-1" || stored.ResultMessageID != "result-1" {
		t.Fatalf("unexpected message ids: progress=%q result=%q", stored.ProgressMessageID, stored.ResultMessageID)
	}
}

func TestRunGeneratorFailureWritesStructuredTaskError(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerQueuedTaskWithContext(t, "task-1", `{"version":1,"shape":"square","artists":"artist:foo"}`),
	}
	service := NewService(
		repository,
		&runnerTxRunnerStub{},
		&translatorStub{result: domaindraw.Translation{Prompt: "moonlit_girl", NegativePrompt: "blurry"}},
		&imageGeneratorStub{err: errors.New("boom")},
		&imageStoreStub{path: "data/images/user-1/task-1.jpg"},
		&notifierStub{sendTextID: "progress-1"},
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(context.Background(), RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	stored := repository.storedTask
	if stored.Status != domaintask.StatusFailed {
		t.Fatalf("unexpected final status: %s", stored.Status)
	}
	if stored.Error == nil {
		t.Fatal("expected task error")
	}
	if stored.Error.Code != "IMAGE_GENERATE_FAILED" || stored.Error.Stage != "drawing" || stored.Error.Message != "boom" {
		t.Fatalf("unexpected task error: %#v", stored.Error)
	}
}

func TestRunSendImageFailureWritesStructuredTaskError(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerQueuedTaskWithContext(t, "task-1", `{"version":1,"shape":"square","artists":"artist:foo"}`),
	}
	store := &imageStoreStub{path: "data/images/user-1/task-1.jpg"}
	service := NewService(
		repository,
		&runnerTxRunnerStub{},
		&translatorStub{result: domaindraw.Translation{Prompt: "moonlit_girl", NegativePrompt: "blurry"}},
		&imageGeneratorStub{image: []byte("png")},
		store,
		&notifierStub{sendTextID: "progress-1", sendImageErr: errors.New("send failed")},
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(context.Background(), RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	if store.callCount != 1 {
		t.Fatalf("expected image saved before notify failure, got %d", store.callCount)
	}
	stored := repository.storedTask
	if stored.Status != domaintask.StatusFailed {
		t.Fatalf("unexpected final status: %s", stored.Status)
	}
	if stored.Error == nil || stored.Error.Code != "SEND_RESULT_FAILED" || stored.Error.Stage != "notifying" || stored.Error.Message != "send failed" {
		t.Fatalf("unexpected task error: %#v", stored.Error)
	}
}

func TestRunStopsAfterGenerateWhenTaskIsStoppedConcurrently(t *testing.T) {
	repository := &runnerTaskRepositoryStub{
		storedTask: mustRunnerQueuedTaskWithContext(t, "task-1", `{"version":1,"shape":"square","artists":"artist:foo"}`),
	}
	generator := &imageGeneratorStub{
		image: []byte("png"),
		onCall: func() {
			stopped := repository.storedTask
			if err := stopped.MarkStopped(time.Unix(20, 0).UTC()); err != nil {
				t.Fatalf("mark stopped: %v", err)
			}
			repository.storedTask = stopped
		},
	}
	store := &imageStoreStub{path: "data/images/user-1/task-1.jpg"}
	notifier := &notifierStub{sendTextID: "progress-1"}
	service := NewService(
		repository,
		&runnerTxRunnerStub{},
		&translatorStub{result: domaindraw.Translation{Prompt: "moonlit_girl", NegativePrompt: "blurry"}},
		generator,
		store,
		notifier,
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(context.Background(), RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	if store.callCount != 0 {
		t.Fatalf("expected image store skipped after stop, got %d", store.callCount)
	}
	if notifier.callSendImage != 0 {
		t.Fatalf("expected send image skipped after stop, got %d", notifier.callSendImage)
	}
	stored := repository.storedTask
	if stored.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected final status: %s", stored.Status)
	}
	if len(notifier.editedTexts) == 0 || notifier.editedTexts[len(notifier.editedTexts)-1] != "已停止任务" {
		t.Fatalf("unexpected edited texts: %#v", notifier.editedTexts)
	}
	if notifier.editedOptions[len(notifier.editedOptions)-1].Variant != MessageVariantNone {
		t.Fatalf("expected stopped text without progress controls, got %#v", notifier.editedOptions)
	}
}

func TestRunIgnoresMissingTask(t *testing.T) {
	service := NewService(
		&runnerTaskRepositoryStub{getErr: sql.ErrNoRows},
		&runnerTxRunnerStub{},
		&translatorStub{},
		&imageGeneratorStub{},
		&imageStoreStub{},
		&notifierStub{},
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(context.Background(), RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}
}

func mustRunnerQueuedTaskWithContext(t *testing.T, taskID string, rawContext string) domaintask.Task {
	t.Helper()
	contextSnapshot, err := domaintask.NewContext(rawContext)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(taskID, "user-1", "session-1", "draw a moonlit girl", contextSnapshot, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
