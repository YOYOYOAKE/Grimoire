package preferences

import (
	"context"
	"testing"
	"time"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type preferenceRepoStub struct {
	preferences map[int64]domainpreferences.UserPreference
}

func (s *preferenceRepoStub) GetByUserID(_ context.Context, userID int64) (domainpreferences.UserPreference, error) {
	pref, ok := s.preferences[userID]
	if !ok {
		return domainpreferences.UserPreference{}, ErrPreferenceNotFound
	}
	return pref, nil
}

func (s *preferenceRepoStub) Save(_ context.Context, preference domainpreferences.UserPreference) error {
	s.preferences[preference.UserID] = preference
	return nil
}

func TestGetOrCreateCreatesDefaultPreference(t *testing.T) {
	repo := &preferenceRepoStub{preferences: map[int64]domainpreferences.UserPreference{}}
	service := NewService(repo, func() time.Time { return time.Unix(100, 0) })

	pref, err := service.GetOrCreate(context.Background(), 42)
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if pref.DefaultShape != draw.ShapeSquare {
		t.Fatalf("unexpected shape: %s", pref.DefaultShape)
	}
}

func TestUpdateArtist(t *testing.T) {
	repo := &preferenceRepoStub{preferences: map[int64]domainpreferences.UserPreference{}}
	service := NewService(repo, time.Now)

	pref, err := service.UpdateArtist(context.Background(), 42, " artist:foo ")
	if err != nil {
		t.Fatalf("update artist: %v", err)
	}
	if pref.Artist != "artist:foo" {
		t.Fatalf("unexpected artist: %q", pref.Artist)
	}
}
