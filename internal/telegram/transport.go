package telegram

import (
	"bytes"
	"context"
	"encoding/json"
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
)

func (b *Bot) setMyCommands(ctx context.Context) error {
	cfg := b.cfg.Snapshot()
	endpoint := fmt.Sprintf("%s/bot%s/setMyCommands", apiBase, cfg.Telegram.BotToken)

	payload := map[string]any{
		"commands": []map[string]string{
			{"command": "start", "description": "机器人介绍"},
			{"command": "llm", "description": "LLM 设置"},
			{"command": "nai", "description": "NAI 设置"},
			{"command": "img", "description": "绘图设置"},
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

func (b *Bot) fetchLLMModels(ctx context.Context) ([]string, error) {
	cfg := b.cfg.Snapshot()
	baseURL := strings.TrimSpace(cfg.LLM.BaseURL)
	apiKey := strings.TrimSpace(cfg.LLM.APIKey)
	if baseURL == "" {
		return nil, fmt.Errorf("llm.base_url 未设置")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("llm.api_key 未设置")
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := b.llmHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求模型列表失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取模型列表响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("模型列表接口返回非 200: status=%d body=%s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("解析模型列表响应失败: %w", err)
	}

	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		model := strings.TrimSpace(item.ID)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("模型列表为空")
	}
	return models, nil
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

func shouldFallbackToCaption(body string) bool {
	body = strings.ToLower(body)
	return strings.Contains(body, "there is no text in the message to edit") ||
		strings.Contains(body, "message caption is not modified") ||
		strings.Contains(body, "message can't be edited")
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

func newDirectHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// LLM upstream must bypass proxy; telegram proxy only applies to Telegram API.
	transport.Proxy = nil
	return &http.Client{Transport: transport}
}
