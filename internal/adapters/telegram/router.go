package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	chatapp "grimoire/internal/app/chat"
	domainpreferences "grimoire/internal/domain/preferences"
)

func (b *Bot) routeUpdate(ctx context.Context, update Update) {
	if update.CallbackQuery != nil {
		b.handleCallbackQuery(ctx, *update.CallbackQuery)
		return
	}
	if update.Message != nil {
		b.handleMessage(ctx, *update.Message)
	}
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
	switch firstWord(text) {
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
			b.sendSimpleMessage(ctx, message.Chat.ID, buildArtistsPromptText())
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
	if b.chatService == nil {
		b.logWarn("chat service is not initialized", "chat_id", message.Chat.ID)
		b.sendSimpleMessage(ctx, message.Chat.ID, "聊天服务未初始化")
		return
	}
	result, err := b.chatService.HandleText(ctx, chatapp.HandleTextCommand{
		UserID:    strconv.FormatInt(message.From.ID, 10),
		MessageID: strconv.FormatInt(message.MessageID, 10),
		Text:      text,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		b.logWarn("handle chat message failed", "chat_id", message.Chat.ID, "message_id", message.MessageID, "error", err)
		b.sendSimpleMessage(ctx, message.Chat.ID, fmt.Sprintf("处理消息失败: %v", err))
		return
	}
	if _, err := b.sendMessage(ctx, message.Chat.ID, result.Reply, nil, message.MessageID); err != nil {
		b.logWarn("send chat reply failed", "chat_id", message.Chat.ID, "message_id", message.MessageID, "error", err)
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

	action, ok := parseCallbackAction(query.Data)
	if !ok {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}

	var (
		pref domainpreferences.Preference
		err  error
	)

	switch action.Kind {
	case callbackActionUpdateShape:
		pref, err = b.preferenceService.UpdateShape(ctx, action.Shape)
	case callbackActionSetArtists:
		b.setPendingArtists()
		b.answerCallbackQueryBestEffort(ctx, query.ID, "请发送新的画师串", false)
		b.sendSimpleMessage(ctx, query.Message.Chat.ID, buildArtistsPromptText())
		return
	case callbackActionClearArtists:
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
