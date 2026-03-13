package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	nai "grimoire/internal/adapters/imagegen/nai"
	openai "grimoire/internal/adapters/llm/openai"
	memoryqueue "grimoire/internal/adapters/queue/memory"
	memoryrepo "grimoire/internal/adapters/repository/memory"
	runtimerepo "grimoire/internal/adapters/repository/runtime"
	"grimoire/internal/adapters/telegram"
	drawapp "grimoire/internal/app/draw"
	preferencesapp "grimoire/internal/app/preferences"
	"grimoire/internal/config"
)

const workerConcurrency = 1 // NAI rejects concurrent jobs, so draw tasks must be processed serially.

type App struct {
	bot         *telegram.Bot
	worker      *memoryqueue.Worker
	logger      *slog.Logger
	adminChatID int64
}

func NewApp(cfg config.Config, logger *slog.Logger) (*App, error) {
	taskRepo := memoryrepo.NewTaskRepository()
	preferenceRepo, err := runtimerepo.NewPreferenceRepository(os.Executable)
	if err != nil {
		return nil, fmt.Errorf("init runtime preference repository: %w", err)
	}

	telegramBot := telegram.NewBot(cfg, logger)
	preferenceService := preferencesapp.NewService(preferenceRepo)
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
		nil,
		func() string { return memoryrepo.NewTaskID() },
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
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.worker.Start(ctx)
	a.logger.Info("grimoire v2 started")
	if _, err := a.bot.SendText(ctx, a.adminChatID, 0, "Grimoire v2 已启动"); err != nil && a.logger != nil {
		a.logger.Warn("send startup notification failed", "chat_id", a.adminChatID, "error", err)
	}
	defer a.logger.Info("grimoire v2 stopped")
	return a.bot.Run(ctx)
}
