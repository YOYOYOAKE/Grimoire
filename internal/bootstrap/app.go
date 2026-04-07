package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	nai "grimoire/internal/adapters/imagegen/nai"
	openai "grimoire/internal/adapters/llm/openai"
	memoryqueue "grimoire/internal/adapters/queue/memory"
	memoryrepo "grimoire/internal/adapters/repository/memory"
	runtimerepo "grimoire/internal/adapters/repository/runtime"
	"grimoire/internal/adapters/telegram"
	drawapp "grimoire/internal/app/draw"
	preferencesapp "grimoire/internal/app/preferences"
	"grimoire/internal/config"
	platformclock "grimoire/internal/platform/clock"
	platformid "grimoire/internal/platform/id"
)

const workerConcurrency = 1 // NAI rejects concurrent jobs, so draw tasks must be processed serially.

type App struct {
	bot         *telegram.Bot
	worker      *memoryqueue.Worker
	logger      *slog.Logger
	adminChatID int64
	wiring      reservedWiring
}

func NewApp(cfg config.Config, configPath string, logger *slog.Logger) (*App, error) {
	wiring, err := resolveReservedWiring(cfg, configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve bootstrap wiring: %w", err)
	}

	taskRepo := memoryrepo.NewTaskRepository()
	preferenceRepo, err := runtimerepo.NewPreferenceRepository(configPath)
	if err != nil {
		return nil, fmt.Errorf("init runtime preference repository: %w", err)
	}

	telegramBot := telegram.NewBot(cfg, logger)
	preferenceService := preferencesapp.NewService(preferenceRepo)
	systemClock := platformclock.NewSystemClock()
	idGenerator := platformid.NewUUIDGenerator()
	imageGenerator, err := nai.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init official nai client: %w", err)
	}
	drawService := drawapp.NewService(
		taskRepo,
		preferenceRepo,
		openai.NewFailoverClient(cfg.LLMs, logger),
		imageGenerator,
		telegramBot,
		systemClock.Now,
		idGenerator.NewString,
		logger,
	)

	worker := memoryqueue.NewWorker(workerConcurrency, func(ctx context.Context, taskID string) {
		if err := drawService.Process(ctx, taskID); err != nil {
			logger.Error("process task failed", "task_id", taskID, "error", err)
		}
	}, logger)

	drawService.SetScheduler(worker)
	telegramBot.SetDrawService(drawService)
	telegramBot.SetPreferenceService(preferenceService)
	telegramBot.SetBalanceService(imageGenerator)

	return &App{
		bot:         telegramBot,
		worker:      worker,
		logger:      logger,
		adminChatID: cfg.Telegram.AdminUserID,
		wiring:      wiring,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.worker.Start(ctx)
	a.logger.Info(
		"reserved runtime wiring prepared",
		"database_path", a.wiring.StorageLayout.DatabasePath,
		"image_dir", a.wiring.StorageLayout.ImageDir,
		"recovery_enabled", a.wiring.RecoveryEnabled,
		"conversation_recent_message_limit", a.wiring.ConversationMessageLimit,
	)
	a.logger.Info("grimoire v2 started")
	if _, err := a.bot.SendText(ctx, a.adminChatID, 0, "Grimoire v2 已启动"); err != nil && a.logger != nil {
		a.logger.Warn("send startup notification failed", "chat_id", a.adminChatID, "error", err)
	}
	defer a.logger.Info("grimoire v2 stopped")
	return a.bot.Run(ctx)
}
