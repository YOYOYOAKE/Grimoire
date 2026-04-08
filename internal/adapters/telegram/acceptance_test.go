package telegram

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	localstore "grimoire/internal/adapters/filestore/local"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	conversationapp "grimoire/internal/app/conversation"
	preferencesapp "grimoire/internal/app/preferences"
	requestapp "grimoire/internal/app/request"
	runnerapp "grimoire/internal/app/runner"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
	platformdb "grimoire/internal/platform/db"
	platformid "grimoire/internal/platform/id"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

type acceptanceConversationModelStub struct {
	output conversationapp.ConversationOutput
	input  conversationapp.ConversationInput
}

func (s *acceptanceConversationModelStub) Converse(_ context.Context, input conversationapp.ConversationInput) (conversationapp.ConversationOutput, error) {
	s.input = input
	return s.output, nil
}

type acceptanceTranslatorStub struct {
	translation domaindraw.Translation
}

func (s *acceptanceTranslatorStub) Translate(context.Context, string, domaindraw.Shape) (domaindraw.Translation, error) {
	return s.translation, nil
}

type acceptanceImageGeneratorStub struct {
	image []byte
}

func (s *acceptanceImageGeneratorStub) Generate(context.Context, domaindraw.GenerateRequest) ([]byte, error) {
	return append([]byte(nil), s.image...), nil
}

type acceptanceNotifier struct {
	nextID      int
	sentTexts   []string
	sentImages  []string
	deletedIDs  []string
	editedTexts []string
}

func (n *acceptanceNotifier) SendText(_ context.Context, _ string, text string, _ runnerapp.MessageOptions) (string, error) {
	n.nextID++
	n.sentTexts = append(n.sentTexts, text)
	return fmt.Sprintf("message-%d", n.nextID), nil
}

func (n *acceptanceNotifier) EditText(_ context.Context, _ string, _ string, text string, _ runnerapp.MessageOptions) error {
	n.editedTexts = append(n.editedTexts, text)
	return nil
}

func (n *acceptanceNotifier) SendImage(_ context.Context, _ string, path string, _ string, _ runnerapp.MessageOptions) (string, error) {
	n.nextID++
	n.sentImages = append(n.sentImages, path)
	return fmt.Sprintf("message-%d", n.nextID), nil
}

func (n *acceptanceNotifier) DeleteMessage(_ context.Context, _ string, messageID string) error {
	n.deletedIDs = append(n.deletedIDs, messageID)
	return nil
}

type acceptanceHarness struct {
	bot              *Bot
	buffer           *bytes.Buffer
	db               *sql.DB
	taskRepo         *sqliterepo.TaskRepository
	taskService      *taskapp.Service
	runnerService    *runnerapp.Service
	scheduler        *telegramSchedulerStub
	requestGenerator *telegramRequestGeneratorStub
	notifier         *acceptanceNotifier
}

func TestAcceptanceChatConfirmRunPromptAndRetryFlow(t *testing.T) {
	ctx := context.Background()
	harness := newAcceptanceHarness(t)

	harness.bot.handleMessage(ctx, Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "画一个月下的少女，补一点夜景氛围",
	})

	logOutput := harness.buffer.String()
	if !strings.Contains(logOutput, "需要补充一点光线方向。") {
		t.Fatalf("expected chat reply in output, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "待确认 request") {
		t.Fatalf("expected pending request in output, got %s", logOutput)
	}

	harness.bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-confirm-acceptance",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 20,
			Chat:      Chat{ID: 100},
		},
		Data: "request:confirm:session-1",
	})

	if len(harness.scheduler.taskIDs) != 1 {
		t.Fatalf("expected one queued task, got %#v", harness.scheduler.taskIDs)
	}
	taskID := harness.scheduler.taskIDs[0]
	if err := harness.runnerService.Run(ctx, runnerapp.RunCommand{TaskID: taskID}); err != nil {
		t.Fatalf("run accepted task: %v", err)
	}

	stored, err := harness.taskRepo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get completed task: %v", err)
	}
	if stored.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected task status: %s", stored.Status)
	}
	if strings.TrimSpace(stored.Prompt) == "" {
		t.Fatal("expected completed task prompt")
	}
	if len(harness.notifier.sentImages) != 1 {
		t.Fatalf("expected one result image notification, got %#v", harness.notifier.sentImages)
	}

	harness.buffer.Reset()
	harness.bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-prompt-acceptance",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 21,
			Chat:      Chat{ID: 100},
		},
		Data: "task:prompt:" + taskID,
	})
	promptOutput := harness.buffer.String()
	if !strings.Contains(promptOutput, "Prompt") || !strings.Contains(promptOutput, stored.Prompt) {
		t.Fatalf("expected prompt message in output, got %s", promptOutput)
	}

	harness.bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-retry-draw-acceptance",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 22,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:draw:" + taskID,
	})

	if len(harness.scheduler.taskIDs) != 2 {
		t.Fatalf("expected retry task to be queued, got %#v", harness.scheduler.taskIDs)
	}
	retryTaskID := harness.scheduler.taskIDs[1]
	retryTask, err := harness.taskRepo.Get(ctx, retryTaskID)
	if err != nil {
		t.Fatalf("get retry task: %v", err)
	}
	if retryTask.SourceTaskID != taskID {
		t.Fatalf("unexpected retry source: %q", retryTask.SourceTaskID)
	}
	if retryTask.Prompt != stored.Prompt {
		t.Fatalf("expected retry draw to reuse prompt %q, got %q", stored.Prompt, retryTask.Prompt)
	}
	if err := harness.runnerService.Run(ctx, runnerapp.RunCommand{TaskID: retryTaskID}); err != nil {
		t.Fatalf("run retry task: %v", err)
	}
	retryStored, err := harness.taskRepo.Get(ctx, retryTaskID)
	if err != nil {
		t.Fatalf("get completed retry task: %v", err)
	}
	if retryStored.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected retry task status: %s", retryStored.Status)
	}
}

