package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	preferencesapp "grimoire/internal/app/preferences"
	taskapp "grimoire/internal/app/task"
	"grimoire/internal/config"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
	"grimoire/internal/platform/httpclient"
)

const apiBase = "https://api.telegram.org"

type ChatService interface {
	HandleText(ctx context.Context, command chatapp.HandleTextCommand) (chatapp.HandleTextResult, error)
}

type AccessService interface {
	Check(ctx context.Context, command accessapp.CheckCommand) (accessapp.Decision, error)
}

type TaskService interface {
	Create(ctx context.Context, command taskapp.CreateCommand) (domaintask.Task, error)
	Stop(ctx context.Context, command taskapp.StopCommand) (domaintask.Task, error)
	GetPrompt(ctx context.Context, command taskapp.GetPromptCommand) (string, error)
	RetryTranslate(ctx context.Context, command taskapp.RetryCommand) (domaintask.Task, error)
	RetryDraw(ctx context.Context, command taskapp.RetryCommand) (domaintask.Task, error)
}

type PreferenceService interface {
	Get(ctx context.Context, command preferencesapp.GetCommand) (domainpreferences.Preference, error)
	UpdateShape(ctx context.Context, command preferencesapp.UpdateShapeCommand) (domainpreferences.Preference, error)
	UpdateArtists(ctx context.Context, command preferencesapp.UpdateArtistsCommand) (domainpreferences.Preference, error)
	ClearArtists(ctx context.Context, command preferencesapp.ClearArtistsCommand) (domainpreferences.Preference, error)
}

type BalanceService interface {
	GetBalance(ctx context.Context) (domainnai.AccountBalance, error)
}

type Bot struct {
	cfg               config.Config
	logger            *slog.Logger
	httpClient        *http.Client
	updateOffset      int64
	accessService     AccessService
	chatService       ChatService
	taskService       TaskService
	preferenceService PreferenceService
	balanceService    BalanceService

	pendingArtistsMu sync.Mutex
	pendingArtists   bool
}

func NewBot(cfg config.Config, logger *slog.Logger) *Bot {
	return &Bot{
		cfg:          cfg,
		logger:       logger,
		httpClient:   httpclient.New(cfg.Telegram.TimeoutSec, cfg.Telegram.Proxy, logger, "telegram"),
		updateOffset: 0,
	}
}

func (b *Bot) SetChatService(service ChatService) {
	b.chatService = service
}

func (b *Bot) SetAccessService(service AccessService) {
	b.accessService = service
}

func (b *Bot) SetTaskService(service TaskService) {
	b.taskService = service
}

func (b *Bot) SetPreferenceService(service PreferenceService) {
	b.preferenceService = service
}

func (b *Bot) SetBalanceService(service BalanceService) {
	b.balanceService = service
}

func (b *Bot) Run(ctx context.Context) error {
	if err := b.setMyCommands(ctx); err != nil && b.logger != nil {
		b.logger.Warn("set telegram commands failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := b.getUpdates(ctx)
		if err != nil {
			if b.logger != nil {
				b.logger.Error("get telegram updates failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= b.updateOffset {
				b.updateOffset = update.UpdateID + 1
			}
			b.routeUpdate(ctx, update)
		}
	}
}

func (b *Bot) SendText(ctx context.Context, chatID int64, replyToMessageID int64, text string) (int64, error) {
	return b.sendMessage(ctx, chatID, text, nil, replyToMessageID)
}

func (b *Bot) SendProgressText(ctx context.Context, chatID int64, replyToMessageID int64, text string, taskID string) (int64, error) {
	return b.sendMessage(ctx, chatID, text, taskProgressMarkup(taskID), replyToMessageID)
}

func (b *Bot) EditText(ctx context.Context, chatID int64, messageID int64, text string) error {
	return b.editMessage(ctx, chatID, messageID, text, nil)
}

func (b *Bot) EditProgressText(ctx context.Context, chatID int64, messageID int64, text string, taskID string) error {
	return b.editMessage(ctx, chatID, messageID, text, taskProgressMarkup(taskID))
}

func (b *Bot) SendPhoto(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) error {
	_, err := b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content, nil)
	return err
}

func (b *Bot) SendPhotoMessage(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) (int64, error) {
	return b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content, nil)
}

func (b *Bot) SendResultPhotoMessage(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte, taskID string) (int64, error) {
	return b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content, resultTaskMarkup(taskID))
}

func (b *Bot) DeleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	return b.deleteMessage(ctx, chatID, messageID)
}

func (b *Bot) logWarn(message string, attrs ...any) {
	if b.logger != nil {
		b.logger.Warn(message, attrs...)
	}
}
