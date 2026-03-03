package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	cbSetArtist     = "menu:set_artist"
	cbSetImageSize  = "menu:set_image_size"
	cbBackMain      = "menu:back_main"
	cbSizePrefix    = "size:"
	cbRegenPrefix   = "regen:"
	cbGalleryPrev   = "gallery_prev:"
	cbGalleryNext   = "gallery_next:"
	cbRetryPrefix   = "retry:"
)

type PendingAction int

const (
	pendingNone PendingAction = iota
	pendingSetLLMBaseURL
	pendingSetLLMAPIKey
	pendingSetArtist
)

type TaskQueue interface {
	Enqueue(task types.DrawTask) (taskID string, queuePos int)
	Stats() types.QueueStats
}

type Bot struct {
	cfg          *config.Manager
	queue        TaskQueue
	taskStore    store.TaskStore
	logger       *slog.Logger
	httpClient   *http.Client
	updateOffset int64

	pendingMu    sync.Mutex
	pendingInput map[int64]PendingAction

	retryMu   sync.Mutex
	retryTask map[string]types.DrawTask
}

func NewBot(cfg *config.Manager, queue TaskQueue, taskStore store.TaskStore, logger *slog.Logger) *Bot {
	snapshot := cfg.Snapshot()
	return &Bot{
		cfg:          cfg,
		queue:        queue,
		taskStore:    taskStore,
		logger:       logger,
		httpClient:   newTelegramHTTPClient(snapshot.Telegram.ProxyURL, logger),
		pendingInput: make(map[int64]PendingAction),
		retryTask:    make(map[string]types.DrawTask),
	}
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

func (b *Bot) handleMessage(ctx context.Context, msg Message) {
	if msg.From == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)
	b.logger.Info("telegram message received",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID,
		"message_id", msg.MessageID,
		"text", truncate(text, 300),
	)

	if !isAdminUser(b.cfg.Snapshot().Telegram.AdminUserID, msg.From.ID) {
		b.logger.Warn("telegram unauthorized message",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID,
		)
		_, _ = b.sendMessage(ctx, msg.Chat.ID, "无权限")
		return
	}

	if text != "" {
		if err := b.taskStore.CreateInboundMessage(ctx, msg.Chat.ID, msg.From.ID, msg.MessageID, text, time.Now()); err != nil {
			b.logger.Error("persist inbound message failed",
				"chat_id", msg.Chat.ID,
				"user_id", msg.From.ID,
				"message_id", msg.MessageID,
				"error", err,
			)
			_, _ = b.sendMessage(ctx, msg.Chat.ID, fmt.Sprintf("消息持久化失败：%v", err))
			return
		}
	}

	command, _ := splitCommand(text)
	if command == "/start" {
		b.logger.Info("telegram start command", "chat_id", msg.Chat.ID, "user_id", msg.From.ID)
		b.clearPendingAction(msg.From.ID)
		b.sendMainMenu(ctx, msg.Chat.ID, "")
		return
	}

	pending := b.getPendingAction(msg.From.ID)
	if pending != pendingNone {
		if text == "" {
			_, _ = b.sendMessage(ctx, msg.Chat.ID, "请输入有效值，或发送 /start 取消。")
			return
		}
		b.logger.Info("telegram pending input received",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID,
			"pending_action", pending,
			"value", truncate(text, 200),
		)
		if err := b.applyPendingAction(pending, text); err != nil {
			b.logger.Error("telegram pending input apply failed",
				"chat_id", msg.Chat.ID,
				"user_id", msg.From.ID,
				"pending_action", pending,
				"error", err,
			)
			_, _ = b.sendMessage(ctx, msg.Chat.ID, fmt.Sprintf("设置失败：%v\n请重新输入，或发送 /start 取消。", err))
			return
		}
		b.logger.Info("telegram pending input applied",
			"chat_id", msg.Chat.ID,
			"user_id", msg.From.ID,
			"pending_action", pending,
		)
		b.clearPendingAction(msg.From.ID)
		b.sendMainMenu(ctx, msg.Chat.ID, "配置已更新并生效。")
		return
	}

	if text == "" {
		return
	}

	b.enqueueDrawTask(ctx, msg, text)
}