func TestAcceptanceStopFlow(t *testing.T) {
	ctx := context.Background()
	harness := newAcceptanceHarness(t)

	harness.bot.handleMessage(ctx, Message{
		MessageID: 11,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "画一个准备停止的任务",
	})
	harness.bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-confirm-stop-flow",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 23,
			Chat:      Chat{ID: 100},
		},
		Data: "request:confirm:session-1",
	})

	if len(harness.scheduler.taskIDs) != 1 {
		t.Fatalf("expected one queued task, got %#v", harness.scheduler.taskIDs)
	}
	taskID := harness.scheduler.taskIDs[0]

	harness.bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-stop-acceptance",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 24,
			Chat:      Chat{ID: 100},
		},
		Data: "task:stop:" + taskID,
	})

	stored, err := harness.taskRepo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("get stopped task: %v", err)
	}
	if stored.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected task status: %s", stored.Status)
	}
	if err := harness.runnerService.Run(ctx, runnerapp.RunCommand{TaskID: taskID}); err != nil {
		t.Fatalf("run stopped task: %v", err)
	}
	stored, err = harness.taskRepo.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("reload stopped task: %v", err)
	}
	if stored.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected task status after runner: %s", stored.Status)
	}
	if !strings.Contains(harness.buffer.String(), "已停止任务") {
		t.Fatalf("expected stop message in output, got %s", harness.buffer.String())
	}
}

func newAcceptanceHarness(t *testing.T) acceptanceHarness {
	t.Helper()

	bot, _, _, _, _, _, _, buffer := newTestBot(t)
	db := sqlitefixture.OpenDB(t)
	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	sqlitefixture.CreateUserAndSession(t, db, "1", "session-1", preference)

	userRepo := sqliterepo.NewUserRepository(db)
	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)

	sessionService := sessionapp.NewService(sessionRepo, messageRepo, txRunner)
	conversationService := conversationapp.NewService(
		&acceptanceConversationModelStub{
			output: conversationapp.ConversationOutput{
				Reply:   "需要补充一点光线方向。",
				Summary: domainsession.NewSummary(`{"topic":"moon","step":"clarified"}`),
			},
		},
		sessionRepo,
		messageRepo,
		txRunner,
		15,
		func() time.Time { return time.Unix(2, 0).UTC() },
		func() string { return "assistant-msg-1" },
	)
	chatService := chatapp.NewService(userRepo, sessionService, conversationService)
	requestGenerator := &telegramRequestGeneratorStub{output: "draw a moonlit girl"}
	scheduler := &telegramSchedulerStub{}
	taskIndex := 0
	taskService := taskapp.NewService(
		taskRepo,
		txRunner,
		scheduler,
		func() time.Time { return time.Unix(3, 0).UTC() },
		func() string {
			taskIndex++
			return fmt.Sprintf("task-%d", taskIndex)
		},
	)

	rootDir := t.TempDir()
	imageStore, err := localstore.NewImageStore(platformdb.SQLiteLayout{
		RootDir:  rootDir,
		ImageDir: filepath.Join(rootDir, "data", "images"),
	})
	if err != nil {
		t.Fatalf("new image store: %v", err)
	}
	notifier := &acceptanceNotifier{}
	runnerService := runnerapp.NewService(
		taskRepo,
		txRunner,
		&acceptanceTranslatorStub{translation: domaindraw.Translation{Prompt: "moonlit_girl", NegativePrompt: "blurry"}},
		&acceptanceImageGeneratorStub{image: []byte("jpg")},
		imageStore,
		notifier,
		func() time.Time { return time.Unix(4, 0).UTC() },
	)

	bot.SetAccessService(accessapp.NewService(userRepo))
	bot.SetPreferenceService(preferencesapp.NewService(userRepo))
	bot.SetChatService(chatService)
	bot.SetRequestService(requestapp.NewService(requestGenerator, sessionRepo, messageRepo, 15))
	bot.SetTaskService(taskService)
	bot.SetBalanceService(&balanceServiceMock{})

	return acceptanceHarness{
		bot:              bot,
		buffer:           buffer,
		db:               db,
		taskRepo:         taskRepo,
		taskService:      taskService,
		runnerService:    runnerService,
		scheduler:        scheduler,
		requestGenerator: requestGenerator,
		notifier:         notifier,
	}
}

func waitForAcceptanceTaskStatus(t *testing.T, taskRepo *sqliterepo.TaskRepository, taskID string, status domaintask.Status) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		task, err := taskRepo.Get(context.Background(), taskID)
		if err != nil {
			t.Fatalf("get task %s: %v", taskID, err)
		}
		if task.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, err := taskRepo.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get final task %s: %v", taskID, err)
	}
	t.Fatalf("task %s did not reach %s, got %s", taskID, status, task.Status)
}
