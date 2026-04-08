package telegram

import (
	"bytes"
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	conversationapp "grimoire/internal/app/conversation"
	preferencesapp "grimoire/internal/app/preferences"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
	platformid "grimoire/internal/platform/id"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

type sqliteConversationModelStub struct {
	output conversationapp.ConversationOutput
	input  conversationapp.ConversationInput
}

func (s *sqliteConversationModelStub) Converse(_ context.Context, input conversationapp.ConversationInput) (conversationapp.ConversationOutput, error) {
	s.input = input
	return s.output, nil
}

type telegramSchedulerStub struct {
	taskIDs []string
}

func (s *telegramSchedulerStub) Enqueue(taskID string) error {
	s.taskIDs = append(s.taskIDs, taskID)
	return nil
}

func TestHandleMessageWithSQLiteChatServicesCreatesTaskFromConversationToolCall(t *testing.T) {
	ctx := context.Background()
	db := sqlitefixture.OpenDB(t)
	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	sqlitefixture.CreateUserAndSession(t, db, "1", "session-1", preference)

	model := &sqliteConversationModelStub{
		output: conversationapp.ConversationOutput{
			Summary: domainsession.NewSummary(`{"goal":"moonlit_girl","ready":true}`),
			CreateDrawingTask: &conversationapp.CreateDrawingTask{
				Request: "绘制一位月下少女，夜景氛围，纵向构图。",
			},
		},
	}
	bot, taskRepo, scheduler, buffer := newSQLiteBackedTestBot(t, db, []string{"task-confirm"}, model)

	bot.handleMessage(ctx, Message{
		MessageID: 30,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "开始绘图",
	})

	stored, err := taskRepo.Get(ctx, "task-confirm")
	if err != nil {
		t.Fatalf("get created task: %v", err)
	}
	if stored.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected task status: %s", stored.Status)
	}
	if stored.Request != "绘制一位月下少女，夜景氛围，纵向构图。" {
		t.Fatalf("unexpected request: %q", stored.Request)
	}
	if stored.Context.Raw() != `{"version":1,"shape":"portrait","artists":"artist:foo"}` {
		t.Fatalf("unexpected task context: %q", stored.Context.Raw())
	}
	if len(scheduler.taskIDs) != 1 || scheduler.taskIDs[0] != "task-confirm" {
		t.Fatalf("unexpected scheduled task ids: %#v", scheduler.taskIDs)
	}
	if model.input.Preference.Shape != domaindraw.ShapePortrait || model.input.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected conversation preference: %#v", model.input.Preference)
	}
	if len(model.input.Messages) != 1 || model.input.Messages[0].Content != "开始绘图" {
		t.Fatalf("unexpected conversation messages: %#v", model.input.Messages)
	}
	if !strings.Contains(buffer.String(), `"text":"已开始绘图"`) {
		t.Fatalf("expected task started text, got %s", buffer.String())
	}
}

func TestHandleStopTaskCallbackWithSQLiteTaskServicePersistsStoppedTask(t *testing.T) {
	ctx := context.Background()
	db := sqlitefixture.OpenDB(t)
	sqlitefixture.CreateUserAndSession(t, db, "1", "session-1", domainpreferences.DefaultPreference())

	taskRepo := sqliterepo.NewTaskRepository(db)
	source := mustTelegramTaskAtStatus(t, "task-stop", domaintask.StatusDrawing, time.Unix(1, 0).UTC())
	if err := taskRepo.Create(ctx, source); err != nil {
		t.Fatalf("create drawing task: %v", err)
	}

	bot, _, _, buffer := newSQLiteBackedTestBot(t, db, nil, nil)
	bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-stop-real",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 31,
			Chat:      Chat{ID: 100},
		},
		Data: "task:stop:task-stop",
	})

	stored, err := taskRepo.Get(ctx, "task-stop")
	if err != nil {
		t.Fatalf("get stopped task: %v", err)
	}
	if stored.Status != domaintask.StatusStopped {
		t.Fatalf("unexpected task status: %s", stored.Status)
	}
	if stored.Timeline.StoppedAt == nil {
		t.Fatal("expected stopped timeline to be set")
	}
	if !strings.Contains(buffer.String(), `"text":"已停止任务"`) {
		t.Fatalf("expected stop acknowledgement, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `editMessageText`) {
		t.Fatalf("expected progress message edit, got %s", buffer.String())
	}
}

