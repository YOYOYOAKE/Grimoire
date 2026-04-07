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
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
	"grimoire/internal/platform/httpclient"
)

const apiBase = "https://api.telegram.org"

const (
	cbShapeSmallPortrait  = "img:shape:small-portrait"
	cbShapeSmallLandscape = "img:shape:small-landscape"
	cbShapeSmallSquare    = "img:shape:small-square"
	cbShapePortrait       = "img:shape:portrait"
	cbShapeLandscape      = "img:shape:landscape"
	cbShapeSquare         = "img:shape:square"
	cbShapeLargePortrait  = "img:shape:large-portrait"
	cbShapeLargeLandscape = "img:shape:large-landscape"
	cbShapeLargeSquare    = "img:shape:large-square"
	cbSetArtists          = "img:artists:set"
	cbClearArtists        = "img:artists:clear"
)

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
	_, err := b.sendPhoto(ctx, chatID, replyToMessageID, filename, caption, content)
	return err
}

func (b *Bot) SendPhotoMessage(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) (int64, error) {
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
		b.sendSimpleMessage(ctx, message.Chat.ID, "无权限")
		return
	}

	text := strings.TrimSpace(message.Text)
	command := firstWord(text)
	switch command {
	case "/start":
		b.clearPendingArtists()
		_, _ = b.sendMessage(ctx, message.Chat.ID, buildStartText(), nil, 0)
		return
	case "/img":
		b.clearPendingArtists()
		b.sendImageMenu(ctx, message.Chat.ID, 0, "")
		return
	case "/balance":
		b.clearPendingArtists()
		b.sendBalance(ctx, message.Chat.ID)
		return
	}

	if b.isPendingArtists() {
		if text == "" {
			b.sendSimpleMessage(ctx, message.Chat.ID, "请输入新的画师串，或发送 /start 取消。")
			return
		}
		b.clearPendingArtists()
		if _, err := b.preferenceService.UpdateArtists(ctx, text); err != nil {
			b.logWarn("update artists failed", "chat_id", message.Chat.ID, "error", err)
			b.sendSimpleMessage(ctx, message.Chat.ID, fmt.Sprintf("设置画师串失败: %v", err))
			return
		}
		b.sendImageMenu(ctx, message.Chat.ID, 0, "全局画师串已更新。")
		return
	}

	if text == "" {
		return
	}
	if b.drawService == nil {
		b.logWarn("draw service is not initialized", "chat_id", message.Chat.ID)
		b.sendSimpleMessage(ctx, message.Chat.ID, "绘图服务未初始化")
		return
	}
	if _, err := b.drawService.Submit(ctx, drawapp.SubmitCommand{
		ChatID:           message.Chat.ID,
		Prompt:           text,
		RequestMessageID: message.MessageID,
	}); err != nil {
		b.logWarn("submit draw task failed", "chat_id", message.Chat.ID, "message_id", message.MessageID, "error", err)
		b.sendSimpleMessage(ctx, message.Chat.ID, fmt.Sprintf("创建任务失败: %v", err))
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
		pref domainpreferences.Preference
		err  error
	)

	switch query.Data {
	case cbShapeSmallPortrait:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeSmallPortrait)
	case cbShapeSmallLandscape:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeSmallLandscape)
	case cbShapeSmallSquare:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeSmallSquare)
	case cbShapeSquare:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeSquare)
	case cbShapeLandscape:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeLandscape)
	case cbShapePortrait:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapePortrait)
	case cbShapeLargePortrait:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeLargePortrait)
	case cbShapeLargeLandscape:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeLargeLandscape)
	case cbShapeLargeSquare:
		pref, err = b.preferenceService.UpdateShape(ctx, domaindraw.ShapeLargeSquare)
	case cbSetArtists:
		b.setPendingArtists()
		b.answerCallbackQueryBestEffort(ctx, query.ID, "请发送新的画师串", false)
		b.sendSimpleMessage(ctx, query.Message.Chat.ID, "请发送新的画师串，或发送 /start 取消。")
		return
	case cbClearArtists:
		pref, err = b.preferenceService.ClearArtists(ctx)
	default:
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}

	if err != nil {
		b.logWarn("update image preference failed", "chat_id", query.Message.Chat.ID, "callback_data", query.Data, "error", err)
		b.answerCallbackQueryBestEffort(ctx, query.ID, "设置失败", true)
		return
	}
	b.answerCallbackQueryBestEffort(ctx, query.ID, "已更新", false)
	_ = b.editMessage(ctx, query.Message.Chat.ID, query.Message.MessageID, buildImageMenuText("", pref), imageMenuMarkup())
}

