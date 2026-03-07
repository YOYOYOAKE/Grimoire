package bootstrap

import (
	"context"
	"log/slog"
	"time"

	xianyun "grimoire/internal/adapters/imagegen/xianyun"
	openai "grimoire/internal/adapters/llm/openai"
	memoryqueue "grimoire/internal/adapters/queue/memory"
	memoryrepo "grimoire/internal/adapters/repository/memory"
	"grimoire/internal/adapters/telegram"
	drawapp "grimoire/internal/app/draw"
	preferencesapp "grimoire/internal/app/preferences"
	"grimoire/internal/config"
)

const workerConcurrency = 3 // Fixed for v2 initial release to keep runtime behavior simple while preserving async processing.

type App struct {
	bot    *telegram.Bot
	worker *memoryqueue.Worker
	logger *slog.Logger
}

func NewApp(cfg config.Config, logger *slog.Logger) (*App, error) {
	taskRepo := memoryrepo.NewTaskRepository()
	preferenceRepo := memoryrepo.NewPreferenceRepository()

	telegramBot := telegram.NewBot(cfg, logger)
	preferenceService := preferencesapp.NewService(preferenceRepo, func() time.Time { return time.Now() })
	drawService := drawapp.NewService(
		taskRepo,
		preferenceRepo,
		openai.NewFailoverClient(cfg.LLMs, logger),
		xianyun.NewClient(cfg, logger),
		telegramBot,
		func() time.Time { return time.Now() },
		func() string { return memoryrepo.NewTaskID() },
		time.Duration(cfg.NAI.PollIntervalSec)*time.Second,
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

	return &App{
		bot:    telegramBot,
		worker: worker,
		logger: logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.worker.Start(ctx)
	a.logger.Info("grimoire v2 started")
	defer a.logger.Info("grimoire v2 stopped")
	return a.bot.Run(ctx)
}