func (b *Bot) handleCallbackQuery(ctx context.Context, query CallbackQuery) {
	chatID := int64(0)
	messageID := int64(0)
	if query.Message != nil {
		chatID = query.Message.Chat.ID
		messageID = query.Message.MessageID
	}
	b.logger.Info("telegram callback received",
		"callback_id", query.ID,
		"chat_id", chatID,
		"user_id", query.From.ID,
		"message_id", messageID,
		"data", query.Data,
	)

	if !isAdminUser(b.cfg.Snapshot().Telegram.AdminUserID, query.From.ID) {
		_ = b.answerCallbackQuery(ctx, query.ID, "无权限", true)
		return
	}
	if query.Message == nil {
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新 /start", true)
		return
	}

	chatID = query.Message.Chat.ID
	messageID = query.Message.MessageID
	data := strings.TrimSpace(query.Data)

	switch {
	case data == cbSetLLMBaseURL:
		b.logger.Info("telegram callback set llm base url", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetLLMBaseURL)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的 LLM API 地址", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的 LLM API 地址。\n发送 /start 取消。")

	case data == cbSetLLMAPIKey:
		b.logger.Info("telegram callback set llm api key", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetLLMAPIKey)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的 LLM Key", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的 LLM Key。\n发送 /start 取消。")

	case data == cbSetArtist:
		b.logger.Info("telegram callback set artist", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetArtist)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的画师串", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的画师串（将拼接到正面提示词前）。\n发送 /start 取消。")

	case data == cbSetImageSize:
		b.logger.Info("telegram callback open size menu", "chat_id", chatID, "user_id", query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "请选择默认图像大小", false)
		shape := b.cfg.Snapshot().Generation.ShapeDefault
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, buildSizeMenuText(shape), sizeMenuMarkup()); err != nil {
			b.logger.Warn("edit size menu failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, buildSizeMenuText(shape), sizeMenuMarkup())
		}

	case data == cbBackMain:
		b.logger.Info("telegram callback back main menu", "chat_id", chatID, "user_id", query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "已返回主菜单", false)
		snapshot := b.cfg.Snapshot()
		text := buildMainMenuText("", snapshot.Generation.ShapeDefault, snapshot.Generation.Artist)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, mainMenuMarkup()); err != nil {
			b.logger.Warn("edit back main failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
		}

	case strings.HasPrefix(data, cbSizePrefix):
		shape := strings.TrimPrefix(data, cbSizePrefix)
		b.logger.Info("telegram callback set image size", "chat_id", chatID, "user_id", query.From.ID, "shape", shape)
		if err := b.cfg.SetByPath("generation.shape_default", shape); err != nil {
			_ = b.answerCallbackQuery(ctx, query.ID, "设置失败", true)
			_, _ = b.sendMessage(ctx, chatID, fmt.Sprintf("设置失败：%v", err))
			return
		}
		_ = b.answerCallbackQuery(ctx, query.ID, fmt.Sprintf("已设置为 %s", shape), false)
		snapshot := b.cfg.Snapshot()
		text := buildMainMenuText(fmt.Sprintf("默认图像大小已更新为 %s。", shape), snapshot.Generation.ShapeDefault, snapshot.Generation.Artist)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, mainMenuMarkup()); err != nil {
			b.logger.Warn("edit after size set failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
		}

	case strings.HasPrefix(data, cbRegenPrefix), strings.HasPrefix(data, cbRetryPrefix):
		originTaskID := strings.TrimPrefix(data, cbRegenPrefix)
		if strings.HasPrefix(data, cbRetryPrefix) {
			originTaskID = strings.TrimPrefix(data, cbRetryPrefix)
		}
		b.logger.Info("telegram callback regen task", "chat_id", chatID, "user_id", query.From.ID, "origin_task_id", originTaskID)
		originTask, ok := b.getRetryTask(ctx, originTaskID)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新生成", true)
			return
		}

		taskID, err := b.taskStore.NextTaskID(ctx)
		if err != nil {
			b.logger.Error("regen next task id failed", "origin_task_id", originTaskID, "error", err)
			_ = b.answerCallbackQuery(ctx, query.ID, "重新生成失败：任务编号生成失败", true)
			return
		}

		retryTask := types.DrawTask{
			TaskID:          taskID,
			ChatID:          chatID,
			UserID:          query.From.ID,
			StatusMessageID: messageID,
			Prompt:          originTask.Prompt,
			Shape:           originTask.Shape,
			Seed:            originTask.Seed,
			RetryOfTaskID:   originTaskID,
			CreatedAt:       time.Now(),
		}
		if err := b.taskStore.CreateTask(ctx, retryTask); err != nil {
			b.logger.Error("create regen task record failed", "task_id", taskID, "origin_task_id", originTaskID, "error", err)
			_ = b.answerCallbackQuery(ctx, query.ID, "重新生成失败：任务记录写入失败", true)
			return
		}

		taskID, queuePos := b.queue.Enqueue(retryTask)
		b.logger.Info("telegram regen task enqueued", "new_task_id", taskID, "origin_task_id", originTaskID, "queue_pos", queuePos)
		retryTask.TaskID = taskID
		b.rememberRetryTask(retryTask)
		_ = b.answerCallbackQuery(ctx, query.ID, "已提交重新生成", false)
		statusText := fmt.Sprintf("任务已重新生成提交\nTask ID: %s\n状态: queued\n队列位置: %d", taskID, queuePos)
		if err := b.editMessage(ctx, chatID, messageID, statusText); err != nil {
			b.logger.Warn("edit regen status failed", "task_id", taskID, "error", err)
			_, _ = b.sendMessage(ctx, chatID, statusText)
		}

	case strings.HasPrefix(data, cbGalleryPrev), strings.HasPrefix(data, cbGalleryNext):
		currentTaskID := strings.TrimPrefix(data, cbGalleryPrev)
		direction := "prev"
		if strings.HasPrefix(data, cbGalleryNext) {
			currentTaskID = strings.TrimPrefix(data, cbGalleryNext)
			direction = "next"
		}
		target, ok, err := b.resolveGalleryTarget(ctx, chatID, messageID, currentTaskID, direction)
		if err != nil {
			b.logger.Warn("resolve gallery target failed",
				"chat_id", chatID,
				"message_id", messageID,
				"task_id", currentTaskID,
				"direction", direction,
				"error", err,
			)
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新生成", true)
			return
		}
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "暂无可浏览图片", true)
			return
		}
		markup := b.buildTaskActionMarkup(ctx, chatID, messageID, target.Caption)
		if err := b.editMessagePhotoWithMarkup(ctx, chatID, messageID, target.FilePath, target.Caption, markup); err != nil {
			b.logger.Warn("edit gallery photo failed",
				"chat_id", chatID,
				"message_id", messageID,
				"task_id", target.TaskID,
				"error", err,
			)
			_ = b.answerCallbackQuery(ctx, query.ID, "切换失败，请稍后重试", true)
			return
		}
		_ = b.answerCallbackQuery(ctx, query.ID, "已切换图片", false)

	default:
		_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新 /start", true)
	}
}