func (b *Bot) sendImageMenu(ctx context.Context, chatID int64, messageID int64, notice string) {
	pref, err := b.preferenceService.Get(ctx)
	if err != nil {
		b.logWarn("load image preference failed", "chat_id", chatID, "error", err)
		b.sendSimpleMessage(ctx, chatID, fmt.Sprintf("加载偏好失败: %v", err))
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

func (b *Bot) setPendingArtists() {
	b.pendingArtistsMu.Lock()
	b.pendingArtists = true
	b.pendingArtistsMu.Unlock()
}

func (b *Bot) clearPendingArtists() {
	b.pendingArtistsMu.Lock()
	b.pendingArtists = false
	b.pendingArtistsMu.Unlock()
}

func (b *Bot) isPendingArtists() bool {
	b.pendingArtistsMu.Lock()
	ok := b.pendingArtists
	b.pendingArtistsMu.Unlock()
	return ok
}

func buildStartText() string {
	return "Grimoire v2\n\n发送任意文本即可开始绘图。\n发送 /img 可修改全局默认图像尺寸和画师串。\n发送 /balance 可查询 NAI 余额。"
}

func buildImageMenuText(notice string, pref domainpreferences.Preference) string {
	text := fmt.Sprintf("全局绘图偏好\n当前尺寸: %s\n当前画师串: %s", pref.Shape.Label(), displayArtist(pref.Artists))
	if strings.TrimSpace(notice) == "" {
		return text
	}
	return notice + "\n\n" + text
}

func imageMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "Small Portrait", CallbackData: cbShapeSmallPortrait},
				{Text: "Small Landscape", CallbackData: cbShapeSmallLandscape},
				{Text: "Small Square", CallbackData: cbShapeSmallSquare},
			},
			{
				{Text: "Normal Portrait", CallbackData: cbShapePortrait},
				{Text: "Normal Landscape", CallbackData: cbShapeLandscape},
				{Text: "Normal Square", CallbackData: cbShapeSquare},
			},
			{
				{Text: "Large Portrait", CallbackData: cbShapeLargePortrait},
				{Text: "Large Landscape", CallbackData: cbShapeLargeLandscape},
				{Text: "Large Square", CallbackData: cbShapeLargeSquare},
			},
			{
				{Text: "设置画师串", CallbackData: cbSetArtists},
				{Text: "清空画师串", CallbackData: cbClearArtists},
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

func (b *Bot) sendBalance(ctx context.Context, chatID int64) {
	if b.balanceService == nil {
		b.logWarn("balance service is not initialized", "chat_id", chatID)
		b.sendSimpleMessage(ctx, chatID, "余额服务未初始化")
		return
	}

	balance, err := b.balanceService.GetBalance(ctx)
	if err != nil {
		b.logWarn("query balance failed", "chat_id", chatID, "error", err)
		b.sendSimpleMessage(ctx, chatID, fmt.Sprintf("查询余额失败: %v", err))
		return
	}

	b.sendSimpleMessage(ctx, chatID, buildBalanceText(balance))
}

func buildBalanceText(balance domainnai.AccountBalance) string {
	subscriptionStatus := "未激活"
	if balance.SubscriptionActive {
		subscriptionStatus = fmt.Sprintf("已激活 (tier=%d)", balance.SubscriptionTier)
	}

	return fmt.Sprintf(
		"NAI 余额\n购买余额: %d\n月度余额: %d\n试用剩余图片: %d\n订阅: %s",
		balance.PurchasedTrainingSteps,
		balance.FixedTrainingStepsLeft,
		balance.TrialImagesLeft,
		subscriptionStatus,
	)
}

func (b *Bot) sendSimpleMessage(ctx context.Context, chatID int64, text string) {
	if _, err := b.sendMessage(ctx, chatID, text, nil, 0); err != nil {
		b.logWarn("send telegram message failed", "chat_id", chatID, "error", err)
	}
}

func (b *Bot) answerCallbackQueryBestEffort(ctx context.Context, callbackID string, text string, showAlert bool) {
	if err := b.answerCallbackQuery(ctx, callbackID, text, showAlert); err != nil {
		b.logWarn("answer callback query failed", "callback_id", callbackID, "error", err)
	}
}

func (b *Bot) logWarn(message string, attrs ...any) {
	if b.logger != nil {
		b.logger.Warn(message, attrs...)
	}
}
