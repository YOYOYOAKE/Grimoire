package preferences

import (
	"context"
	"fmt"
	"strings"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type Service struct {
	repository Repository
	telegramID string
}

func NewService(repository Repository, telegramID string) *Service {
	return &Service{
		repository: repository,
		telegramID: telegramID,
	}
}

func (s *Service) Get(ctx context.Context) (domainpreferences.Preference, error) {
	user, err := s.loadUser(ctx)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	return user.Preference, nil
}

func (s *Service) UpdateShape(ctx context.Context, shape draw.Shape) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := preference.SetShape(shape); err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := s.repository.UpdatePreference(ctx, s.normalizedTelegramID(), preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) UpdateArtists(ctx context.Context, artists string) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.SetArtists(artists)
	if err := s.repository.UpdatePreference(ctx, s.normalizedTelegramID(), preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) ClearArtists(ctx context.Context) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.ClearArtists()
	if err := s.repository.UpdatePreference(ctx, s.normalizedTelegramID(), preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) loadUser(ctx context.Context) (domainuser.User, error) {
	telegramID := s.normalizedTelegramID()
	if telegramID == "" {
		return domainuser.User{}, fmt.Errorf("telegram id is required")
	}

	user, err := s.repository.GetByTelegramID(ctx, telegramID)
	if err != nil {
		return domainuser.User{}, err
	}
	return user, nil
}

func (s *Service) normalizedTelegramID() string {
	return strings.TrimSpace(s.telegramID)
}
