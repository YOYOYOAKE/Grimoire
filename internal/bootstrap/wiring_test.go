package bootstrap

import (
	"testing"

	"grimoire/internal/config"
)

func TestResolveReservedWiring(t *testing.T) {
	wiring, err := resolveReservedWiring(config.Config{
		Storage: config.Storage{
			DataDir:    "runtime",
			SQLitePath: "state/grimoire.sqlite",
			ImageDir:   "images/output",
		},
		Conversation: config.Conversation{
			RecentMessageLimit: 24,
		},
		Recovery: config.Recovery{
			Enabled: boolRef(true),
		},
	}, "/tmp/grimoire/config/config.yaml")
	if err != nil {
		t.Fatalf("resolve reserved wiring: %v", err)
	}

	if wiring.StorageLayout.DataDir != "/tmp/grimoire/runtime" {
		t.Fatalf("unexpected data dir: %q", wiring.StorageLayout.DataDir)
	}
	if wiring.StorageLayout.DatabasePath != "/tmp/grimoire/state/grimoire.sqlite" {
		t.Fatalf("unexpected database path: %q", wiring.StorageLayout.DatabasePath)
	}
	if wiring.StorageLayout.ImageDir != "/tmp/grimoire/images/output" {
		t.Fatalf("unexpected image dir: %q", wiring.StorageLayout.ImageDir)
	}
	if !wiring.RecoveryEnabled {
		t.Fatal("expected recovery to be enabled")
	}
	if wiring.ConversationMessageLimit != 24 {
		t.Fatalf("unexpected conversation message limit: %d", wiring.ConversationMessageLimit)
	}
}

func TestResolveReservedWiringRejectsInvalidConfigPath(t *testing.T) {
	if _, err := resolveReservedWiring(config.Config{
		Conversation: config.Conversation{RecentMessageLimit: 15},
	}, ""); err == nil {
		t.Fatal("expected error")
	}
}

func boolRef(value bool) *bool {
	return &value
}
