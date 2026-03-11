package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
)

func (b *Bot) setMyCommands(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/bot%s/setMyCommands", apiBase, b.cfg.Telegram.BotToken)
	payload := map[string]any{
		"commands": []map[string]string{
			{"command": "start", "description": "机器人介绍"},
			{"command": "img", "description": "绘图偏好"},
			{"command": "balance", "description": "查询 NAI 余额"},
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
	return b.doAPIRequest(req, nil)
}

func (b *Bot) answerCallbackQuery(ctx context.Context, callbackID string, text string, showAlert bool) error {
	endpoint := fmt.Sprintf("%s/bot%s/answerCallbackQuery", apiBase, b.cfg.Telegram.BotToken)
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
	return b.doAPIRequest(req, nil)
}

func (b *Bot) getUpdates(ctx context.Context) ([]Update, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?timeout=25&offset=%d", apiBase, b.cfg.Telegram.BotToken, b.updateOffset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var response updatesResponse
	if err := b.doAPIRequest(req, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup, replyToMessageID int64) (int64, error) {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, b.cfg.Telegram.BotToken)
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

	var response sendMessageResponse
	if err := b.doAPIRequest(req, &response); err != nil {
		return 0, err
	}
	return response.Result.MessageID, nil
}

func (b *Bot) editMessage(ctx context.Context, chatID int64, messageID int64, text string, markup *InlineKeyboardMarkup) error {
	endpoint := fmt.Sprintf("%s/bot%s/editMessageText", apiBase, b.cfg.Telegram.BotToken)
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
	err = b.doAPIRequest(req, nil)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (b *Bot) sendPhoto(ctx context.Context, chatID int64, replyToMessageID int64, filename string, caption string, content []byte) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendPhoto", apiBase, b.cfg.Telegram.BotToken)
	payload := map[string]any{
		"chat_id": chatID,
	}
	if replyToMessageID > 0 {
		payload["reply_to_message_id"] = replyToMessageID
	}
	if strings.TrimSpace(caption) != "" {
		payload["caption"] = caption
	}
	req, err := newMultipartPhotoRequest(ctx, endpoint, payload, filename, content)
	if err != nil {
		return err
	}
	return b.doAPIRequest(req, nil)
}

func (b *Bot) deleteMessage(ctx context.Context, chatID int64, messageID int64) error {
	endpoint := fmt.Sprintf("%s/bot%s/deleteMessage", apiBase, b.cfg.Telegram.BotToken)
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
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
	return b.doAPIRequest(req, nil)
}

func newMultipartPhotoRequest(ctx context.Context, endpoint string, fields map[string]any, filename string, content []byte) (*http.Request, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for key, value := range fields {
		if err := writer.WriteField(key, stringifyFieldValue(value)); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}

	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if _, err := part.Write(content); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func stringifyFieldValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return fmt.Sprint(value)
	}
}

func (b *Bot) doAPIRequest(req *http.Request, target any) error {
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram status=%d body=%s", resp.StatusCode, string(body))
	}
	if target == nil {
		var response apiResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		if !response.OK {
			return fmt.Errorf("telegram api error: %s", response.Description)
		}
		return nil
	}

	switch typed := target.(type) {
	case *updatesResponse:
		if err := json.Unmarshal(body, typed); err != nil {
			return err
		}
		if !typed.OK {
			return fmt.Errorf("telegram api error: %s", typed.Description)
		}
	case *sendMessageResponse:
		if err := json.Unmarshal(body, typed); err != nil {
			return err
		}
		if !typed.OK {
			return fmt.Errorf("telegram api error: %s", typed.Description)
		}
	default:
		if err := json.Unmarshal(body, target); err != nil {
			return err
		}
	}
	return nil
}
