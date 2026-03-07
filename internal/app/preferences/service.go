package preferences

import (
	"context"
	"errors"
	"fmt"
	"time"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

func (s *Service) GetOrCreate(ctx context.Context, userID int64) (domainpreferences.UserPreference, error) {
	pref, err := s.repository.GetByUserID(ctx, userID)
	if err == nil {
		return pref, nil
	}
	if !errors.Is(err, ErrPreferenceNotFound) {
		return domainpreferences.UserPreference{}, err
	}

	pref = domainpreferences.NewUserPreference(userID, s.now())
	if err := s.repository.Save(ctx, pref); err != nil {
		return domainpreferences.UserPreference{}, err
	}
	return pref, nil
}

func (s *Service) UpdateShape(ctx context.Context, userID int64, shape draw.Shape) (domainpreferences.UserPreference, error) {
	if !shape.Valid() {
		return domainpreferences.UserPreference{}, fmt.Errorf("invalid shape %q", shape)
	}
	pref, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return domainpreferences.UserPreference{}, err
	}
	pref.SetShape(shape, s.now())
	if err := s.repository.Save(ctx, pref); err != nil {
		return domainpreferences.UserPreference{}, err
	}
	return pref, nil
}

func (s *Service) UpdateArtist(ctx context.Context, userID int64, artist string) (domainpreferences.UserPreference, error) {
	pref, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return domainpreferences.UserPreference{}, err
	}
	pref.SetArtist(artist, s.now())
	if err := s.repository.Save(ctx, pref); err != nil {
		return domainpreferences.UserPreference{}, err
	}
	return pref, nil
}

func (s *Service) ClearArtist(ctx context.Context, userID int64) (domainpreferences.UserPreference, error) {
	pref, err := s.GetOrCreate(ctx, userID)
	if err != nil {
		return domainpreferences.UserPreference{}, err
	}
	pref.ClearArtist(s.now())
	if err := s.repository.Save(ctx, pref); err != nil {
		return domainpreferences.UserPreference{}, err
	}
	return pref, nil
}
