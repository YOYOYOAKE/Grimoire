package preferences

import (
	"context"
	"fmt"
	"strings"

	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) Get(ctx context.Context, command GetCommand) (domainpreferences.Preference, error) {
	user, err := s.loadUser(ctx, command.UserID)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	return user.Preference, nil
}

func (s *Service) UpdateShape(ctx context.Context, command UpdateShapeCommand) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx, GetCommand{UserID: command.UserID})
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := preference.SetShape(command.Shape); err != nil {
		return domainpreferences.Preference{}, err
	}
	userID, err := normalizeUserID(command.UserID)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := s.repository.UpdatePreference(ctx, userID, preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) UpdateArtists(ctx context.Context, command UpdateArtistsCommand) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx, GetCommand{UserID: command.UserID})
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.SetArtists(command.Artists)
	userID, err := normalizeUserID(command.UserID)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := s.repository.UpdatePreference(ctx, userID, preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) ClearArtists(ctx context.Context, command ClearArtistsCommand) (domainpreferences.Preference, error) {
	preference, err := s.Get(ctx, GetCommand{UserID: command.UserID})
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.ClearArtists()
	userID, err := normalizeUserID(command.UserID)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if err := s.repository.UpdatePreference(ctx, userID, preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) loadUser(ctx context.Context, userID string) (domainuser.User, error) {
	normalizedUserID, err := normalizeUserID(userID)
	if err != nil {
		return domainuser.User{}, err
	}
	user, err := s.repository.GetByTelegramID(ctx, normalizedUserID)
	if err != nil {
		return domainuser.User{}, err
	}
	return user, nil
}

func normalizeUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user id is required")
	}
	return userID, nil
}
