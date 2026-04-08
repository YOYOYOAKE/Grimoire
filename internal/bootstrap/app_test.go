package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	recoveryapp "grimoire/internal/app/recovery"
)

type workerStarterStub struct {
	order *[]string
	name  string
}

func (s *workerStarterStub) Start(context.Context) {
	*s.order = append(*s.order, s.name)
}

type recoveryExecutorStub struct {
	order  *[]string
	err    error
	result recoveryapp.RecoverResult
}

func (s *recoveryExecutorStub) Recover(context.Context, recoveryapp.RecoverCommand) (recoveryapp.RecoverResult, error) {
	*s.order = append(*s.order, "recover")
	if s.err != nil {
		return recoveryapp.RecoverResult{}, s.err
	}
	return s.result, nil
}

func TestStartBackgroundServicesStartsWorkersBeforeRecovery(t *testing.T) {
	order := []string{}
	app := &App{
		runnerWorker: &workerStarterStub{order: &order, name: "runner-worker"},
		recovery:     &recoveryExecutorStub{order: &order},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		wiring: reservedWiring{
			RecoveryEnabled: true,
		},
	}

	if err := app.startBackgroundServices(context.Background()); err != nil {
		t.Fatalf("start background services: %v", err)
	}
	if len(order) != 2 || order[0] != "runner-worker" || order[1] != "recover" {
		t.Fatalf("unexpected startup order: %#v", order)
	}
}

func TestStartBackgroundServicesSkipsRecoveryWhenDisabled(t *testing.T) {
	order := []string{}
	app := &App{
		runnerWorker: &workerStarterStub{order: &order, name: "runner-worker"},
		recovery:     &recoveryExecutorStub{order: &order},
		logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if err := app.startBackgroundServices(context.Background()); err != nil {
		t.Fatalf("start background services: %v", err)
	}
	if len(order) != 1 || order[0] != "runner-worker" {
		t.Fatalf("unexpected startup order: %#v", order)
	}
}

func TestStartBackgroundServicesReturnsRecoveryError(t *testing.T) {
	app := &App{
		recovery: &recoveryExecutorStub{order: &[]string{}, err: errors.New("boom")},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		wiring: reservedWiring{
			RecoveryEnabled: true,
		},
	}

	if err := app.startBackgroundServices(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
