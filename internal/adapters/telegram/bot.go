package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	drawapp "grimoire/internal/app/draw"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	"grimoire/internal/platform/httpclient"
)

const apiBase = "https://api.telegram.org"

const (
	cbShapeSquare    = "img:shape:square"
	cbShapeLandscape = "img:shape:landscape"
	cbShapePortrait  = "img:shape:portrait"
	cbSetArtist      = "img:artist:set"
	cbClearArtist    = "img:artist:clear"
)

type DrawService interface {
	Submit(ctx context.Context, command drawapp.SubmitCommand) (domaindraw.Task, error)
}

type PreferenceService interface {
	GetOrCreate(ctx context.Context, userID int64) (domainpreferences.UserPreference, error)
	UpdateShape(ctx context.Context, userID int64, shape domaindraw.Shape) (domainpreferences.UserPreference, error)
	UpdateArtist(ctx context.Context, userID int64, artist string) (domainpreferences.UserPreference, error)
	ClearArtist(ctx context.Context, userID int64) (domainpreferences.UserPreference, error)
}

type Bot struct {
	cfg               config.Config
	logger            *slog.Logger
	httpClient        *http.Client
	updateOffset      int64
	drawService       DrawService
	preferenceService PreferenceService

	pendingArtistMu sync.Mutex
	pendingArtist   map[int64]struct{}
}

func NewBot(cfg config.Config, logger *slog.Logger) *Bot {
	return &Bot{
		cfg:           cfg,
		logger:        logger,
		httpClient:    httpclient.New(cfg.Telegram.TimeoutSec, cfg.Telegram.Proxy, logger, "telegram"),
		pendingArtist: make(map[int64]struct{}),
		updateOffset:  0,
		drawService:   nil,
	}
}

func (b *Bot) SetDrawService(service DrawService) {
	b.drawService = service
}

func (b *Bot) SetPreferenceService(service PreferenceService) {
	b.preferenceService = service
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
			if update.CallbackQuery != nil {
				b.handleCallbackQuery(ctx, *update.CallbackQuery)
				continue
			}
			if update.Message != nil {
				b.handleMessage(ctx, *update.Message)
			}
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
	return b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content)
}

func (b *Bot) DeleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	return b.deleteMessage(ctx, chatID, messageID)
}

