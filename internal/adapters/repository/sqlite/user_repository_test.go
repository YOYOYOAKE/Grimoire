package sqlite

import (
	"context"
	"testing"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

func TestUserRepositoryCreateAndGetByTelegramID(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewUserRepository(db)
	user := newTestUser(t, "user-1", domainuser.RoleNormal, domaindraw.ShapePortrait, "artist:foo")

	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := repository.GetByTelegramID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Role != domainuser.RoleNormal {
		t.Fatalf("unexpected role: %s", got.Role)
	}
	if got.Preference.Shape != domaindraw.ShapePortrait {
		t.Fatalf("unexpected shape: %s", got.Preference.Shape)
	}
	if got.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", got.Preference.Artists)
	}
}

func TestUserRepositoryUpdateRole(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewUserRepository(db)
	user := newTestUser(t, "user-1", domainuser.RoleNormal, domaindraw.ShapeSquare, "")

	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repository.UpdateRole(context.Background(), "user-1", domainuser.RoleBanned); err != nil {
		t.Fatalf("update role: %v", err)
	}

	got, err := repository.GetByTelegramID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Role != domainuser.RoleBanned {
		t.Fatalf("unexpected role: %s", got.Role)
	}
}

func TestUserRepositoryUpdatePreference(t *testing.T) {
	db := openMigratedTestDB(t)
	repository := NewUserRepository(db)
	user := newTestUser(t, "user-1", domainuser.RoleNormal, domaindraw.ShapeSquare, "")

	if err := repository.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	preference, err := domainpreferences.New(domaindraw.ShapeLandscape, "artist:bar")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	if err := repository.UpdatePreference(context.Background(), "user-1", preference); err != nil {
		t.Fatalf("update preference: %v", err)
	}

	got, err := repository.GetByTelegramID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Preference.Shape != domaindraw.ShapeLandscape {
		t.Fatalf("unexpected shape: %s", got.Preference.Shape)
	}
	if got.Preference.Artists != "artist:bar" {
		t.Fatalf("unexpected artists: %q", got.Preference.Artists)
	}
}

func newTestUser(t *testing.T, telegramID string, role domainuser.Role, shape domaindraw.Shape, artists string) domainuser.User {
	t.Helper()

	preference, err := domainpreferences.New(shape, artists)
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	user, err := domainuser.New(telegramID, role, preference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	return user
}
