package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	preferencesapp "grimoire/internal/app/preferences"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

func preparePreferenceRepository(
	ctx context.Context,
	databasePath string,
	adminTelegramID string,
) (preferencesapp.Repository, *sql.DB, error) {
	db, err := sqliterepo.Open(ctx, databasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite preference store: %w", err)
	}

	if err := sqliterepo.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate sqlite preference store: %w", err)
	}

	userRepo := sqliterepo.NewUserRepository(db)
	if err := ensureAdminPreferenceUser(ctx, userRepo, adminTelegramID); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	return userRepo, db, nil
}

func ensureAdminPreferenceUser(
	ctx context.Context,
	userRepo *sqliterepo.UserRepository,
	adminTelegramID string,
) error {
	adminTelegramID = strings.TrimSpace(adminTelegramID)
	if adminTelegramID == "" {
		return fmt.Errorf("admin telegram id is required")
	}

	_, err := userRepo.GetByTelegramID(ctx, adminTelegramID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load preference user %s: %w", adminTelegramID, err)
	}

	seedUser, err := domainuser.New(adminTelegramID, domainuser.RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		return fmt.Errorf("construct preference user %s: %w", adminTelegramID, err)
	}
	if err := userRepo.Create(ctx, seedUser); err != nil {
		return fmt.Errorf("seed preference user %s: %w", adminTelegramID, err)
	}
	return nil
}
