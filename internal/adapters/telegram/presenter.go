package telegram

import (
	"fmt"
	"strings"

	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
)

func buildStartText() string {
	return "Grimoire v2\n\n发送文本即可进入需求对话，确认后再开始绘图。\n发送 /img 可修改全局默认图像尺寸和画师串。\n发送 /balance 可查询 NAI 余额。"
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

func buildArtistsPromptText() string {
	return "请发送新的画师串，或发送 /start 取消。"
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
