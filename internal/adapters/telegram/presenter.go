package telegram

import (
	"fmt"
	"strings"

	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
)

func buildStartText() string {
	return "Grimoire v2\n\n发送文本即可进入需求对话，确认后再开始绘图。\n发送 /new 可新建一个会话并重新开始需求收敛。\n发送 /img 可修改全局默认图像尺寸和画师串。\n发送 /balance 可查询 NAI 余额。"
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
				{Text: "Small Portrait", CallbackData: requestShapeCallback(domaindraw.ShapeSmallPortrait)},
				{Text: "Small Landscape", CallbackData: requestShapeCallback(domaindraw.ShapeSmallLandscape)},
				{Text: "Small Square", CallbackData: requestShapeCallback(domaindraw.ShapeSmallSquare)},
			},
			{
				{Text: "Normal Portrait", CallbackData: requestShapeCallback(domaindraw.ShapePortrait)},
				{Text: "Normal Landscape", CallbackData: requestShapeCallback(domaindraw.ShapeLandscape)},
				{Text: "Normal Square", CallbackData: requestShapeCallback(domaindraw.ShapeSquare)},
			},
			{
				{Text: "Large Portrait", CallbackData: requestShapeCallback(domaindraw.ShapeLargePortrait)},
				{Text: "Large Landscape", CallbackData: requestShapeCallback(domaindraw.ShapeLargeLandscape)},
				{Text: "Large Square", CallbackData: requestShapeCallback(domaindraw.ShapeLargeSquare)},
			},
			{
				{Text: "设置画师串", CallbackData: requestArtistsSet},
				{Text: "清空画师串", CallbackData: requestArtistsClear},
			},
		},
	}
}

func buildArtistsPromptText() string {
	return "请发送新的画师串，或发送 /start 取消。"
}

func buildTaskStartedText() string {
	return "已开始绘图"
}

func buildNewSessionText() string {
	return "已开始新的会话，之前的对话不会影响后续需求。"
}

func taskProgressMarkup(taskID string) *InlineKeyboardMarkup {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "停止任务", CallbackData: taskStopPrefix + taskID},
			},
		},
	}
}

func resultTaskMarkup(taskID string) *InlineKeyboardMarkup {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	return &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "查看 prompt", CallbackData: taskPromptPrefix + taskID},
			},
			{
				{Text: "重新翻译并绘图", CallbackData: taskRetryTranslatePrefix + taskID},
				{Text: "不翻译并重新绘图", CallbackData: taskRetryDrawPrefix + taskID},
			},
		},
	}
}

func buildStoppedTaskText() string {
	return "已停止任务"
}

func buildPromptText(details taskapp.PromptDetails) string {
	lines := []string{
		"Prompt",
		"",
		"Global Prompt",
		strings.TrimSpace(details.Prompt),
		"",
		"Negative Prompt",
		displayPromptValue(details.NegativePrompt),
	}
	for index, character := range details.Characters {
		lines = append(lines,
			"",
			fmt.Sprintf("Character %d", index+1),
			"Prompt: "+displayPromptValue(character.Prompt),
			"Negative Prompt: "+displayPromptValue(character.NegativePrompt),
			"Position: "+displayPromptValue(character.Position),
		)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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

func displayArtist(artist string) string {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return "未设置"
	}
	return artist
}

func displayPromptValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "无"
	}
	return value
}
