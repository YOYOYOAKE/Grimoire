package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	drawapp "grimoire/internal/app/draw"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
	"grimoire/internal/platform/httpclient"
)

const apiBase = "https://api.telegram.org"

type DrawService interface {
	Submit(ctx context.Context, command drawapp.SubmitCommand) (domaindraw.Task, error)
}

type PreferenceService interface {
	Get(ctx context.Context) (domainpreferences.Preference, error)
	UpdateShape(ctx context.Context, shape domaindraw.Shape) (domainpreferences.Preference, error)
	UpdateArtists(ctx context.Context, artists string) (domainpreferences.Preference, error)
	ClearArtists(ctx context.Context) (domainpreferences.Preference, error)
}

type BalanceService interface {
	GetBalance(ctx context.Context) (domainnai.AccountBalance, error)
}

type Bot struct {
	cfg               config.Config
	logger            *slog.Logger
	httpClient        *http.Client
	updateOffset      int64
	drawService       DrawService
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
		drawService:  nil,
	}
}

func (b *Bot) SetDrawService(service DrawService) {
	b.drawService = service
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

func (b *Bot) EditText(ctx context.Context, chatID int64, messageID int64, text string) error {
	return b.editMessage(ctx, chatID, messageID, text, nil)
}

func (b *Bot) SendPhoto(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) error {
	_, err := b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content)
	return err
}

func (b *Bot) SendPhotoMessage(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) (int64, error) {
	return b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content)
}

func (b *Bot) DeleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	return b.deleteMessage(ctx, chatID, messageID)
}

func (b *Bot) logWarn(message string, attrs ...any) {
	if b.logger != nil {
		b.logger.Warn(message, attrs...)
	}
}
