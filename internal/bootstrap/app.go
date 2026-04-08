package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"

	localstore "grimoire/internal/adapters/filestore/local"
	nai "grimoire/internal/adapters/imagegen/nai"
	openai "grimoire/internal/adapters/llm/openai"
	memoryqueue "grimoire/internal/adapters/queue/memory"
	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	"grimoire/internal/adapters/telegram"
	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	conversationapp "grimoire/internal/app/conversation"
	preferencesapp "grimoire/internal/app/preferences"
	recoveryapp "grimoire/internal/app/recovery"
	runnerapp "grimoire/internal/app/runner"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	"grimoire/internal/config"
	platformclock "grimoire/internal/platform/clock"
	platformid "grimoire/internal/platform/id"
)

const workerConcurrency = 1 // NAI rejects concurrent jobs, so draw tasks must be processed serially.

type workerStarter interface {
	Start(ctx context.Context)
}

type recoveryExecutor interface {
	Recover(ctx context.Context, command recoveryapp.RecoverCommand) (recoveryapp.RecoverResult, error)
}

type App struct {
	bot          *telegram.Bot
	runnerWorker workerStarter
	recovery     recoveryExecutor
	database     *sql.DB
	logger       *slog.Logger
	adminChatID  int64
	wiring       reservedWiring
}

func NewApp(cfg config.Config, configPath string, logger *slog.Logger) (*App, error) {
	wiring, err := resolveReservedWiring(cfg, configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve bootstrap wiring: %w", err)
	}
	if len(cfg.LLMs) == 0 {
		return nil, fmt.Errorf("at least one llm is required")
	}

	adminTelegramID := strconv.FormatInt(cfg.Telegram.AdminUserID, 10)
	preferenceRepo, db, err := preparePreferenceRepository(
		context.Background(),
		wiring.StorageLayout.DatabasePath,
		adminTelegramID,
	)
	if err != nil {
		return nil, fmt.Errorf("init preference repository: %w", err)
	}

	telegramBot := telegram.NewBot(cfg, logger)
	accessService := accessapp.NewService(preferenceRepo)
	preferenceService := preferencesapp.NewService(preferenceRepo)
	systemClock := platformclock.NewSystemClock()
	idGenerator := platformid.NewUUIDGenerator()
	primaryLLM := cfg.LLMs[0]
	conversationClient := openai.NewConversationClient(primaryLLM, logger)
	translateClient := openai.NewTranslateFailoverClient(cfg.LLMs, logger)
	imageGenerator, err := nai.NewClient(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("init official nai client: %w", err)
	}

	sqliteSessionRepo := sqliterepo.NewSessionRepository(db, idGenerator)
	sqliteSessionMessageRepo := sqliterepo.NewSessionMessageRepository(db)
	sqliteTaskRepo := sqliterepo.NewTaskRepository(db)
	sqliteTxRunner := sqliterepo.NewTxRunner(db)
	sessionService := sessionapp.NewService(sqliteSessionRepo, sqliteSessionMessageRepo, sqliteTxRunner)
	conversationService := conversationapp.NewService(
		conversationClient,
		sqliteSessionRepo,
		sqliteSessionMessageRepo,
		sqliteTxRunner,
		wiring.ConversationMessageLimit,
		systemClock.Now,
		idGenerator.NewString,
	)
	imageStore, err := localstore.NewImageStore(wiring.StorageLayout)
	if err != nil {
		return nil, fmt.Errorf("init local image store: %w", err)
	}
	runnerNotifier := newBootstrapRunnerNotifier(telegramBot, wiring.StorageLayout.RootDir)
	runnerService := runnerapp.NewService(
		sqliteTaskRepo,
		sqliteTxRunner,
		translateClient,
		imageGenerator,
		imageStore,
		runnerNotifier,
		systemClock.Now,
	)
	runnerWorker := memoryqueue.NewWorker(workerConcurrency, func(ctx context.Context, taskID string) {
		if err := runnerService.Run(ctx, runnerapp.RunCommand{TaskID: taskID}); err != nil {
			logger.Error("run task failed", "task_id", taskID, "error", err)
		}
	}, logger)
	runnerScheduler := memoryqueue.NewScheduler(runnerWorker)
	recoveryService := recoveryapp.NewService(sqliteTaskRepo, runnerScheduler)
	taskService := taskapp.NewService(
		sqliteTaskRepo,
		sqliteTxRunner,
		runnerScheduler,
		systemClock.Now,
		idGenerator.NewString,
	)
	chatService := chatapp.NewService(preferenceRepo, sessionService, conversationService, taskService)

	telegramBot.SetAccessService(accessService)
	telegramBot.SetChatService(chatService)
	telegramBot.SetTaskService(taskService)
	telegramBot.SetPreferenceService(preferenceService)
	telegramBot.SetBalanceService(imageGenerator)

	return &App{
		bot:          telegramBot,
		runnerWorker: runnerWorker,
		recovery:     recoveryService,
		database:     db,
		logger:       logger,
		adminChatID:  cfg.Telegram.AdminUserID,
		wiring:       wiring,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if a.database == nil {
			return
		}
		if err := a.database.Close(); err != nil && a.logger != nil {
			a.logger.Warn("close sqlite database failed", "error", err)
		}
	}()

	if err := a.startBackgroundServices(ctx); err != nil {
		return err
	}
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

func (a *App) startBackgroundServices(ctx context.Context) error {
	if a.runnerWorker != nil {
		a.runnerWorker.Start(ctx)
	}
	if !a.wiring.RecoveryEnabled || a.recovery == nil {
		return nil
	}

	result, err := a.recovery.Recover(ctx, recoveryapp.RecoverCommand{})
	if err != nil {
		return fmt.Errorf("run recovery: %w", err)
	}
	if a.logger != nil {
		a.logger.Info("recovery completed", "requeued_tasks", result.RequeuedTaskIDs)
	}
	return nil
}
