package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Service struct {
	users UserRepository
}

func NewService(users UserRepository) *Service {
	return &Service{users: users}
}

func (s *Service) Check(ctx context.Context, command CheckCommand) (Decision, error) {
	telegramID := strings.TrimSpace(command.TelegramID)
	if telegramID == "" {
		return Decision{}, fmt.Errorf("telegram id is required")
	}

	user, err := s.users.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, sql.ErrNoRows) {
			return Decision{Allowed: false, Reason: ReasonUserNotFound}, nil
		}
		return Decision{}, err
	}

	if !user.CanAccess() {
		return Decision{Allowed: false, Reason: ReasonUserBanned}, nil
	}

	return Decision{Allowed: true}, nil
}