func (b *Bot) enqueueDrawTask(ctx context.Context, msg Message, prompt string) {
	cfg := b.cfg.Snapshot()
	b.logger.Info("draw enqueue requested",
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID,
		"shape", cfg.Generation.ShapeDefault,
		"prompt", truncate(prompt, 400),
	)

	statusMessageID, err := b.sendMessageReply(ctx, msg.Chat.ID, "任务创建中...", msg.MessageID)
	if err != nil {
		b.logger.Warn("send initial status failed", "error", err)
		statusMessageID = 0
	}

	taskID, err := b.taskStore.NextTaskID(ctx)
	if err != nil {
		b.logger.Error("next task id failed", "error", err)
		failText := fmt.Sprintf("任务创建失败：生成任务编号失败（%v）", err)
		if statusMessageID > 0 {
			_ = b.editMessage(ctx, msg.Chat.ID, statusMessageID, failText)
		} else {
			_, _ = b.sendMessage(ctx, msg.Chat.ID, failText)
		}
		return
	}

	task := types.DrawTask{
		TaskID:          taskID,
		ChatID:          msg.Chat.ID,
		UserID:          msg.From.ID,
		StatusMessageID: statusMessageID,
		Prompt:          prompt,
		Shape:           cfg.Generation.ShapeDefault,
		CreatedAt:       time.Now(),
	}
	if err := b.taskStore.CreateTask(ctx, task); err != nil {
		b.logger.Error("create task record failed", "task_id", taskID, "error", err)
		failText := fmt.Sprintf("任务创建失败：写入任务记录失败（%v）", err)
		if statusMessageID > 0 {
			_ = b.editMessage(ctx, msg.Chat.ID, statusMessageID, failText)
		} else {
			_, _ = b.sendMessage(ctx, msg.Chat.ID, failText)
		}
		return
	}

	taskID, queuePos := b.queue.Enqueue(task)
	b.rememberRetryTask(task)
	b.logger.Info("draw task enqueued",
		"task_id", taskID,
		"chat_id", msg.Chat.ID,
		"user_id", msg.From.ID,
		"queue_pos", queuePos,
		"shape", task.Shape,
	)
	statusText := fmt.Sprintf("任务已提交\nTask ID: %s\n状态: queued\n队列位置: %d", taskID, queuePos)

	if statusMessageID > 0 {
		if err := b.editMessage(ctx, msg.Chat.ID, statusMessageID, statusText); err != nil {
			b.logger.Warn("edit initial status failed", "task_id", taskID, "error", err)
			_, _ = b.sendMessage(ctx, msg.Chat.ID, statusText)
		}
		return
	}
	_, _ = b.sendMessage(ctx, msg.Chat.ID, statusText)
}

