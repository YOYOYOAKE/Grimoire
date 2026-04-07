package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	runtimerepo "grimoire/internal/adapters/repository/runtime"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	preferencesapp "grimoire/internal/app/preferences"
	domainpreferences "grimoire/internal/domain/preferences"
	domainuser "grimoire/internal/domain/user"
)

type mirroredPreferenceRepository struct {
	users  *sqliterepo.UserRepository
	legacy *runtimerepo.PreferenceRepository
}

func newMirroredPreferenceRepository(
	users *sqliterepo.UserRepository,
	legacy *runtimerepo.PreferenceRepository,
) preferencesapp.Repository {
	return &mirroredPreferenceRepository{
		users:  users,
		legacy: legacy,
	}
}

func (r *mirroredPreferenceRepository) GetByTelegramID(ctx context.Context, telegramID string) (domainuser.User, error) {
	return r.users.GetByTelegramID(ctx, telegramID)
}

func (r *mirroredPreferenceRepository) UpdatePreference(
	ctx context.Context,
	telegramID string,
	preference domainpreferences.Preference,
) error {
	if err := r.users.UpdatePreference(ctx, telegramID, preference); err != nil {
		return err
	}
	if err := r.legacy.Save(preference); err != nil {
		return err
	}
	return nil
}

func preparePreferenceRepository(
	ctx context.Context,
	databasePath string,
	runtimeRepo *runtimerepo.PreferenceRepository,
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
	if err := ensureAdminPreferenceMirror(ctx, userRepo, runtimeRepo, adminTelegramID); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	return newMirroredPreferenceRepository(userRepo, runtimeRepo), db, nil
}

func ensureAdminPreferenceMirror(
	ctx context.Context,
	userRepo *sqliterepo.UserRepository,
	runtimeRepo *runtimerepo.PreferenceRepository,
	adminTelegramID string,
) error {
	adminTelegramID = strings.TrimSpace(adminTelegramID)
	if adminTelegramID == "" {
		return fmt.Errorf("admin telegram id is required")
	}

	runtimePreference, err := runtimeRepo.Get()
	if err != nil {
		return fmt.Errorf("load runtime preference: %w", err)
	}

	user, err := userRepo.GetByTelegramID(ctx, adminTelegramID)
	if err == nil {
		if err := runtimeRepo.Save(user.Preference); err != nil {
			return fmt.Errorf("sync runtime preference mirror: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load preference user %s: %w", adminTelegramID, err)
	}

	seedUser, err := domainuser.New(adminTelegramID, domainuser.RoleNormal, runtimePreference)
	if err != nil {
		return fmt.Errorf("construct preference user %s: %w", adminTelegramID, err)
	}
	if err := userRepo.Create(ctx, seedUser); err != nil {
		return fmt.Errorf("seed preference user %s: %w", adminTelegramID, err)
	}
	return nil
}
