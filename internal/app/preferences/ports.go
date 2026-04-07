package preferences

import (
	"context"

	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type Repository interface {
	GetByTelegramID(ctx context.Context, telegramID string) (domainuser.User, error)
	UpdatePreference(ctx context.Context, telegramID string, preference domainpreferences.Preference) error
}
