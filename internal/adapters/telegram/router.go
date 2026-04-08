package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	preferencesapp "grimoire/internal/app/preferences"
	taskapp "grimoire/internal/app/task"
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
	b.logInfo(
		"telegram inbound message received",
		"chat_id", message.Chat.ID,
		"telegram_user_id", message.From.ID,
		"message_id", message.MessageID,
		"text", message.Text,
	)
	if !b.authorizeMessage(ctx, message.Chat.ID, message.From.ID) {
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
		b.sendImageMenu(ctx, telegramUserID(message.From.ID), message.Chat.ID, 0, "")
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
		if _, err := b.preferenceService.UpdateArtists(ctx, preferencesapp.UpdateArtistsCommand{
			UserID:  telegramUserID(message.From.ID),
			Artists: text,
		}); err != nil {
			b.logWarn("update artists failed", "chat_id", message.Chat.ID, "error", err)
			b.sendSimpleMessage(ctx, message.Chat.ID, fmt.Sprintf("设置画师串失败: %v", err))
			return
		}
		b.sendImageMenu(ctx, telegramUserID(message.From.ID), message.Chat.ID, 0, "全局画师串已更新。")
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
	reply := strings.TrimSpace(result.Reply)
	if result.CreatedTaskID != "" {
		reply = buildTaskStartedText()
	}
	if reply == "" {
		return
	}
	if _, err := b.sendMessage(ctx, message.Chat.ID, reply, nil, message.MessageID); err != nil {
		b.logWarn("send chat reply failed", "chat_id", message.Chat.ID, "message_id", message.MessageID, "error", err)
		return
	}
	b.logInfo(
		"telegram outbound chat reply sent",
		"chat_id", message.Chat.ID,
		"telegram_user_id", message.From.ID,
		"message_id", message.MessageID,
		"reply", reply,
		"session_id", result.SessionID,
		"created_task_id", result.CreatedTaskID,
	)
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query CallbackQuery) {
	b.logInfo(
		"telegram callback query received",
		"callback_id", query.ID,
		"callback_data", query.Data,
		"telegram_user_id", query.From.ID,
		"chat_id", callbackChatID(query),
		"message_id", callbackMessageID(query),
	)
	if !b.authorizeCallback(ctx, query) {
		return
	}
	if query.Message == nil {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}
	if action, ok := parseTaskAction(query.Data); ok {
		b.handleTaskAction(ctx, query, action)
		return
	}

	action, ok := parseRequestAction(query.Data)
	if !ok {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}

	var (
		pref domainpreferences.Preference
		err  error
	)

	switch action.Kind {
	case requestActionUpdateShape:
		pref, err = b.preferenceService.UpdateShape(ctx, preferencesapp.UpdateShapeCommand{
			UserID: telegramUserID(query.From.ID),
			Shape:  action.Shape,
		})
	case requestActionSetArtists:
		b.setPendingArtists()
		b.answerCallbackQueryBestEffort(ctx, query.ID, "请发送新的画师串", false)
		b.sendSimpleMessage(ctx, query.Message.Chat.ID, buildArtistsPromptText())
		b.logInfo(
			"telegram artists update requested",
			"callback_id", query.ID,
			"callback_data", query.Data,
			"chat_id", query.Message.Chat.ID,
			"telegram_user_id", query.From.ID,
		)
		return
	case requestActionClearArtists:
		pref, err = b.preferenceService.ClearArtists(ctx, preferencesapp.ClearArtistsCommand{
			UserID: telegramUserID(query.From.ID),
		})
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
	b.logInfo(
		"telegram image preference updated",
		"callback_id", query.ID,
		"callback_data", query.Data,
		"chat_id", query.Message.Chat.ID,
		"telegram_user_id", query.From.ID,
		"shape", pref.Shape,
		"artists", pref.Artists,
	)
}

func (b *Bot) sendImageMenu(ctx context.Context, userID string, chatID int64, messageID int64, notice string) {
	pref, err := b.preferenceService.Get(ctx, preferencesapp.GetCommand{UserID: userID})
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
	b.logInfo(
		"telegram image menu sent",
		"chat_id", chatID,
		"user_id", userID,
		"message_id", messageID,
		"text", text,
	)
}

func (b *Bot) handleTaskAction(ctx context.Context, query CallbackQuery, action taskAction) {
	if query.Message == nil {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
		return
	}
	if b.taskService == nil {
		b.logWarn("task service is not initialized", "callback_id", query.ID, "task_id", action.TaskID)
		b.answerCallbackQueryBestEffort(ctx, query.ID, "任务服务未初始化", true)
		return
	}
	userID := telegramUserID(query.From.ID)

	switch action.Kind {
	case taskActionStop:
		if _, err := b.taskService.Stop(ctx, taskapp.StopCommand{TaskID: action.TaskID, UserID: userID}); err != nil {
			b.logWarn("stop task failed", "callback_id", query.ID, "task_id", action.TaskID, "error", err)
			b.answerCallbackQueryBestEffort(ctx, query.ID, "停止任务失败", true)
			return
		}
		b.answerCallbackQueryBestEffort(ctx, query.ID, "已停止任务", false)
		_ = b.editMessage(ctx, query.Message.Chat.ID, query.Message.MessageID, buildStoppedTaskText(), nil)
		b.logInfo("telegram task stopped", "callback_id", query.ID, "task_id", action.TaskID, "user_id", userID)
	case taskActionPrompt:
		prompt, err := b.taskService.GetPrompt(ctx, taskapp.GetPromptCommand{TaskID: action.TaskID, UserID: userID})
		if err != nil {
			b.logWarn("load task prompt failed", "callback_id", query.ID, "task_id", action.TaskID, "error", err)
			b.answerCallbackQueryBestEffort(ctx, query.ID, "查看 prompt 失败", true)
			return
		}
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			b.answerCallbackQueryBestEffort(ctx, query.ID, "当前任务没有 prompt", true)
			return
		}
		b.answerCallbackQueryBestEffort(ctx, query.ID, "已发送 prompt", false)
		if _, err := b.sendMessage(ctx, query.Message.Chat.ID, buildPromptText(prompt), nil, query.Message.MessageID); err != nil {
			b.logWarn("send prompt message failed", "callback_id", query.ID, "task_id", action.TaskID, "error", err)
			return
		}
		b.logInfo("telegram task prompt sent", "callback_id", query.ID, "task_id", action.TaskID, "user_id", userID, "prompt", prompt)
	case taskActionRetryTranslate:
		if _, err := b.taskService.RetryTranslate(ctx, taskapp.RetryCommand{TaskID: action.TaskID, UserID: userID}); err != nil {
			b.logWarn("retry translate task failed", "callback_id", query.ID, "task_id", action.TaskID, "error", err)
			b.answerCallbackQueryBestEffort(ctx, query.ID, "重新翻译失败", true)
			return
		}
		b.answerCallbackQueryBestEffort(ctx, query.ID, "已重新翻译并开始绘图", false)
		b.logInfo("telegram task retry translate requested", "callback_id", query.ID, "task_id", action.TaskID, "user_id", userID)
	case taskActionRetryDraw:
		if _, err := b.taskService.RetryDraw(ctx, taskapp.RetryCommand{TaskID: action.TaskID, UserID: userID}); err != nil {
			b.logWarn("retry draw task failed", "callback_id", query.ID, "task_id", action.TaskID, "error", err)
			b.answerCallbackQueryBestEffort(ctx, query.ID, "重新绘图失败", true)
			return
		}
		b.answerCallbackQueryBestEffort(ctx, query.ID, "已开始重新绘图", false)
		b.logInfo("telegram task retry draw requested", "callback_id", query.ID, "task_id", action.TaskID, "user_id", userID)
	default:
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效", true)
	}
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
	b.logInfo(
		"telegram balance sent",
		"chat_id", chatID,
		"purchased_training_steps", balance.PurchasedTrainingSteps,
		"fixed_training_steps_left", balance.FixedTrainingStepsLeft,
		"trial_images_left", balance.TrialImagesLeft,
		"subscription_active", balance.SubscriptionActive,
		"subscription_tier", balance.SubscriptionTier,
	)
}

