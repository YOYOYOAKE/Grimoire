package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/llm"
	"grimoire/internal/nai"
	"grimoire/internal/queue"
	"grimoire/internal/service"
	"grimoire/internal/store/sqlite"
	"grimoire/internal/telegram"
	"grimoire/internal/types"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sqlitePath := config.DefaultSQLitePath

	cfgManager, err := config.NewManager(sqlitePath)
	if err != nil {
		logger.Error("加载配置失败", "sqlite_path", sqlitePath, "error", err)
		fmt.Fprintf(os.Stderr, "配置加载失败：%v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := cfgManager.Close(); err != nil {
			logger.Warn("关闭配置数据库失败", "error", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	taskStore, err := sqlite.NewTaskStore(sqlitePath)
	if err != nil {
		logger.Error("初始化 SQLite 失败", "path", sqlitePath, "error", err)
		fmt.Fprintf(os.Stderr, "SQLite 初始化失败：%v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := taskStore.Close(); err != nil {
			logger.Warn("关闭 SQLite 失败", "error", err)
		}
	}()
	if err := taskStore.Init(ctx); err != nil {
		logger.Error("执行 SQLite schema 失败", "error", err)
		fmt.Fprintf(os.Stderr, "SQLite schema 初始化失败：%v\n", err)
		os.Exit(1)
	}

	llmClient := llm.NewOpenAIClient(cfgManager, logger)
	naiClient := nai.NewXianyunClient(cfgManager, logger)

	var orchestrator *service.Orchestrator
	worker := queue.NewWorker(1, func(ctx context.Context, task types.DrawTask) {
		if orchestrator == nil {
			logger.Error("orchestrator 未初始化")
			return
		}
		orchestrator.ProcessTask(ctx, task)
	}, logger)

	bot := telegram.NewBot(cfgManager, worker, taskStore, logger)
	orchestrator = service.NewOrchestrator(llmClient, naiClient, bot, cfgManager, taskStore, logger)
	bot.SetTaskController(orchestrator)

	worker.Start(ctx)
	recoverTasks(ctx, taskStore, worker, logger)
	logger.Info(
		"grimoire bot started",
		"config_path", cfgManager.ConfigPath(),
		"sqlite_path", cfgManager.Path(),
	)

	if err := bot.Run(ctx); err != nil {
		logger.Error("bot 运行失败", "error", err)
		os.Exit(1)
	}

	logger.Info("grimoire bot stopped")
}

func recoverTasks(ctx context.Context, taskStore interface {
	ListRecoverableTasks(ctx context.Context) ([]types.DrawTask, error)
}, worker interface {
	Enqueue(task types.DrawTask) (taskID string, queuePos int)
}, logger *slog.Logger) {
	tasks, err := taskStore.ListRecoverableTasks(ctx)
	if err != nil {
		logger.Error("加载可恢复任务失败", "error", err)
		return
	}
	if len(tasks) == 0 {
		return
	}
	logger.Info("发现可恢复任务", "count", len(tasks))
	for _, task := range tasks {
		if task.CreatedAt.IsZero() {
			task.CreatedAt = time.Now()
		}
		_, queuePos := worker.Enqueue(task)
		logger.Info("恢复任务已入队",
			"task_id", task.TaskID,
			"resume_job_id", task.ResumeJobID,
			"queue_pos", queuePos,
			"chat_id", task.ChatID,
		)
	}
}
