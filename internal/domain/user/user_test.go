package user

import (
	"testing"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

func TestNewNormalUserCanAccess(t *testing.T) {
	u, err := New("123", RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	if !u.CanAccess() {
		t.Fatal("expected normal user to have access")
	}
}

func TestNewRejectsInvalidRole(t *testing.T) {
	if _, err := New("123", Role("guest"), domainpreferences.DefaultPreference()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsEmptyTelegramID(t *testing.T) {
	if _, err := New("", RoleNormal, domainpreferences.DefaultPreference()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsInvalidPreference(t *testing.T) {
	preference := domainpreferences.DefaultPreference()
	preference.Shape = domaindraw.Shape("invalid")

	if _, err := New("123", RoleNormal, preference); err == nil {
		t.Fatal("expected user creation to reject invalid preference")
	}
}

func TestBannedRoleCannotAccess(t *testing.T) {
	u, err := New("123", RoleBanned, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	if u.CanAccess() {
		t.Fatal("expected banned user to be denied access")
	}
}

func TestSetRoleRejectsInvalidValue(t *testing.T) {
	u, err := New("123", RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	if err := u.SetRole(Role("guest")); err == nil {
		t.Fatal("expected error")
	}
	if u.Role != RoleNormal {
		t.Fatalf("unexpected role after failed update: %q", u.Role)
	}
}

func TestSetRoleUpdatesRole(t *testing.T) {
	u, err := New("123", RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	if err := u.SetRole(RoleBanned); err != nil {
		t.Fatalf("set role: %v", err)
	}
	if u.Role != RoleBanned {
		t.Fatalf("unexpected role: %q", u.Role)
	}
}

func TestSetPreferenceUpdatesPreference(t *testing.T) {
	u, err := New("123", RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	preference, err := domainpreferences.New(domaindraw.ShapeLandscape, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	if err := u.SetPreference(preference); err != nil {
		t.Fatalf("set preference: %v", err)
	}
	if u.Preference != preference {
		t.Fatalf("unexpected preference: %#v", u.Preference)
	}
}

func TestSetPreferenceRejectsInvalidPreference(t *testing.T) {
	u, err := New("123", RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	preference := domainpreferences.DefaultPreference()
	preference.Shape = domaindraw.Shape("invalid")
	if err := u.SetPreference(preference); err == nil {
		t.Fatal("expected error")
	}
	if u.Preference != domainpreferences.DefaultPreference() {
		t.Fatalf("unexpected preference after failed update: %#v", u.Preference)
	}
}