func (b *Bot) sendSimpleMessage(ctx context.Context, chatID int64, text string) {
	if _, err := b.sendMessage(ctx, chatID, text, nil, 0); err != nil {
		b.logWarn("send telegram message failed", "chat_id", chatID, "error", err)
		return
	}
	b.logInfo("telegram simple message sent", "chat_id", chatID, "text", text)
}

func (b *Bot) answerCallbackQueryBestEffort(ctx context.Context, callbackID string, text string, showAlert bool) {
	if err := b.answerCallbackQuery(ctx, callbackID, text, showAlert); err != nil {
		b.logWarn("answer callback query failed", "callback_id", callbackID, "error", err)
	}
}

func (b *Bot) authorizeMessage(ctx context.Context, chatID int64, userID int64) bool {
	decision, err := b.checkAccess(ctx, userID)
	if err != nil {
		b.logWarn("check telegram access failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.sendSimpleMessage(ctx, chatID, "访问校验失败")
		return false
	}
	if decision.Allowed {
		return true
	}
	b.sendSimpleMessage(ctx, chatID, "无权限")
	return false
}

func (b *Bot) authorizeCallback(ctx context.Context, query CallbackQuery) bool {
	decision, err := b.checkAccess(ctx, query.From.ID)
	if err != nil {
		b.logWarn("check telegram callback access failed", "callback_id", query.ID, "user_id", query.From.ID, "error", err)
		_ = b.answerCallbackQuery(ctx, query.ID, "访问校验失败", true)
		return false
	}
	if decision.Allowed {
		return true
	}
	_ = b.answerCallbackQuery(ctx, query.ID, "无权限", true)
	return false
}

func (b *Bot) checkAccess(ctx context.Context, userID int64) (accessapp.Decision, error) {
	if b.accessService == nil {
		return accessapp.Decision{}, fmt.Errorf("access service is not initialized")
	}
	return b.accessService.Check(ctx, accessapp.CheckCommand{
		TelegramID: strconv.FormatInt(userID, 10),
	})
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

func telegramUserID(userID int64) string {
	return strconv.FormatInt(userID, 10)
}

func callbackChatID(query CallbackQuery) int64 {
	if query.Message == nil {
		return 0
	}
	return query.Message.Chat.ID
}

func callbackMessageID(query CallbackQuery) int64 {
	if query.Message == nil {
		return 0
	}
	return query.Message.MessageID
}
