package memory

import (
	"context"
	"sync"

	preferencesapp "grimoire/internal/app/preferences"
	domainpreferences "grimoire/internal/domain/preferences"
)

type PreferenceRepository struct {
	mu          sync.RWMutex
	preferences map[int64]domainpreferences.UserPreference
}

func NewPreferenceRepository() *PreferenceRepository {
	return &PreferenceRepository{preferences: make(map[int64]domainpreferences.UserPreference)}
}

func (r *PreferenceRepository) GetByUserID(_ context.Context, userID int64) (domainpreferences.UserPreference, error) {
	r.mu.RLock()
	preference, ok := r.preferences[userID]
	r.mu.RUnlock()
	if !ok {
		return domainpreferences.UserPreference{}, preferencesapp.ErrPreferenceNotFound
	}
	return preference, nil
}

func (r *PreferenceRepository) Save(_ context.Context, preference domainpreferences.UserPreference) error {
	r.mu.Lock()
	r.preferences[preference.UserID] = preference
	r.mu.Unlock()
	return nil
}
