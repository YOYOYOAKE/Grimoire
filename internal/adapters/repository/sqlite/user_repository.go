package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type UserRepository struct {
	db *sql.DB
}

type userRecord struct {
	TelegramID string
	Role       string
	Preference string
}

type preferenceRecord struct {
	Shape   string `json:"shape"`
	Artists string `json:"artists"`
	Mode    string `json:"mode,omitempty"`
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID string) (domainuser.User, error) {
	row := ConnFromContext(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT telegram_id, role, preference FROM users WHERE telegram_id = ?`,
		telegramID,
	)

	var record userRecord
	if err := row.Scan(&record.TelegramID, &record.Role, &record.Preference); err != nil {
		return domainuser.User{}, err
	}

	preference, err := decodePreference(record.Preference)
	if err != nil {
		return domainuser.User{}, err
	}
	return domainuser.New(record.TelegramID, domainuser.Role(record.Role), preference)
}

func (r *UserRepository) Create(ctx context.Context, user domainuser.User) error {
	preferenceJSON, err := encodePreference(user.Preference)
	if err != nil {
		return err
	}

	_, err = ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`INSERT INTO users(telegram_id, role, preference) VALUES (?, ?, ?)`,
		user.TelegramID,
		string(user.Role),
		preferenceJSON,
	)
	if err != nil {
		return fmt.Errorf("insert user %s: %w", user.TelegramID, err)
	}
	return nil
}

func (r *UserRepository) UpdateRole(ctx context.Context, telegramID string, role domainuser.Role) error {
	if !role.Valid() {
		return fmt.Errorf("invalid role %q", role)
	}

	_, err := ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`UPDATE users SET role = ? WHERE telegram_id = ?`,
		string(role),
		telegramID,
	)
	if err != nil {
		return fmt.Errorf("update user role %s: %w", telegramID, err)
	}
	return nil
}

func (r *UserRepository) UpdatePreference(ctx context.Context, telegramID string, preference domainpreferences.Preference) error {
	preferenceJSON, err := encodePreference(preference)
	if err != nil {
		return err
	}

	_, err = ConnFromContext(ctx, r.db).ExecContext(
		ctx,
		`UPDATE users SET preference = ? WHERE telegram_id = ?`,
		preferenceJSON,
		telegramID,
	)
	if err != nil {
		return fmt.Errorf("update user preference %s: %w", telegramID, err)
	}
	return nil
}

func encodePreference(preference domainpreferences.Preference) (string, error) {
	if err := preference.Validate(); err != nil {
		return "", err
	}

	data, err := json.Marshal(preferenceRecord{
		Shape:   string(preference.Shape),
		Artists: preference.Artists,
		Mode:    string(preference.Mode),
	})
	if err != nil {
		return "", fmt.Errorf("encode user preference: %w", err)
	}
	return string(data), nil
}

func decodePreference(raw string) (domainpreferences.Preference, error) {
	var record preferenceRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return domainpreferences.Preference{}, fmt.Errorf("decode user preference: %w", err)
	}
	if strings.TrimSpace(record.Shape) == "" {
		preference := domainpreferences.DefaultPreference()
		preference.SetArtists(record.Artists)
		if mode := strings.TrimSpace(record.Mode); mode != "" {
			if err := preference.SetMode(domainpreferences.Mode(mode)); err != nil {
				return domainpreferences.Preference{}, err
			}
		}
		return preference, nil
	}
	preference, err := domainpreferences.New(domaindraw.Shape(record.Shape), record.Artists)
	if err != nil {
		return domainpreferences.Preference{}, err
	}
	if mode := strings.TrimSpace(record.Mode); mode != "" {
		if err := preference.SetMode(domainpreferences.Mode(mode)); err != nil {
			return domainpreferences.Preference{}, err
		}
	}
	return preference, nil
}