func (b *Bot) sendMainMenu(ctx context.Context, chatID int64, notice string) {
	snapshot := b.cfg.Snapshot()
	text := buildMainMenuText(notice, snapshot.Generation.ShapeDefault, snapshot.Generation.Artist)
	_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
}

func (b *Bot) applyPendingAction(action PendingAction, value string) error {
	switch action {
	case pendingSetLLMBaseURL:
		return b.cfg.SetByPath("llm.base_url", value)
	case pendingSetLLMAPIKey:
		return b.cfg.SetByPath("llm.api_key", value)
	case pendingSetArtist:
		return b.cfg.SetByPath("generation.artist", value)
	default:
		return fmt.Errorf("未知待处理动作")
	}
}

func (b *Bot) setPendingAction(userID int64, action PendingAction) {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	if action == pendingNone {
		delete(b.pendingInput, userID)
		return
	}
	b.pendingInput[userID] = action
}

func (b *Bot) getPendingAction(userID int64) PendingAction {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	return b.pendingInput[userID]
}

func (b *Bot) clearPendingAction(userID int64) {
	b.setPendingAction(userID, pendingNone)
}

func (b *Bot) setMyCommands(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/setMyCommands", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"commands": []map[string]string{
			{"command": "start", "description": "打开主菜单"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("setMyCommands failed: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("setMyCommands api error: %s", out.Description)
	}
	return nil
}

func (b *Bot) answerCallbackQuery(ctx context.Context, callbackID string, text string, showAlert bool) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/answerCallbackQuery", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
		"show_alert":        showAlert,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("answerCallbackQuery failed: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("answerCallbackQuery api error: %s", out.Description)
	}
	return nil
}

