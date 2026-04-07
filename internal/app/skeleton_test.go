package app_test

import (
	"testing"

	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	conversationapp "grimoire/internal/app/conversation"
	preferencesapp "grimoire/internal/app/preferences"
	recoveryapp "grimoire/internal/app/recovery"
	requestapp "grimoire/internal/app/request"
	runnerapp "grimoire/internal/app/runner"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
)

func TestAppServiceSkeletonsConstruct(t *testing.T) {
	if accessapp.NewService(nil) == nil {
		t.Fatal("expected access service")
	}
	if chatapp.NewService() == nil {
		t.Fatal("expected chat service")
	}
	if sessionapp.NewService(nil, nil, nil) == nil {
		t.Fatal("expected session service")
	}
	if conversationapp.NewService(nil, nil, nil, nil, 15, nil, nil) == nil {
		t.Fatal("expected conversation service")
	}
	if requestapp.NewService(nil) == nil {
		t.Fatal("expected request service")
	}
	if taskapp.NewService(nil, nil, nil) == nil {
		t.Fatal("expected task service")
	}
	if runnerapp.NewService(nil, nil, nil, nil) == nil {
		t.Fatal("expected runner service")
	}
	if recoveryapp.NewService() == nil {
		t.Fatal("expected recovery service")
	}
	if preferencesapp.NewService(nil, "") == nil {
		t.Fatal("expected preferences service")
	}
}