func (b *Bot) handleMessage(ctx context.Context, message Message) {
	if message.From == nil {
		return
	}
	if !b.isAdmin(message.From.ID) {
		_, _ = b.sendMessage(ctx, message.Chat.ID, "无权限", nil, 0)
		return
	}

	text := strings.TrimSpace(message.Text)
	command := firstWord(text)
	switch command {
	case "/start":
		b.clearPendingArtist(message.From.ID)
		_, _ = b.sendMessage(ctx, message.Chat.ID, buildStartText(), nil, 0)
		return
	case "/img":
		b.clearPendingArtist(message.From.ID)
		b.sendImageMenu(ctx, message.Chat.ID, message.From.ID, 0, "")
		return
	}

	if b.isPendingArtist(message.From.ID) {
		if text == "" {
			_, _ = b.sendMessage(ctx, message.Chat.ID, "请输入新的画师串，或发送 /start 取消。", nil, 0)
			return
		}
		b.clearPendingArtist(message.From.ID)
		if _, err := b.preferenceService.UpdateArtist(ctx, message.From.ID, text); err != nil {
			_, _ = b.sendMessage(ctx, message.Chat.ID, fmt.Sprintf("设置画师串失败: %v", err), nil, 0)
			return
		}
		b.sendImageMenu(ctx, message.Chat.ID, message.From.ID, 0, "画师串已更新。")
		return
	}

	if text == "" {
		return
	}
	if b.drawService == nil {
		_, _ = b.sendMessage(ctx, message.Chat.ID, "绘图服务未初始化", nil, 0)
		return
	}
	if _, err := b.drawService.Submit(ctx, drawapp.SubmitCommand{
		ChatID:           message.Chat.ID,
		UserID:           message.From.ID,
		Prompt:           text,
		RequestMessageID: message.MessageID,
	}); err != nil {
		_, _ = b.sendMessage(ctx, message.Chat.ID, fmt.Sprintf("创建任务失败: %v", err), nil, 0)
	}
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query CallbackQuery) {
	if !b.isAdmin(query.From.ID) {
		_ = b.answerCallbackQuery(ctx, query.ID, "无权限", true)
		return
	}
	if query.Message == nil {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}

	var (
		pref domainpreferences.UserPreference
		err  error
	)

	switch query.Data {
	case cbShapeSquare:
		pref, err = b.preferenceService.UpdateShape(ctx, query.From.ID, domaindraw.ShapeSquare)
	case cbShapeLandscape:
		pref, err = b.preferenceService.UpdateShape(ctx, query.From.ID, domaindraw.ShapeLandscape)
	case cbShapePortrait:
		pref, err = b.preferenceService.UpdateShape(ctx, query.From.ID, domaindraw.ShapePortrait)
	case cbSetArtist:
		b.setPendingArtist(query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的画师串", false)
		_, _ = b.sendMessage(ctx, query.Message.Chat.ID, "请发送新的画师串，或发送 /start 取消。", nil, 0)
		return
	case cbClearArtist:
		pref, err = b.preferenceService.ClearArtist(ctx, query.From.ID)
	default:
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}

	if err != nil {
		_ = b.answerCallbackQuery(ctx, query.ID, "设置失败", true)
		return
	}
	_ = b.answerCallbackQuery(ctx, query.ID, "已更新", false)
	_ = b.editMessage(ctx, query.Message.Chat.ID, query.Message.MessageID, buildImageMenuText("", pref), imageMenuMarkup())
}

func (b *Bot) sendImageMenu(ctx context.Context, chatID int64, userID int64, messageID int64, notice string) {
	pref, err := b.preferenceService.GetOrCreate(ctx, userID)
	if err != nil {
		_, _ = b.sendMessage(ctx, chatID, fmt.Sprintf("加载偏好失败: %v", err), nil, 0)
		return
	}
	text := buildImageMenuText(notice, pref)
	if messageID > 0 {
		if err := b.editMessage(ctx, chatID, messageID, text, imageMenuMarkup()); err == nil {
			return
		}
	}
	_, _ = b.sendMessage(ctx, chatID, text, imageMenuMarkup(), 0)
}

func (b *Bot) isAdmin(userID int64) bool {
	return b.cfg.Telegram.AdminUserID == userID
}

func (b *Bot) setPendingArtist(userID int64) {
	b.pendingArtistMu.Lock()
	b.pendingArtist[userID] = struct{}{}
	b.pendingArtistMu.Unlock()
}

func (b *Bot) clearPendingArtist(userID int64) {
	b.pendingArtistMu.Lock()
	delete(b.pendingArtist, userID)
	b.pendingArtistMu.Unlock()
}

func (b *Bot) isPendingArtist(userID int64) bool {
	b.pendingArtistMu.Lock()
	_, ok := b.pendingArtist[userID]
	b.pendingArtistMu.Unlock()
	return ok
}

func buildStartText() string {
	return "Grimoire v2\n\n发送任意文本即可开始绘图。\n发送 /img 可修改默认图像尺寸和画师串。"
}

func buildImageMenuText(notice string, pref domainpreferences.UserPreference) string {
	text := fmt.Sprintf("绘图偏好\n当前尺寸: %s\n当前画师串: %s", pref.DefaultShape, displayArtist(pref.Artist))
	if strings.TrimSpace(notice) == "" {
		return text
	}
	return notice + "\n\n" + text
}

func imageMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "方形", CallbackData: cbShapeSquare},
				{Text: "横向", CallbackData: cbShapeLandscape},
				{Text: "纵向", CallbackData: cbShapePortrait},
			},
			{
				{Text: "设置画师串", CallbackData: cbSetArtist},
				{Text: "清空画师串", CallbackData: cbClearArtist},
			},
		},
	}
}

func displayArtist(artist string) string {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return "未设置"
	}
	return artist
}

func firstWord(text string) string {
	if text == "" {
		return ""
	}
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}