func TestHandleRetryCallbacksWithSQLiteTaskServiceCreateDerivedTasks(t *testing.T) {
	ctx := context.Background()
	db := sqlitefixture.OpenDB(t)
	sqlitefixture.CreateUserAndSession(t, db, "1", "session-1", domainpreferences.DefaultPreference())

	taskRepo := sqliterepo.NewTaskRepository(db)
	source := mustTelegramTaskAtStatus(t, "task-source", domaintask.StatusCompleted, time.Unix(1, 0).UTC())
	if err := taskRepo.Create(ctx, source); err != nil {
		t.Fatalf("create source task: %v", err)
	}

	bot, _, scheduler, buffer := newSQLiteBackedTestBot(t, db, []string{"task-retry-translate", "task-retry-draw"}, nil)

	bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-retry-translate-real",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 32,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:translate:task-source",
	})

	retryTranslate, err := taskRepo.Get(ctx, "task-retry-translate")
	if err != nil {
		t.Fatalf("get retry translate task: %v", err)
	}
	if retryTranslate.SourceTaskID != "task-source" {
		t.Fatalf("unexpected retry translate source: %q", retryTranslate.SourceTaskID)
	}
	if retryTranslate.Prompt != "" {
		t.Fatalf("expected retry translate to clear prompt, got %q", retryTranslate.Prompt)
	}

	bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-retry-draw-real",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 33,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:draw:task-source",
	})

	retryDraw, err := taskRepo.Get(ctx, "task-retry-draw")
	if err != nil {
		t.Fatalf("get retry draw task: %v", err)
	}
	if retryDraw.SourceTaskID != "task-source" {
		t.Fatalf("unexpected retry draw source: %q", retryDraw.SourceTaskID)
	}
	if retryDraw.Prompt != source.Prompt {
		t.Fatalf("unexpected retry draw prompt: %q", retryDraw.Prompt)
	}
	if len(scheduler.taskIDs) != 2 || scheduler.taskIDs[0] != "task-retry-translate" || scheduler.taskIDs[1] != "task-retry-draw" {
		t.Fatalf("unexpected scheduled task ids: %#v", scheduler.taskIDs)
	}
	if !strings.Contains(buffer.String(), `"text":"已重新翻译并开始绘图"`) {
		t.Fatalf("expected retry translate acknowledgement, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"text":"已开始重新绘图"`) {
		t.Fatalf("expected retry draw acknowledgement, got %s", buffer.String())
	}
}

func newSQLiteBackedTestBot(
	t *testing.T,
	db *sql.DB,
	taskIDs []string,
	model conversationapp.ConversationModel,
) (*Bot, *sqliterepo.TaskRepository, *telegramSchedulerStub, *bytes.Buffer) {
	t.Helper()

	bot, _, _, _, _, _, buffer := newTestBot(t)
	userRepo := sqliterepo.NewUserRepository(db)
	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator("unused-session"))
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	scheduler := &telegramSchedulerStub{}

	taskIndex := 0
	taskIDGenerator := func() string {
		if taskIndex < len(taskIDs) {
			taskID := taskIDs[taskIndex]
			taskIndex++
			return taskID
		}
		taskIndex++
		return "task-generated-" + strconv.Itoa(taskIndex)
	}

	if model != nil {
		sessionService := sessionapp.NewService(sessionRepo, messageRepo, txRunner)
		conversationService := conversationapp.NewService(
			model,
			sessionRepo,
			messageRepo,
			txRunner,
			10,
			func() time.Time { return time.Unix(9, 0).UTC() },
			func() string { return "assistant-message-1" },
		)
		bot.SetChatService(chatapp.NewService(
			userRepo,
			sessionService,
			conversationService,
			taskapp.NewService(
				taskRepo,
				txRunner,
				scheduler,
				func() time.Time { return time.Unix(10, 0).UTC() },
				taskIDGenerator,
			),
		))
	}
	bot.SetAccessService(accessapp.NewService(userRepo))
	bot.SetPreferenceService(preferencesapp.NewService(userRepo))
	bot.SetTaskService(taskapp.NewService(
		taskRepo,
		txRunner,
		scheduler,
		func() time.Time { return time.Unix(10, 0).UTC() },
		taskIDGenerator,
	))

	return bot, taskRepo, scheduler, buffer
}

func mustTelegramTaskAtStatus(t *testing.T, id string, status domaintask.Status, createdAt time.Time) domaintask.Task {
	t.Helper()

	contextSnapshot, err := domaintask.NewContext(`{"version":1,"shape":"small-square"}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(id, "1", "session-1", "draw a moonlit girl", contextSnapshot, createdAt)
	if err != nil {
		t.Fatalf("new task: %v", err)
	}

	switch status {
	case domaintask.StatusQueued:
		return task
	case domaintask.StatusDrawing:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		if err := task.SetPrompt("masterpiece, moonlit_girl"); err != nil {
			t.Fatalf("set prompt: %v", err)
		}
		if err := task.MarkDrawing(createdAt.Add(2 * time.Second)); err != nil {
			t.Fatalf("mark drawing: %v", err)
		}
		return task
	case domaintask.StatusCompleted:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		if err := task.SetPrompt("masterpiece, moonlit_girl"); err != nil {
			t.Fatalf("set prompt: %v", err)
		}
		if err := task.MarkDrawing(createdAt.Add(2 * time.Second)); err != nil {
			t.Fatalf("mark drawing: %v", err)
		}
		if err := task.MarkCompleted("data/images/1/"+id+".jpg", createdAt.Add(3*time.Second)); err != nil {
			t.Fatalf("mark completed: %v", err)
		}
		return task
	default:
		t.Fatalf("unsupported task status: %s", status)
		return domaintask.Task{}
	}
}
