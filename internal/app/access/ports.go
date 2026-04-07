package access

import (
	"context"

	domainuser "grimoire/internal/domain/user"
)

type UserRepository interface {
	GetByTelegramID(ctx context.Context, telegramID string) (domainuser.User, error)
}
