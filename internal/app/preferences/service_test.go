package preferences

import (
	"context"
	"errors"
	"testing"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type preferenceRepoStub struct {
	preference domainpreferences.Preference
	err        error
	telegramID string
}

func (s *preferenceRepoStub) GetByTelegramID(_ context.Context, telegramID string) (domainuser.User, error) {
	s.telegramID = telegramID
	if s.err != nil {
		return domainuser.User{}, s.err
	}
	return domainuser.New(telegramID, domainuser.RoleNormal, s.preference)
}

func (s *preferenceRepoStub) UpdatePreference(_ context.Context, telegramID string, preference domainpreferences.Preference) error {
	s.telegramID = telegramID
	if s.err != nil {
		return s.err
	}
	s.preference = preference
	return nil
}

func TestGetReturnsStoredPreference(t *testing.T) {
	repo := &preferenceRepoStub{preference: domainpreferences.DefaultPreference()}
	service := NewService(repo, " user-1 ")

	preference, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if preference.Shape != draw.ShapeSmallSquare {
		t.Fatalf("unexpected shape: %s", preference.Shape)
	}
	if repo.telegramID != "user-1" {
		t.Fatalf("expected trimmed telegram id, got %q", repo.telegramID)
	}
}

func TestUpdateArtistsTrimsWhitespace(t *testing.T) {
	repo := &preferenceRepoStub{preference: domainpreferences.DefaultPreference()}
	service := NewService(repo, "user-1")

	preference, err := service.UpdateArtists(context.Background(), " artist:foo ")
	if err != nil {
		t.Fatalf("update artists: %v", err)
	}
	if preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", preference.Artists)
	}
}

func TestUpdateShapeRejectsInvalidShape(t *testing.T) {
	repo := &preferenceRepoStub{preference: domainpreferences.DefaultPreference()}
	service := NewService(repo, "user-1")

	if _, err := service.UpdateShape(context.Background(), draw.Shape("invalid")); err == nil {
		t.Fatal("expected error")
	}
}

func TestClearArtistsPersistsEmptyArtists(t *testing.T) {
	preference := domainpreferences.DefaultPreference()
	preference.SetArtists("artist:foo")
	repo := &preferenceRepoStub{preference: preference}
	service := NewService(repo, "user-1")

	updated, err := service.ClearArtists(context.Background())
	if err != nil {
		t.Fatalf("clear artists: %v", err)
	}
	if updated.Artists != "" {
		t.Fatalf("expected empty artists, got %q", updated.Artists)
	}
}

func TestGetRejectsBlankTelegramID(t *testing.T) {
	service := NewService(&preferenceRepoStub{}, " \t ")

	_, err := service.Get(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "telegram id is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateShapeReturnsRepositoryError(t *testing.T) {
	repositoryErr := errors.New("db unavailable")
	repo := &preferenceRepoStub{
		preference: domainpreferences.DefaultPreference(),
		err:        repositoryErr,
	}
	service := NewService(repo, "user-1")

	_, err := service.UpdateShape(context.Background(), draw.ShapePortrait)
	if !errors.Is(err, repositoryErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}
