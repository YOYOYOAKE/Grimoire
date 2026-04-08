package recovery

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
	platformid "grimoire/internal/platform/id"
)

func TestRecoverWithSQLiteRepositoryRequeuesOnlyRecoverableTasks(t *testing.T) {
	ctx := context.Background()
	db := openRecoveryIntegrationDB(t)
	taskRepo := sqliterepo.NewTaskRepository(db)
	createRecoverySessionFixture(t, db, "user-1", "session-1")

	for _, task := range []domaintask.Task{
		mustRecoverySQLiteTask(t, "task-queued", domaintask.StatusQueued, time.Unix(1, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-translating", domaintask.StatusTranslating, time.Unix(2, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-drawing", domaintask.StatusDrawing, time.Unix(3, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-completed", domaintask.StatusCompleted, time.Unix(4, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-failed", domaintask.StatusFailed, time.Unix(5, 0).UTC()),
		mustRecoverySQLiteTask(t, "task-stopped", domaintask.StatusStopped, time.Unix(6, 0).UTC()),
	} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	scheduler := &recoverySchedulerStub{}
	service := NewService(taskRepo, scheduler)

	result, err := service.Recover(ctx, RecoverCommand{})
	if err != nil {
		t.Fatalf("recover: %v", err)
	}

	want := []string{"task-queued", "task-translating", "task-drawing"}
	if len(result.RequeuedTaskIDs) != len(want) {
		t.Fatalf("unexpected requeued count: %#v", result.RequeuedTaskIDs)
	}
	for index, taskID := range want {
		if result.RequeuedTaskIDs[index] != taskID {
			t.Fatalf("unexpected requeued ids: %#v", result.RequeuedTaskIDs)
		}
		if scheduler.taskIDs[index] != taskID {
			t.Fatalf("unexpected scheduled ids: %#v", scheduler.taskIDs)
		}
	}
}

func openRecoveryIntegrationDB(t *testing.T) *sql.DB {
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

func createRecoverySessionFixture(t *testing.T, db *sql.DB, userID string, sessionID string) {
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

func mustRecoverySQLiteTask(t *testing.T, id string, status domaintask.Status, createdAt time.Time) domaintask.Task {
	t.Helper()

	contextSnapshot, err := domaintask.NewContext(`{"version":1,"shape":"square"}`)
	if err != nil {
		t.Fatalf("new context: %v", err)
	}
	task, err := domaintask.New(id, "user-1", "session-1", "draw a moonlit girl", contextSnapshot, createdAt)
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
		if err := task.MarkCompleted("data/images/user-1/"+id+".jpg", createdAt.Add(3*time.Second)); err != nil {
			t.Fatalf("mark completed: %v", err)
		}
		return task
	case domaintask.StatusFailed:
		taskError, err := domaintask.NewError("PROMPT_TRANSLATE_FAILED", "translating", "boom")
		if err != nil {
			t.Fatalf("new task error: %v", err)
		}
		if err := task.MarkFailed(taskError, createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark failed: %v", err)
		}
		return task
	case domaintask.StatusStopped:
		if err := task.MarkStopped(createdAt.Add(time.Second)); err != nil {
			t.Fatalf("mark stopped: %v", err)
		}
		return task
	default:
		t.Fatalf("unsupported status: %s", status)
		return domaintask.Task{}
	}
}