func (b *Bot) getUpdates(ctx context.Context) ([]Update, error) {
	cfg := b.cfg.Snapshot()
	values := url.Values{}
	values.Set("timeout", "25")
	values.Set("offset", strconv.FormatInt(b.updateOffset, 10))

	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?%s", apiBase, cfg.Telegram.BotToken, values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, truncate(string(body), 300))
	}

	var out updatesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram api error: %s", out.Description)
	}
	return out.Result, nil
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	return b.sendMessageWithMarkupAndReply(ctx, chatID, text, b.buildTaskActionMarkup(ctx, chatID, 0, text), 0)
}

func (b *Bot) sendMessageReply(ctx context.Context, chatID int64, text string, replyToMessageID int64) (int64, error) {
	return b.sendMessageWithMarkupAndReply(ctx, chatID, text, b.buildTaskActionMarkup(ctx, chatID, 0, text), replyToMessageID)
}

func (b *Bot) sendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) (int64, error) {
	return b.sendMessageWithMarkupAndReply(ctx, chatID, text, markup, 0)
}

func (b *Bot) sendMessageWithMarkupAndReply(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup, replyToMessageID int64) (int64, error) {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	if replyToMessageID > 0 {
		payload["reply_to_message_id"] = replyToMessageID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("sendMessage failed: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var out sendMessageResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return 0, err
	}
	if !out.OK {
		return 0, fmt.Errorf("sendMessage api error: %s", out.Description)
	}
	return out.Result.MessageID, nil
}

func (b *Bot) editMessage(ctx context.Context, chatID int64, messageID int64, text string) error {
	return b.editMessageWithMarkup(ctx, chatID, messageID, text, b.buildTaskActionMarkup(ctx, chatID, messageID, text))
}

func (b *Bot) editMessageWithMarkup(ctx context.Context, chatID int64, messageID int64, text string, markup *InlineKeyboardMarkup) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/editMessageText", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		if strings.Contains(bodyStr, "message is not modified") {
			return nil
		}
		if shouldFallbackToCaption(bodyStr) {
			return b.editMessageCaptionWithMarkup(ctx, chatID, messageID, text, markup)
		}
		return fmt.Errorf("editMessageText failed: status=%d body=%s", resp.StatusCode, truncate(bodyStr, 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("editMessageText api error: %s", out.Description)
	}
	return nil
}

func (b *Bot) editMessageCaptionWithMarkup(ctx context.Context, chatID int64, messageID int64, caption string, markup *InlineKeyboardMarkup) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/editMessageCaption", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"caption":    caption,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		if strings.Contains(bodyStr, "message is not modified") {
			return nil
		}
		return fmt.Errorf("editMessageCaption failed: status=%d body=%s", resp.StatusCode, truncate(bodyStr, 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("editMessageCaption api error: %s", out.Description)
	}
	return nil
}

func (b *Bot) sendPhoto(ctx context.Context, chatID int64, filePath string, caption string) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/sendPhoto", apiBase, cfg.Telegram.BotToken)
	markup := b.buildTaskActionMarkup(ctx, chatID, 0, caption)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return err
	}
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return err
		}
	}
	if markup != nil {
		replyMarkupJSON, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		if err := writer.WriteField("reply_markup", string(replyMarkupJSON)); err != nil {
			return err
		}
	}

	part, err := writer.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sendPhoto failed: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("sendPhoto api error: %s", out.Description)
	}
	return nil
}

func (b *Bot) editMessagePhoto(ctx context.Context, chatID int64, messageID int64, filePath string, caption string) error {
	return b.editMessagePhotoWithMarkup(ctx, chatID, messageID, filePath, caption, nil)
}

