package access

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type userRepositoryStub struct {
	user          domainuser.User
	err           error
	gotTelegramID string
}

func (s *userRepositoryStub) GetByTelegramID(_ context.Context, telegramID string) (domainuser.User, error) {
	s.gotTelegramID = telegramID
	if s.err != nil {
		return domainuser.User{}, s.err
	}
	return s.user, nil
}

func TestCheckAllowsNormalUser(t *testing.T) {
	user, err := domainuser.New("user-1", domainuser.RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	repository := &userRepositoryStub{user: user}
	service := NewService(repository)

	decision, err := service.Check(context.Background(), CheckCommand{TelegramID: "  user-1  "})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("expected allowed decision, got %#v", decision)
	}
	if decision.Reason != "" {
		t.Fatalf("expected empty reason for allowed user, got %q", decision.Reason)
	}
	if repository.gotTelegramID != "user-1" {
		t.Fatalf("expected trimmed telegram id, got %q", repository.gotTelegramID)
	}
}

func TestCheckDeniesMissingUserFromSQLNoRows(t *testing.T) {
	service := NewService(&userRepositoryStub{err: sql.ErrNoRows})

	decision, err := service.Check(context.Background(), CheckCommand{TelegramID: "user-1"})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision, got %#v", decision)
	}
	if decision.Reason != ReasonUserNotFound {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestCheckDeniesMissingUserFromDomainSentinel(t *testing.T) {
	service := NewService(&userRepositoryStub{err: ErrUserNotFound})

	decision, err := service.Check(context.Background(), CheckCommand{TelegramID: "user-1"})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision, got %#v", decision)
	}
	if decision.Reason != ReasonUserNotFound {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestCheckDeniesBannedUser(t *testing.T) {
	user, err := domainuser.New("user-1", domainuser.RoleBanned, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}

	service := NewService(&userRepositoryStub{user: user})
	decision, err := service.Check(context.Background(), CheckCommand{TelegramID: "user-1"})
	if err != nil {
		t.Fatalf("check access: %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected denied decision, got %#v", decision)
	}
	if decision.Reason != ReasonUserBanned {
		t.Fatalf("unexpected reason: %q", decision.Reason)
	}
}

func TestCheckReturnsRepositoryError(t *testing.T) {
	repositoryErr := errors.New("database unavailable")
	service := NewService(&userRepositoryStub{err: repositoryErr})

	decision, err := service.Check(context.Background(), CheckCommand{TelegramID: "user-1"})
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
	if decision != (Decision{}) {
		t.Fatalf("expected zero decision on repository error, got %#v", decision)
	}
}

func TestCheckRejectsBlankTelegramID(t *testing.T) {
	service := NewService(&userRepositoryStub{})

	_, err := service.Check(context.Background(), CheckCommand{TelegramID: " \t "})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "telegram id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}
