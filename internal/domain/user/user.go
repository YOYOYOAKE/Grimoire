package user

import (
	"fmt"
	"strings"

	domainpreferences "grimoire/internal/domain/preferences"
)

type User struct {
	TelegramID string
	Role       Role
	Preference domainpreferences.Preference
}

func New(telegramID string, role Role, preference domainpreferences.Preference) (User, error) {
	telegramID = strings.TrimSpace(telegramID)
	if telegramID == "" {
		return User{}, fmt.Errorf("telegram id is required")
	}
	if !role.Valid() {
		return User{}, fmt.Errorf("invalid role %q", role)
	}
	if err := preference.Validate(); err != nil {
		return User{}, fmt.Errorf("invalid preference: %w", err)
	}

	return User{
		TelegramID: telegramID,
		Role:       role,
		Preference: preference,
	}, nil
}

func (u User) CanAccess() bool {
	return u.Role.CanAccess()
}

func (u *User) SetRole(role Role) error {
	if !role.Valid() {
		return fmt.Errorf("invalid role %q", role)
	}
	u.Role = role
	return nil
}

func (u *User) SetPreference(preference domainpreferences.Preference) error {
	if err := preference.Validate(); err != nil {
		return fmt.Errorf("invalid preference: %w", err)
	}
	u.Preference = preference
	return nil
}
