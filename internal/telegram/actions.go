package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"grimoire/internal/store"
	"grimoire/internal/types"
)

func mainMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "更改端点", CallbackData: cbSetLLMBaseURL}},
			{{Text: "更改 Key", CallbackData: cbSetLLMAPIKey}},
			{{Text: "更改模型", CallbackData: cbSetLLMModel}},
		},
	}
}

func naiMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "更改 Key", CallbackData: cbSetNAIAPIKey}},
			{{Text: "更改模型", CallbackData: cbSetNAIModel}},
		},
	}
}

func imageMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "更改图像尺寸", CallbackData: cbSetImageSize}},
			{{Text: "更改画师串", CallbackData: cbSetArtist}},
		},
	}
}

func sizeMenuMarkup() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{{Text: "方形 (1024x1024)", CallbackData: cbSizePrefix + "square"}, {Text: "横向 (1216x832)", CallbackData: cbSizePrefix + "landscape"}},
			{{Text: "纵向 (832x1216)", CallbackData: cbSizePrefix + "portrait"}},
			{{Text: "返回绘图设置", CallbackData: cbBackImageMenu}},
		},
	}
}

func buildStartText() string {
	return "Grimoire Bot\n\n发送任意文本可直接发起绘图。\n使用 /llm 查看或修改 LLM 设置。\n使用 /nai 查看或修改 NAI 设置。\n使用 /img 查看或修改绘图设置。"
}

func buildMainMenuText(notice string, endpoint string, model string) string {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	endpointDisplay := "未设置"
	if endpoint != "" {
		endpointDisplay = endpoint
	}
	modelDisplay := "未设置"
	if model != "" {
		modelDisplay = model
	}
	menu := fmt.Sprintf("LLM 设置\n当前端点: %s\n当前模型: %s\n\n请选择操作：\n- 更改端点\n- 更改 Key\n- 更改模型", endpointDisplay, modelDisplay)
	if strings.TrimSpace(notice) == "" {
		return menu
	}
	return notice + "\n\n" + menu
}

func buildNAIMenuText(notice string, endpoint string, model string) string {
	endpoint = strings.TrimSpace(endpoint)
	model = strings.TrimSpace(model)
	endpointDisplay := "未设置"
	if endpoint != "" {
		endpointDisplay = endpoint
	}
	modelDisplay := "未设置"
	if model != "" {
		modelDisplay = model
	}
	menu := fmt.Sprintf("NAI 设置\n当前端点: %s\n当前模型: %s\n\n请选择操作：\n- 更改 Key\n- 更改模型", endpointDisplay, modelDisplay)
	if strings.TrimSpace(notice) == "" {
		return menu
	}
	return notice + "\n\n" + menu
}

func buildImageMenuText(notice string, shape string, size string, artist string) string {
	artist = strings.TrimSpace(artist)
	artistDisplay := "未设置"
	if artist != "" {
		artistDisplay = truncate(artist, 120)
	}
	shape = strings.TrimSpace(shape)
	size = strings.TrimSpace(size)
	if shape == "" {
		shape = "未设置"
	}
	shapeDisplay := shape
	if size != "" {
		shapeDisplay = fmt.Sprintf("%s (%s)", shape, size)
	}
	menu := fmt.Sprintf("绘图设置\n当前图像尺寸: %s\n当前画师串: %s\n\n请选择操作：\n- 更改图像尺寸\n- 更改画师串", shapeDisplay, artistDisplay)
	if strings.TrimSpace(notice) == "" {
		return menu
	}
	return notice + "\n\n" + menu
}

func buildSizeMenuText(shape string, size string) string {
	shape = strings.TrimSpace(shape)
	size = strings.TrimSpace(size)
	if shape == "" {
		shape = "未设置"
	}
	if size == "" {
		return fmt.Sprintf("请选择默认图像大小。\n当前: %s", shape)
	}
	return fmt.Sprintf("请选择默认图像大小。\n当前: %s (%s)", shape, size)
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
	if statusAllowsStop(status) {
		return &InlineKeyboardMarkup{
			InlineKeyboard: [][]InlineKeyboardButton{
				{{Text: "停止生成", CallbackData: cbStopPrefix + taskID}},
			},
		}
	}
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

func statusAllowsStop(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == types.StatusQueued || status == types.StatusProcessing
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
