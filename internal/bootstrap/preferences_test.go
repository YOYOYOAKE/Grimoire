package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

func TestEnsureAdminPreferenceUserSeedsMissingUserWithDefaultPreference(t *testing.T) {
	ctx := context.Background()
	userRepo := newTestUserRepository(t)

	if err := ensureAdminPreferenceUser(ctx, userRepo, "admin-1"); err != nil {
		t.Fatalf("ensure admin user: %v", err)
	}

	user, err := userRepo.GetByTelegramID(ctx, "admin-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.Preference != domainpreferences.DefaultPreference() {
		t.Fatalf("unexpected preference: %#v", user.Preference)
	}
}

func TestEnsureAdminPreferenceUserKeepsExistingSQLitePreference(t *testing.T) {
	ctx := context.Background()
	userRepo := newTestUserRepository(t)

	existingPreference, err := domainpreferences.New(domaindraw.ShapeLandscape, "artist:bar")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	user, err := domainuser.New("admin-1", domainuser.RoleNormal, existingPreference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := ensureAdminPreferenceUser(ctx, userRepo, "admin-1"); err != nil {
		t.Fatalf("ensure admin user: %v", err)
	}

	storedUser, err := userRepo.GetByTelegramID(ctx, "admin-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if storedUser.Preference != existingPreference {
		t.Fatalf("unexpected preference: %#v", storedUser.Preference)
	}
}

func TestPreparePreferenceRepositoryUsesSQLiteRepository(t *testing.T) {
	ctx := context.Background()
	repository, db, err := preparePreferenceRepository(
		ctx,
		filepath.Join(t.TempDir(), "state", "grimoire.sqlite"),
		"admin-1",
	)
	if err != nil {
		t.Fatalf("prepare preference repository: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	updatedPreference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	if err := repository.UpdatePreference(ctx, "admin-1", updatedPreference); err != nil {
		t.Fatalf("update preference: %v", err)
	}

	userRepo := sqliterepo.NewUserRepository(db)
	user, err := userRepo.GetByTelegramID(ctx, "admin-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.Preference != updatedPreference {
		t.Fatalf("unexpected stored preference: %#v", user.Preference)
	}
}

func TestPreparePreferenceRepositoryRejectsBlankAdminTelegramID(t *testing.T) {
	ctx := context.Background()
	_, db, err := preparePreferenceRepository(
		ctx,
		filepath.Join(t.TempDir(), "state", "grimoire.sqlite"),
		"   ",
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if db != nil {
		t.Fatalf("expected nil db on error, got %#v", db)
	}
}

func newTestUserRepository(t *testing.T) *sqliterepo.UserRepository {
	t.Helper()

	ctx := context.Background()
	db, err := sqliterepo.Open(ctx, filepath.Join(t.TempDir(), "state", "grimoire.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := sqliterepo.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	return sqliterepo.NewUserRepository(db)
}
