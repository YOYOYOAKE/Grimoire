package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/types"
)

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
	switch command {
	case "/start":
		b.logger.Info("telegram start command", "chat_id", msg.Chat.ID, "user_id", msg.From.ID)
		b.clearPendingAction(msg.From.ID)
		_, _ = b.sendMessage(ctx, msg.Chat.ID, buildStartText())
		return
	case "/llm":
		b.logger.Info("telegram llm command removed", "chat_id", msg.Chat.ID, "user_id", msg.From.ID)
		b.clearPendingAction(msg.From.ID)
		_, _ = b.sendMessage(ctx, msg.Chat.ID, buildYAMLConfigOnlyText())
		return
	case "/nai":
		b.logger.Info("telegram nai command removed", "chat_id", msg.Chat.ID, "user_id", msg.From.ID)
		b.clearPendingAction(msg.From.ID)
		_, _ = b.sendMessage(ctx, msg.Chat.ID, buildYAMLConfigOnlyText())
		return
	case "/img":
		b.logger.Info("telegram img command", "chat_id", msg.Chat.ID, "user_id", msg.From.ID)
		b.clearPendingAction(msg.From.ID)
		b.sendImageMenu(ctx, msg.Chat.ID, "")
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
		switch pending {
		case pendingSetLLMBaseURL, pendingSetLLMAPIKey, pendingSetLLMModel:
			b.sendMainMenu(ctx, msg.Chat.ID, "配置已更新并生效。")
		case pendingSetNAIAPIKey, pendingSetNAIModel:
			b.sendNAIMenu(ctx, msg.Chat.ID, "配置已更新并生效。")
		case pendingSetArtist:
			b.sendImageMenu(ctx, msg.Chat.ID, "配置已更新并生效。")
		default:
			_, _ = b.sendMessage(ctx, msg.Chat.ID, "配置已更新并生效。")
		}
		return
	}

	if text == "" {
		return
	}

	if missing := b.cfg.MissingDrawConfigKeys(); len(missing) > 0 {
		_, _ = b.sendMessage(ctx, msg.Chat.ID, buildMissingDrawConfigText(missing))
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
	if isLegacyConfigCallback(data) {
		_ = b.answerCallbackQuery(ctx, query.ID, "请改 YAML 配置后重启", false)
		_, _ = b.sendMessage(ctx, chatID, buildYAMLConfigOnlyText())
		return
	}

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

	case data == cbSetLLMModel:
		b.logger.Info("telegram callback set llm model", "chat_id", chatID, "user_id", query.From.ID)
		b.clearPendingAction(query.From.ID)
		models, err := b.fetchLLMModels(ctx)
		if err != nil {
			b.logger.Warn("fetch llm models failed", "chat_id", chatID, "user_id", query.From.ID, "error", err)
			b.fallbackLLMModelManualInput(ctx, query.ID, chatID, query.From.ID)
			return
		}
		session := b.setLLMModelSession(query.From.ID, models)
		_ = b.answerCallbackQuery(ctx, query.ID, "请选择 LLM 模型", false)
		b.showLLMModelMenu(ctx, chatID, messageID, session, b.cfg.Snapshot().LLM.Model, 0, "")

	case data == cbSetNAIAPIKey:
		b.logger.Info("telegram callback set nai api key", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetNAIAPIKey)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的 NAI Key", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的 NAI Key。\n发送 /start 取消。")

	case data == cbSetNAIModel:
		b.logger.Info("telegram callback set nai model", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetNAIModel)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的 NAI 模型", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的 NAI 模型。\n发送 /start 取消。")

	case data == cbSetArtist:
		b.logger.Info("telegram callback set artist", "chat_id", chatID, "user_id", query.From.ID)
		b.setPendingAction(query.From.ID, pendingSetArtist)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的画师串", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的画师串（将拼接到正面提示词前）。\n发送 /start 取消。")

	case data == cbSetImageSize:
		b.logger.Info("telegram callback open size menu", "chat_id", chatID, "user_id", query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "请选择默认图像大小", false)
		snapshot := b.cfg.Snapshot()
		shape := snapshot.Generation.ShapeDefault
		size := snapshot.Generation.ShapeMap[shape]
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, buildSizeMenuText(shape, size), sizeMenuMarkup()); err != nil {
			b.logger.Warn("edit size menu failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, buildSizeMenuText(shape, size), sizeMenuMarkup())
		}

	case data == cbBackLLMMenu:
		b.logger.Info("telegram callback back llm menu", "chat_id", chatID, "user_id", query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "已返回 LLM 设置", false)
		snapshot := b.cfg.Snapshot()
		text := buildMainMenuText("", snapshot.LLM.BaseURL, snapshot.LLM.Model)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, mainMenuMarkup()); err != nil {
			b.logger.Warn("edit back llm menu failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
		}

	case strings.HasPrefix(data, cbLLMModelPickPrefix):
		sid, idx, ok := parseLLMModelSessionWithIndex(data, cbLLMModelPickPrefix)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新点击更改模型", true)
			return
		}
		session, ok := b.getLLMModelSession(query.From.ID, sid)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "模型列表已过期，请重新点击更改模型", true)
			return
		}
		if idx < 0 || idx >= len(session.Models) {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新点击更改模型", true)
			return
		}
		selectedModel := session.Models[idx]
		if err := b.cfg.SetByPath("llm.model", selectedModel); err != nil {
			_ = b.answerCallbackQuery(ctx, query.ID, "设置失败", true)
			_, _ = b.sendMessage(ctx, chatID, fmt.Sprintf("设置失败：%v", err))
			return
		}
		b.clearPendingAction(query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, fmt.Sprintf("已设置模型: %s", selectedModel), false)
		snapshot := b.cfg.Snapshot()
		text := buildMainMenuText(buildLLMModelPickedNotice(selectedModel), snapshot.LLM.BaseURL, snapshot.LLM.Model)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, mainMenuMarkup()); err != nil {
			b.logger.Warn("edit after llm model set failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
		}

	case strings.HasPrefix(data, cbLLMModelPagePrefix):
		sid, page, ok := parseLLMModelSessionWithIndex(data, cbLLMModelPagePrefix)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新点击更改模型", true)
			return
		}
		session, ok := b.getLLMModelSession(query.From.ID, sid)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "模型列表已过期，请重新点击更改模型", true)
			return
		}
		page = clampLLMModelPage(page, len(session.Models))
		_ = b.answerCallbackQuery(ctx, query.ID, fmt.Sprintf("第 %d 页", page+1), false)
		b.showLLMModelMenu(ctx, chatID, messageID, session, b.cfg.Snapshot().LLM.Model, page, "")

	case strings.HasPrefix(data, cbLLMModelRefreshPrefix):
		sid, page, ok := parseLLMModelSessionWithIndex(data, cbLLMModelRefreshPrefix)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新点击更改模型", true)
			return
		}
		if _, ok := b.getLLMModelSession(query.From.ID, sid); !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "模型列表已过期，请重新点击更改模型", true)
			return
		}
		models, err := b.fetchLLMModels(ctx)
		if err != nil {
			b.logger.Warn("refresh llm models failed", "chat_id", chatID, "user_id", query.From.ID, "error", err)
			_ = b.answerCallbackQuery(ctx, query.ID, "刷新模型列表失败", true)
			return
		}
		session := b.setLLMModelSession(query.From.ID, models)
		page = clampLLMModelPage(page, len(models))
		_ = b.answerCallbackQuery(ctx, query.ID, "模型列表已刷新", false)
		b.showLLMModelMenu(ctx, chatID, messageID, session, b.cfg.Snapshot().LLM.Model, page, "模型列表已刷新。")

	case strings.HasPrefix(data, cbLLMModelManualPrefix):
		sid, ok := parseLLMModelSessionID(data, cbLLMModelManualPrefix)
		if !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新点击更改模型", true)
			return
		}
		if _, ok := b.getLLMModelSession(query.From.ID, sid); !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "模型列表已过期，请重新点击更改模型", true)
			return
		}
		b.setPendingAction(query.From.ID, pendingSetLLMModel)
		_ = b.answerCallbackQuery(ctx, query.ID, "请发送新的 LLM 模型", false)
		_, _ = b.sendMessage(ctx, chatID, "请发送新的 LLM 模型。\n发送 /start 取消。")

	case data == cbBackImageMenu || data == cbBackMain:
		b.logger.Info("telegram callback back image menu", "chat_id", chatID, "user_id", query.From.ID)
		_ = b.answerCallbackQuery(ctx, query.ID, "已返回绘图设置", false)
		snapshot := b.cfg.Snapshot()
		shape := snapshot.Generation.ShapeDefault
		text := buildImageMenuText("", shape, snapshot.Generation.ShapeMap[shape], snapshot.Generation.Artist)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, imageMenuMarkup()); err != nil {
			b.logger.Warn("edit back image menu failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, imageMenuMarkup())
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
		newShape := snapshot.Generation.ShapeDefault
		text := buildImageMenuText(fmt.Sprintf("默认图像大小已更新为 %s。", shape), newShape, snapshot.Generation.ShapeMap[newShape], snapshot.Generation.Artist)
		if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, imageMenuMarkup()); err != nil {
			b.logger.Warn("edit after size set failed", "error", err)
			_, _ = b.sendMessageWithMarkup(ctx, chatID, text, imageMenuMarkup())
		}

	case strings.HasPrefix(data, cbRegenPrefix), strings.HasPrefix(data, cbRetryPrefix):
		originTaskID := strings.TrimPrefix(data, cbRegenPrefix)
		if strings.HasPrefix(data, cbRetryPrefix) {
			originTaskID = strings.TrimPrefix(data, cbRetryPrefix)
		}
		if missing := b.cfg.MissingDrawConfigKeys(); len(missing) > 0 {
			_ = b.answerCallbackQuery(ctx, query.ID, "缺少绘图配置", true)
			_, _ = b.sendMessage(ctx, chatID, buildMissingDrawConfigText(missing))
			return
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

	case strings.HasPrefix(data, cbStopPrefix):
		taskID := strings.TrimPrefix(data, cbStopPrefix)
		if strings.TrimSpace(taskID) == "" {
			_ = b.answerCallbackQuery(ctx, query.ID, "操作无效，请重新生成", true)
			return
		}
		if b.taskControl == nil {
			_ = b.answerCallbackQuery(ctx, query.ID, "暂不支持停止", true)
			return
		}
		if ok := b.taskControl.CancelTask(taskID); !ok {
			_ = b.answerCallbackQuery(ctx, query.ID, "任务不可停止", true)
			return
		}
		_ = b.answerCallbackQuery(ctx, query.ID, "已请求停止任务", false)
		text := fmt.Sprintf("任务 %s\n状态: cancelling\n阶段: 正在停止任务", taskID)
		if err := b.editMessage(ctx, chatID, messageID, text); err != nil {
			b.logger.Warn("edit stop status failed", "task_id", taskID, "error", err)
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
	text := buildMainMenuText(notice, snapshot.LLM.BaseURL, snapshot.LLM.Model)
	_, _ = b.sendMessageWithMarkup(ctx, chatID, text, mainMenuMarkup())
}

func (b *Bot) sendNAIMenu(ctx context.Context, chatID int64, notice string) {
	snapshot := b.cfg.Snapshot()
	text := buildNAIMenuText(notice, snapshot.NAI.BaseURL, snapshot.NAI.Model)
	_, _ = b.sendMessageWithMarkup(ctx, chatID, text, naiMenuMarkup())
}

func (b *Bot) sendImageMenu(ctx context.Context, chatID int64, notice string) {
	snapshot := b.cfg.Snapshot()
	shape := snapshot.Generation.ShapeDefault
	text := buildImageMenuText(notice, shape, snapshot.Generation.ShapeMap[shape], snapshot.Generation.Artist)
	_, _ = b.sendMessageWithMarkup(ctx, chatID, text, imageMenuMarkup())
}

func (b *Bot) applyPendingAction(action PendingAction, value string) error {
	switch action {
	case pendingSetLLMBaseURL:
		return errors.New(buildYAMLConfigOnlyText())
	case pendingSetLLMAPIKey:
		return errors.New(buildYAMLConfigOnlyText())
	case pendingSetLLMModel:
		return errors.New(buildYAMLConfigOnlyText())
	case pendingSetNAIAPIKey:
		return errors.New(buildYAMLConfigOnlyText())
	case pendingSetNAIModel:
		return errors.New(buildYAMLConfigOnlyText())
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

func buildMissingDrawConfigText(missing []string) string {
	if len(missing) == 0 {
		return "配置完整。"
	}
	return fmt.Sprintf(
		"当前缺少绘图配置：\n- %s\n\n请编辑 %s 后重启机器人。",
		strings.Join(missing, "\n- "),
		"configs/config.yaml",
	)
}

func buildYAMLConfigOnlyText() string {
	return "LLM/NAI 配置已迁移到 configs/config.yaml，请修改配置文件后重启机器人。"
}

func isLegacyConfigCallback(data string) bool {
	switch {
	case data == cbSetLLMBaseURL,
		data == cbSetLLMAPIKey,
		data == cbSetLLMModel,
		data == cbBackLLMMenu,
		data == cbSetNAIAPIKey,
		data == cbSetNAIModel,
		strings.HasPrefix(data, cbLLMModelPickPrefix),
		strings.HasPrefix(data, cbLLMModelPagePrefix),
		strings.HasPrefix(data, cbLLMModelRefreshPrefix),
		strings.HasPrefix(data, cbLLMModelManualPrefix):
		return true
	default:
		return false
	}
}
