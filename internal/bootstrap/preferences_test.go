package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	runtimerepo "grimoire/internal/adapters/repository/runtime"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

func TestEnsureAdminPreferenceMirrorSeedsMissingUserFromRuntime(t *testing.T) {
	ctx := context.Background()
	runtimeRepo, userRepo := newTestPreferenceStores(t)

	preference := domainpreferences.DefaultPreference()
	if err := preference.SetShape(domaindraw.ShapePortrait); err != nil {
		t.Fatalf("set shape: %v", err)
	}
	preference.SetArtists("artist:foo")
	if err := runtimeRepo.Save(preference); err != nil {
		t.Fatalf("save runtime preference: %v", err)
	}

	if err := ensureAdminPreferenceMirror(ctx, userRepo, runtimeRepo, "admin-1"); err != nil {
		t.Fatalf("ensure mirror: %v", err)
	}

	user, err := userRepo.GetByTelegramID(ctx, "admin-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.Preference.Shape != domaindraw.ShapePortrait {
		t.Fatalf("unexpected shape: %s", user.Preference.Shape)
	}
	if user.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", user.Preference.Artists)
	}
}

func TestEnsureAdminPreferenceMirrorSyncsSQLitePreferenceToRuntime(t *testing.T) {
	ctx := context.Background()
	runtimeRepo, userRepo := newTestPreferenceStores(t)

	runtimePreference := domainpreferences.DefaultPreference()
	if err := runtimePreference.SetShape(domaindraw.ShapeSquare); err != nil {
		t.Fatalf("set runtime shape: %v", err)
	}
	if err := runtimeRepo.Save(runtimePreference); err != nil {
		t.Fatalf("save runtime preference: %v", err)
	}

	userPreference, err := domainpreferences.New(domaindraw.ShapeLandscape, "artist:bar")
	if err != nil {
		t.Fatalf("new user preference: %v", err)
	}
	user, err := domainuser.New("admin-1", domainuser.RoleNormal, userPreference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := ensureAdminPreferenceMirror(ctx, userRepo, runtimeRepo, "admin-1"); err != nil {
		t.Fatalf("ensure mirror: %v", err)
	}

	mirrored, err := runtimeRepo.Get()
	if err != nil {
		t.Fatalf("get runtime preference: %v", err)
	}
	if mirrored.Shape != domaindraw.ShapeLandscape {
		t.Fatalf("unexpected shape: %s", mirrored.Shape)
	}
	if mirrored.Artists != "artist:bar" {
		t.Fatalf("unexpected artists: %q", mirrored.Artists)
	}
}

func TestMirroredPreferenceRepositoryUpdatesSQLiteAndRuntime(t *testing.T) {
	ctx := context.Background()
	runtimeRepo, userRepo := newTestPreferenceStores(t)

	user, err := domainuser.New("admin-1", domainuser.RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	repository := newMirroredPreferenceRepository(userRepo, runtimeRepo)
	updatedPreference, err := domainpreferences.New(domaindraw.ShapeLandscape, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	if err := repository.UpdatePreference(ctx, "admin-1", updatedPreference); err != nil {
		t.Fatalf("update preference: %v", err)
	}

	storedUser, err := userRepo.GetByTelegramID(ctx, "admin-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if storedUser.Preference.Shape != domaindraw.ShapeLandscape {
		t.Fatalf("unexpected user shape: %s", storedUser.Preference.Shape)
	}
	if storedUser.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected user artists: %q", storedUser.Preference.Artists)
	}

	runtimePreference, err := runtimeRepo.Get()
	if err != nil {
		t.Fatalf("get runtime preference: %v", err)
	}
	if runtimePreference.Shape != domaindraw.ShapeLandscape {
		t.Fatalf("unexpected runtime shape: %s", runtimePreference.Shape)
	}
	if runtimePreference.Artists != "artist:foo" {
		t.Fatalf("unexpected runtime artists: %q", runtimePreference.Artists)
	}
}

func newTestPreferenceStores(t *testing.T) (*runtimerepo.PreferenceRepository, *sqliterepo.UserRepository) {
	t.Helper()

	ctx := context.Background()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config", "config.yaml")
	runtimeRepo, err := runtimerepo.NewPreferenceRepository(configPath)
	if err != nil {
		t.Fatalf("new runtime repository: %v", err)
	}

	db, err := sqliterepo.Open(ctx, filepath.Join(dir, "state", "grimoire.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := sqliterepo.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	return runtimeRepo, sqliterepo.NewUserRepository(db)
}
