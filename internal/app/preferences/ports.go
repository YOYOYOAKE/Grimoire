package preferences

import (
	"context"
	"errors"

	domainpreferences "grimoire/internal/domain/preferences"
)

var ErrPreferenceNotFound = errors.New("preference not found")

type Repository interface {
	GetByUserID(ctx context.Context, userID int64) (domainpreferences.UserPreference, error)
	Save(ctx context.Context, preference domainpreferences.UserPreference) error
}
