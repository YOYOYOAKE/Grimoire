package preferences

import (
	"testing"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type preferenceRepoStub struct {
	preference domainpreferences.Preference
	err        error
}

func (s *preferenceRepoStub) Get() (domainpreferences.Preference, error) {
	if s.err != nil {
		return domainpreferences.Preference{}, s.err
	}
	return s.preference, nil
}

func (s *preferenceRepoStub) Save(preference domainpreferences.Preference) error {
	if s.err != nil {
		return s.err
	}
	s.preference = preference
	return nil
}

func TestGetReturnsStoredPreference(t *testing.T) {
	repo := &preferenceRepoStub{preference: domainpreferences.DefaultPreference()}
	service := NewService(repo)

	preference, err := service.Get()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if preference.Shape != draw.ShapeSmallSquare {
		t.Fatalf("unexpected shape: %s", preference.Shape)
	}
}

func TestUpdateArtistsTrimsWhitespace(t *testing.T) {
	repo := &preferenceRepoStub{preference: domainpreferences.DefaultPreference()}
	service := NewService(repo)

	preference, err := service.UpdateArtists(" artist:foo ")
	if err != nil {
		t.Fatalf("update artists: %v", err)
	}
	if preference.Artists != "artist:foo" {
		t.Fatalf("unexpected artists: %q", preference.Artists)
	}
}
