package preferences

import (
	"fmt"

	"grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Get() (domainpreferences.Preference, error) {
	return s.repository.Get()
}

func (s *Service) UpdateShape(shape draw.Shape) (domainpreferences.Preference, error) {
	if !shape.Valid() {
		return domainpreferences.Preference{}, fmt.Errorf("invalid shape %q", shape)
	}

	preference, err := s.repository.Get()
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.SetShape(shape)
	if err := s.repository.Save(preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) UpdateArtists(artists string) (domainpreferences.Preference, error) {
	preference, err := s.repository.Get()
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.SetArtists(artists)
	if err := s.repository.Save(preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}

func (s *Service) ClearArtists() (domainpreferences.Preference, error) {
	preference, err := s.repository.Get()
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	preference.ClearArtists()
	if err := s.repository.Save(preference); err != nil {
		return domainpreferences.Preference{}, err
	}
	return preference, nil
}
