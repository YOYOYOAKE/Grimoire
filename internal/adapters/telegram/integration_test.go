package telegram

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	accessapp "grimoire/internal/app/access"
	preferencesapp "grimoire/internal/app/preferences"
	requestapp "grimoire/internal/app/request"
	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
	platformid "grimoire/internal/platform/id"
)

type telegramRequestGeneratorStub struct {
	output string
	input  requestapp.GenerateInput
}

func (s *telegramRequestGeneratorStub) Generate(_ context.Context, input requestapp.GenerateInput) (string, error) {
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

func TestRequestConfirmCallbackWithSQLiteServicesCreatesTask(t *testing.T) {
	ctx := context.Background()
	db := openTelegramIntegrationDB(t)
	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	seedTelegramUser(t, db, "1", preference)
	seedTelegramSession(t, db, "1", "session-1")
	appendTelegramSessionMessage(t, db, "session-1", "message-1", "画一个月下的少女", time.Unix(1, 0).UTC())

	bot, taskRepo, scheduler, generator, buffer := newSQLiteBackedTestBot(t, db, []string{"task-confirm"})
	generator.output = "draw a moonlit girl"

	bot.handleCallbackQuery(ctx, CallbackQuery{
		ID:   "cb-confirm-real",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 30,
			Chat:      Chat{ID: 100},
		},
		Data: "request:confirm:session-1",
	})

	stored, err := taskRepo.Get(ctx, "task-confirm")
	if err != nil {
		t.Fatalf("get created task: %v", err)
	}
	if stored.Status != domaintask.StatusQueued {
		t.Fatalf("unexpected task status: %s", stored.Status)
	}
	if stored.Request != "draw a moonlit girl" {
		t.Fatalf("unexpected request: %q", stored.Request)
	}
	if stored.Context.Raw() != `{"version":1,"shape":"portrait","artists":"artist:foo"}` {
		t.Fatalf("unexpected task context: %q", stored.Context.Raw())
	}
	if len(scheduler.taskIDs) != 1 || scheduler.taskIDs[0] != "task-confirm" {
		t.Fatalf("unexpected scheduled task ids: %#v", scheduler.taskIDs)
	}
	if generator.input.Preference.Shape != domaindraw.ShapePortrait || generator.input.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected request preference: %#v", generator.input.Preference)
	}
	if len(generator.input.Messages) != 1 || generator.input.Messages[0].Content != "画一个月下的少女" {
		t.Fatalf("unexpected request messages: %#v", generator.input.Messages)
	}
	if !strings.Contains(buffer.String(), `"text":"已开始执行"`) {
		t.Fatalf("expected confirm callback acknowledgement, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `已确认 request`) {
		t.Fatalf("expected confirmed request edit, got %s", buffer.String())
	}
}

func TestHandleStopTaskCallbackWithSQLiteTaskServicePersistsStoppedTask(t *testing.T) {
	ctx := context.Background()
	db := openTelegramIntegrationDB(t)
	seedTelegramUser(t, db, "1", domainpreferences.DefaultPreference())
	seedTelegramSession(t, db, "1", "session-1")

	taskRepo := sqliterepo.NewTaskRepository(db)
	source := mustTelegramTaskAtStatus(t, "task-stop", domaintask.StatusDrawing, time.Unix(1, 0).UTC())
	if err := taskRepo.Create(ctx, source); err != nil {
		t.Fatalf("create drawing task: %v", err)
	}

	bot, _, _, _, buffer := newSQLiteBackedTestBot(t, db, nil)
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
	db := openTelegramIntegrationDB(t)
	seedTelegramUser(t, db, "1", domainpreferences.DefaultPreference())
	seedTelegramSession(t, db, "1", "session-1")

	taskRepo := sqliterepo.NewTaskRepository(db)
	source := mustTelegramTaskAtStatus(t, "task-source", domaintask.StatusCompleted, time.Unix(1, 0).UTC())
	if err := taskRepo.Create(ctx, source); err != nil {
		t.Fatalf("create source task: %v", err)
	}

	bot, _, scheduler, _, buffer := newSQLiteBackedTestBot(t, db, []string{"task-retry-translate", "task-retry-draw"})

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
) (*Bot, *sqliterepo.TaskRepository, *telegramSchedulerStub, *telegramRequestGeneratorStub, *bytes.Buffer) {
	t.Helper()

	bot, _, _, _, _, _, _, buffer := newTestBot(t)
	userRepo := sqliterepo.NewUserRepository(db)
	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator("unused-session"))
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	scheduler := &telegramSchedulerStub{}
	generator := &telegramRequestGeneratorStub{}

	taskIndex := 0
	taskIDGenerator := func() string {
		if taskIndex < len(taskIDs) {
			taskID := taskIDs[taskIndex]
			taskIndex++
			return taskID
		}
		taskIndex++
		return "task-generated-" + time.Unix(int64(taskIndex), 0).UTC().Format("150405")
	}

	bot.SetAccessService(accessapp.NewService(userRepo))
	bot.SetPreferenceService(preferencesapp.NewService(userRepo))
	bot.SetRequestService(requestapp.NewService(generator, sessionRepo, messageRepo, 10))
	bot.SetTaskService(taskapp.NewService(
		taskRepo,
		txRunner,
		scheduler,
		func() time.Time { return time.Unix(10, 0).UTC() },
		taskIDGenerator,
	))

	return bot, taskRepo, scheduler, generator, buffer
}

func openTelegramIntegrationDB(t *testing.T) *sql.DB {
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

func seedTelegramUser(t *testing.T, db *sql.DB, telegramID string, preference domainpreferences.Preference) {
	t.Helper()

	userRepo := sqliterepo.NewUserRepository(db)
	user, err := domainuser.New(telegramID, domainuser.RoleNormal, preference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func seedTelegramSession(t *testing.T, db *sql.DB, userID string, sessionID string) domainsession.Session {
	t.Helper()

	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator(sessionID))
	session, err := sessionRepo.GetOrCreateActiveByUserID(context.Background(), userID)
	if err != nil {
		t.Fatalf("get or create session: %v", err)
	}
	if session.ID != sessionID {
		t.Fatalf("unexpected session id: %q", session.ID)
	}
	return session
}

func appendTelegramSessionMessage(
	t *testing.T,
	db *sql.DB,
	sessionID string,
	messageID string,
	content string,
	createdAt time.Time,
) {
	t.Helper()

	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator("unused-session"))
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	session, err := sessionRepo.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	message, err := domainsession.NewMessage(messageID, sessionID, domainsession.MessageRoleUser, content, createdAt)
	if err != nil {
		t.Fatalf("new session message: %v", err)
	}
	if err := messageRepo.Append(context.Background(), message); err != nil {
		t.Fatalf("append session message: %v", err)
	}
	if err := session.RecordMessage(message); err != nil {
		t.Fatalf("record session message: %v", err)
	}
	if err := sessionRepo.Save(context.Background(), session); err != nil {
		t.Fatalf("save session: %v", err)
	}
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
