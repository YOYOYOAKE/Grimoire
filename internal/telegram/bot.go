package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/store"
	"grimoire/internal/types"
)

const apiBase = "https://api.telegram.org"

const (
	cbSetLLMBaseURL = "menu:set_llm_base_url"
	cbSetLLMAPIKey  = "menu:set_llm_api_key"
	cbSetLLMModel   = "menu:set_llm_model"
	cbBackLLMMenu   = "menu:back_llm"

	cbLLMModelPickPrefix    = "llm_model_pick:"
	cbLLMModelPagePrefix    = "llm_model_page:"
	cbLLMModelRefreshPrefix = "llm_model_refresh:"
	cbLLMModelManualPrefix  = "llm_model_manual:"

	cbSetNAIAPIKey  = "menu:set_nai_api_key"
	cbSetNAIModel   = "menu:set_nai_model"
	cbSetArtist     = "menu:set_artist"
	cbSetImageSize  = "menu:set_image_size"
	cbBackImageMenu = "menu:back_image"
	cbBackMain      = "menu:back_main" // legacy callback data kept for compatibility.
	cbSizePrefix    = "size:"
	cbStopPrefix    = "stop:"
	cbRegenPrefix   = "regen:"
	cbGalleryPrev   = "gallery_prev:"
	cbGalleryNext   = "gallery_next:"
	cbRetryPrefix   = "retry:"
)

const (
	llmModelPageSize = 10
	llmModelTTL      = 10 * time.Minute
)

type PendingAction int

const (
	pendingNone PendingAction = iota
	pendingSetLLMBaseURL
	pendingSetLLMAPIKey
	pendingSetLLMModel
	pendingSetNAIAPIKey
	pendingSetNAIModel
	pendingSetArtist
)

type TaskQueue interface {
	Enqueue(task types.DrawTask) (taskID string, queuePos int)
	Stats() types.QueueStats
}

type TaskController interface {
	CancelTask(taskID string) bool
}

type Bot struct {
	cfg           *config.Manager
	queue         TaskQueue
	taskStore     store.TaskStore
	taskControl   TaskController
	logger        *slog.Logger
	httpClient    *http.Client
	llmHTTPClient *http.Client
	updateOffset  int64

	pendingMu    sync.Mutex
	pendingInput map[int64]PendingAction

	retryMu   sync.Mutex
	retryTask map[string]types.DrawTask

	llmModelMu       sync.Mutex
	llmModelSessions map[int64]llmModelSession
	llmModelSeq      uint64
}

type llmModelSession struct {
	SessionID string
	Models    []string
	ExpiresAt time.Time
}

func NewBot(cfg *config.Manager, queue TaskQueue, taskStore store.TaskStore, logger *slog.Logger) *Bot {
	snapshot := cfg.Snapshot()
	return &Bot{
		cfg:              cfg,
		queue:            queue,
		taskStore:        taskStore,
		logger:           logger,
		httpClient:       newTelegramHTTPClient(snapshot.Telegram.ProxyURL, snapshot.Telegram.TimeoutSec, logger),
		llmHTTPClient:    newDirectHTTPClient(snapshot.Telegram.TimeoutSec),
		pendingInput:     make(map[int64]PendingAction),
		retryTask:        make(map[string]types.DrawTask),
		llmModelSessions: make(map[int64]llmModelSession),
	}
}

func (b *Bot) SetTaskController(controller TaskController) {
	b.taskControl = controller
}

func (b *Bot) Run(ctx context.Context) error {
	if err := b.setMyCommands(ctx); err != nil {
		b.logger.Warn("setMyCommands failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.getUpdates(ctx)
		if err != nil {
			b.logger.Error("getUpdates failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= b.updateOffset {
				b.updateOffset = upd.UpdateID + 1
			}
			if upd.CallbackQuery != nil {
				b.handleCallbackQuery(ctx, *upd.CallbackQuery)
				continue
			}
			if upd.Message != nil {
				b.handleMessage(ctx, *upd.Message)
			}
		}
	}
}

func (b *Bot) NotifyText(ctx context.Context, chatID int64, text string) (int64, error) {
	return b.sendMessage(ctx, chatID, text)
}

func (b *Bot) EditText(ctx context.Context, chatID int64, messageID int64, text string) error {
	return b.editMessage(ctx, chatID, messageID, text)
}

func (b *Bot) EditPhoto(ctx context.Context, chatID int64, messageID int64, filePath string, caption string) error {
	return b.editMessagePhotoWithMarkup(ctx, chatID, messageID, filePath, caption, b.buildTaskActionMarkup(ctx, chatID, messageID, caption))
}

func (b *Bot) NotifyPhoto(ctx context.Context, chatID int64, filePath string, caption string) error {
	return b.sendPhoto(ctx, chatID, filePath, caption)
}
