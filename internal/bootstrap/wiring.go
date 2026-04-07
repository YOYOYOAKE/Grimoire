package bootstrap

import (
	"fmt"

	"grimoire/internal/config"
	platformdb "grimoire/internal/platform/db"
)

type reservedWiring struct {
	StorageLayout            platformdb.SQLiteLayout
	RecoveryEnabled          bool
	ConversationMessageLimit int
}

func resolveReservedWiring(cfg config.Config, configPath string) (reservedWiring, error) {
	storageLayout, err := cfg.ResolveSQLiteLayout(configPath)
	if err != nil {
		return reservedWiring{}, fmt.Errorf("resolve sqlite layout: %w", err)
	}

	return reservedWiring{
		StorageLayout:            storageLayout,
		RecoveryEnabled:          cfg.Recovery.EnabledValue(),
		ConversationMessageLimit: cfg.Conversation.RecentMessageLimit,
	}, nil
}
