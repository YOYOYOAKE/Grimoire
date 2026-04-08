package runner

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	localstore "grimoire/internal/adapters/filestore/local"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
	platformdb "grimoire/internal/platform/db"
	platformid "grimoire/internal/platform/id"
)

func TestRunWithSQLiteRepositoryPersistsCompletedTaskAndImage(t *testing.T) {
	ctx := context.Background()
	rootDir := t.TempDir()
	db := openRunnerIntegrationDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	createRunnerSessionFixture(t, db, "user-1", "session-1")

	task := mustRunnerTaskWithContext(t, "task-1", "user-1", "session-1", `{"version":1,"shape":"square","artists":"artist:foo"}`)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	store, err := localstore.NewImageStore(platformdb.SQLiteLayout{
		RootDir:  rootDir,
		ImageDir: filepath.Join(rootDir, "data", "images"),
	})
	if err != nil {
		t.Fatalf("new image store: %v", err)
	}

	service := NewService(
		taskRepo,
		txRunner,
		&translatorStub{result: domaindraw.Translation{Prompt: "moonlit_girl", NegativePrompt: "blurry"}},
		&imageGeneratorStub{image: []byte("jpg")},
		store,
		&notifierStub{sendTextID: "progress-1", sendImageID: "result-1"},
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(ctx, RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	stored, err := taskRepo.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != domaintask.StatusCompleted {
		t.Fatalf("unexpected status: %s", stored.Status)
	}
	if stored.Image != "data/images/user-1/task-1.jpg" {
		t.Fatalf("unexpected image path: %q", stored.Image)
	}
	if stored.Prompt != "artist:foo, moonlit_girl" {
		t.Fatalf("unexpected prompt: %q", stored.Prompt)
	}
	if stored.ProgressMessageID != "progress-1" || stored.ResultMessageID != "result-1" {
		t.Fatalf("unexpected message ids: progress=%q result=%q", stored.ProgressMessageID, stored.ResultMessageID)
	}
	if _, err := os.Stat(filepath.Join(rootDir, stored.Image)); err != nil {
		t.Fatalf("expected image file to exist: %v", err)
	}
}

func TestRunWithSQLiteRepositoryPersistsFailureState(t *testing.T) {
	ctx := context.Background()
	db := openRunnerIntegrationDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	createRunnerSessionFixture(t, db, "user-1", "session-1")

	task := mustRunnerTaskWithContext(t, "task-1", "user-1", "session-1", `{"version":1,"shape":"square"}`)
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	service := NewService(
		taskRepo,
		txRunner,
		&translatorStub{err: context.DeadlineExceeded},
		&imageGeneratorStub{},
		&imageStoreStub{path: "data/images/user-1/task-1.jpg"},
		&notifierStub{sendTextID: "progress-1"},
		func() time.Time { return time.Unix(10, 0).UTC() },
	)

	if err := service.Run(ctx, RunCommand{TaskID: "task-1"}); err != nil {
		t.Fatalf("run task: %v", err)
	}

	stored, err := taskRepo.Get(ctx, "task-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.Status != domaintask.StatusFailed {
		t.Fatalf("unexpected status: %s", stored.Status)
	}
	if stored.Error == nil || stored.Error.Code != "PROMPT_TRANSLATE_FAILED" {
		t.Fatalf("unexpected task error: %#v", stored.Error)
	}
}

func openRunnerIntegrationDB(t *testing.T) *sql.DB {
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

func createRunnerSessionFixture(t *testing.T, db *sql.DB, userID string, sessionID string) {
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

func mustRunnerTaskWithContext(t *testing.T, taskID string, userID string, sessionID string, rawContext string) domaintask.Task {
	t.Helper()

	contextSnapshot, err := domaintask.NewContext(rawContext)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(taskID, userID, sessionID, "draw a moonlit girl", contextSnapshot, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	return task
}