func (b *Bot) editMessagePhotoWithMarkup(ctx context.Context, chatID int64, messageID int64, filePath string, caption string, markup *InlineKeyboardMarkup) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/editMessageMedia", apiBase, cfg.Telegram.BotToken)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		return err
	}
	if err := writer.WriteField("message_id", strconv.FormatInt(messageID, 10)); err != nil {
		return err
	}
	media := map[string]any{
		"type":  "photo",
		"media": "attach://photo",
	}
	if strings.TrimSpace(caption) != "" {
		media["caption"] = caption
	}
	mediaJSON, err := json.Marshal(media)
	if err != nil {
		return err
	}
	if err := writer.WriteField("media", string(mediaJSON)); err != nil {
		return err
	}
	if markup != nil {
		replyMarkupJSON, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		if err := writer.WriteField("reply_markup", string(replyMarkupJSON)); err != nil {
			return err
		}
	}

	part, err := writer.CreateFormFile("photo", filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		if strings.Contains(bodyStr, "message is not modified") {
			return nil
		}
		return fmt.Errorf("editMessageMedia failed: status=%d body=%s", resp.StatusCode, truncate(bodyStr, 300))
	}

	var out apiResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("editMessageMedia api error: %s", out.Description)
	}
	return nil
}

func mainMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "设置 LLM API", CallbackData: cbSetLLMBaseURL}},
			{{Text: "设置 LLM Key", CallbackData: cbSetLLMAPIKey}},
			{{Text: "设置画师串", CallbackData: cbSetArtist}},
			{{Text: "设置图像大小", CallbackData: cbSetImageSize}},
		},
	}
}

func sizeMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "方形 (1024x1024)", CallbackData: "size:square"}, {Text: "横向 (1216x832)", CallbackData: "size:landscape"}},
			{{Text: "纵向 (832x1216)", CallbackData: "size:portrait"}},
			{{Text: "返回主菜单", CallbackData: cbBackMain}},
		},
	}
}

func buildMainMenuText(notice string, shape string, artist string) string {
	artist = strings.TrimSpace(artist)
	artistDisplay := "未设置"
	if artist != "" {
		artistDisplay = truncate(artist, 120)
	}
	menu := fmt.Sprintf("主菜单\n请选择操作：\n- 设置 LLM API\n- 设置 LLM Key\n- 设置画师串\n- 设置图像大小\n\n当前默认图像大小: %s\n当前画师串: %s", shape, artistDisplay)
	if strings.TrimSpace(notice) == "" {
		return menu
	}
	return notice + "\n\n" + menu
}

func buildSizeMenuText(shape string) string {
	return fmt.Sprintf("请选择默认图像大小。\n当前: %s", shape)
}

func (b *Bot) rememberRetryTask(task types.DrawTask) {
	if strings.TrimSpace(task.TaskID) == "" {
		return
	}
	b.retryMu.Lock()
	b.retryTask[task.TaskID] = task
	b.retryMu.Unlock()
}

func (b *Bot) getRetryTask(ctx context.Context, taskID string) (types.DrawTask, bool) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.DrawTask{}, false
	}
	b.retryMu.Lock()
	task, ok := b.retryTask[taskID]
	b.retryMu.Unlock()
	if ok {
		return task, true
	}

	task, err := b.taskStore.GetTaskByID(ctx, taskID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			b.logger.Warn("load retry task from store failed", "task_id", taskID, "error", err)
		}
		return types.DrawTask{}, false
	}
	b.rememberRetryTask(task)
	return task, true
}

