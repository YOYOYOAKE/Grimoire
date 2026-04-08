package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	localstore "grimoire/internal/adapters/filestore/local"
	memoryqueue "grimoire/internal/adapters/queue/memory"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	recoveryapp "grimoire/internal/app/recovery"
	runnerapp "grimoire/internal/app/runner"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	platformdb "grimoire/internal/platform/db"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

type bootstrapAcceptanceTranslatorStub struct{}

func (bootstrapAcceptanceTranslatorStub) Translate(context.Context, string, domaindraw.Shape) (domaindraw.Translation, error) {
	return domaindraw.Translation{
		Prompt:         "moonlit_girl",
		NegativePrompt: "blurry",
	}, nil
}

type bootstrapAcceptanceImageGeneratorStub struct{}

func (bootstrapAcceptanceImageGeneratorStub) Generate(context.Context, domaindraw.GenerateRequest) ([]byte, error) {
	return []byte("jpg"), nil
}

type bootstrapAcceptanceNotifierStub struct {
	sentImagePaths []string
}

func (s *bootstrapAcceptanceNotifierStub) SendText(context.Context, string, string, runnerapp.MessageOptions) (string, error) {
	return "progress-1", nil
}

func (s *bootstrapAcceptanceNotifierStub) EditText(context.Context, string, string, string, runnerapp.MessageOptions) error {
	return nil
}

func (s *bootstrapAcceptanceNotifierStub) SendImage(_ context.Context, _ string, path string, _ string, _ runnerapp.MessageOptions) (string, error) {
	s.sentImagePaths = append(s.sentImagePaths, path)
	return "result-1", nil
}

func (s *bootstrapAcceptanceNotifierStub) DeleteMessage(context.Context, string, string) error {
	return nil
}

func TestAcceptanceStartupRecoveryFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rootDir := t.TempDir()
	db := sqlitefixture.OpenDB(t)
	sqlitefixture.CreateUserAndSession(t, db, "1", "session-1", domainpreferences.DefaultPreference())

	taskRepo := sqliterepo.NewTaskRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)
	task := mustBootstrapAcceptanceTask(t, "task-recover", domaintask.StatusTranslating, time.Unix(1, 0).UTC())
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create recoverable task: %v", err)
	}

	store, err := localstore.NewImageStore(platformdb.SQLiteLayout{
		RootDir:  rootDir,
		ImageDir: filepath.Join(rootDir, "data", "images"),
	})
	if err != nil {
		t.Fatalf("new image store: %v", err)
	}
	notifier := &bootstrapAcceptanceNotifierStub{}
	runnerService := runnerapp.NewService(
		taskRepo,
		txRunner,
		bootstrapAcceptanceTranslatorStub{},
		bootstrapAcceptanceImageGeneratorStub{},
		store,
		notifier,
		func() time.Time { return time.Unix(4, 0).UTC() },
		nil,
	)

	runErrCh := make(chan error, 1)
	worker := memoryqueue.NewWorker(1, func(runCtx context.Context, taskID string) {
		if err := runnerService.Run(runCtx, runnerapp.RunCommand{TaskID: taskID}); err != nil {
			runErrCh <- err
		}
	}, nil)
	recoveryService := recoveryapp.NewService(taskRepo, memoryqueue.NewScheduler(worker))
	app := &App{
		runnerWorker: worker,
		recovery:     recoveryService,
		wiring: reservedWiring{
			RecoveryEnabled: true,
		},
	}

	if err := app.startBackgroundServices(ctx); err != nil {
		t.Fatalf("start background services: %v", err)
	}

	waitForBootstrapAcceptanceTaskStatus(t, taskRepo, "task-recover", domaintask.StatusCompleted)

	select {
	case err := <-runErrCh:
		t.Fatalf("run recovered task: %v", err)
	default:
	}

	stored, err := taskRepo.Get(ctx, "task-recover")
	if err != nil {
		t.Fatalf("get recovered task: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, stored.Image)); err != nil {
		t.Fatalf("expected recovered image to exist: %v", err)
	}
	if len(notifier.sentImagePaths) != 1 {
		t.Fatalf("expected one result image notification, got %#v", notifier.sentImagePaths)
	}
}

func waitForBootstrapAcceptanceTaskStatus(t *testing.T, taskRepo *sqliterepo.TaskRepository, taskID string, status domaintask.Status) {
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

func mustBootstrapAcceptanceTask(t *testing.T, id string, status domaintask.Status, createdAt time.Time) domaintask.Task {
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
	case domaintask.StatusTranslating:
		if err := task.MarkTranslating(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark translating: %v", err)
		}
		return task
	default:
		t.Fatalf("unsupported task status: %s", status)
		return domaintask.Task{}
	}
}