func (b *Bot) buildTaskActionMarkup(ctx context.Context, chatID int64, messageID int64, text string) *InlineKeyboardMarkup {
	taskID := extractTaskIDFromStatus(text)
	if taskID == "" {
		return nil
	}
	status := extractTaskStatus(text)
	if !statusAllowsRegen(status) {
		return nil
	}
	if _, ok := b.getRetryTask(ctx, taskID); !ok {
		return nil
	}

	rows := [][]InlineKeyboardButton{
		{{Text: "重新生成", CallbackData: cbRegenPrefix + taskID}},
	}

	if messageID <= 0 {
		return &InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	items, err := b.taskStore.ListGalleryItems(ctx, chatID, messageID)
	if err != nil {
		b.logger.Warn("list gallery items failed", "chat_id", chatID, "message_id", messageID, "error", err)
		return &InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	if len(items) >= 2 {
		rows = append(rows, []InlineKeyboardButton{
			{Text: "上一页", CallbackData: cbGalleryPrev + taskID},
			{Text: "下一页", CallbackData: cbGalleryNext + taskID},
		})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func extractTaskIDFromStatus(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "任务 ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.TrimSpace(fields[1])
			}
		}
		if strings.HasPrefix(line, "Task ID:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Task ID:"))
		}
	}
	return ""
}

func extractTaskStatus(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "状态:") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "状态:")))
		}
		if strings.HasPrefix(line, "任务 ") && strings.HasSuffix(line, " 完成") {
			return types.StatusCompleted
		}
	}
	return ""
}

func statusAllowsRegen(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == types.StatusFailed || status == types.StatusCompleted {
		return true
	}
	return strings.HasPrefix(status, "completed")
}

func shouldFallbackToCaption(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "there is no text in the message to edit") ||
		strings.Contains(body, "message caption is not modified") ||
		strings.Contains(body, "message can't be edited")
}

func (b *Bot) resolveGalleryTarget(ctx context.Context, chatID int64, messageID int64, currentTaskID string, direction string) (store.GalleryItem, bool, error) {
	items, err := b.taskStore.ListGalleryItems(ctx, chatID, messageID)
	if err != nil {
		return store.GalleryItem{}, false, err
	}
	if len(items) == 0 {
		return store.GalleryItem{}, false, nil
	}

	currentIdx := -1
	for idx, item := range items {
		if item.TaskID == currentTaskID {
			currentIdx = idx
			break
		}
	}
	if currentIdx < 0 {
		// When current state is failed/processing (not in gallery), use latest successful page as pivot.
		currentIdx = len(items) - 1
	}

	targetIdx := currentIdx
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "prev":
		targetIdx = (currentIdx - 1 + len(items)) % len(items)
	case "next":
		targetIdx = (currentIdx + 1) % len(items)
	default:
		return store.GalleryItem{}, false, fmt.Errorf("unknown direction: %s", direction)
	}
	target := items[targetIdx]
	if strings.TrimSpace(target.FilePath) == "" {
		return store.GalleryItem{}, false, fmt.Errorf("empty target file path")
	}
	if _, err := os.Stat(target.FilePath); err != nil {
		return store.GalleryItem{}, false, fmt.Errorf("target file not found: %w", err)
	}
	return target, true, nil
}

func splitCommand(text string) (command string, rest string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", ""
	}
	command = strings.ToLower(parts[0])
	if i := strings.Index(command, "@"); i >= 0 {
		command = command[:i]
	}
	if len(parts) > 1 {
		rest = strings.Join(parts[1:], " ")
	}
	return command, rest
}

func newTelegramHTTPClient(proxyRaw string, logger *slog.Logger) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw == "" {
		return &http.Client{Transport: transport}
	}

	parsed, err := url.Parse(proxyRaw)
	if err != nil {
		logger.Warn("invalid telegram proxy url, fallback to direct", "proxy_url", proxyRaw, "error", err)
		return &http.Client{Transport: transport}
	}
	transport.Proxy = http.ProxyURL(parsed)
	return &http.Client{Transport: transport}
}

func isAdminUser(adminUserID int64, userID int64) bool {
	return adminUserID > 0 && adminUserID == userID
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

type updatesResponse struct {
	OK          bool     `json:"ok"`
	Result      []Update `json:"result"`
	Description string   `json:"description"`
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type User struct {
	ID int64 `json:"id"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}
