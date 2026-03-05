package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (b *Bot) setLLMModelSession(userID int64, models []string) llmModelSession {
	b.llmModelMu.Lock()
	defer b.llmModelMu.Unlock()

	b.llmModelSeq++
	sessionID := nextLLMModelSessionID(time.Now(), b.llmModelSeq)
	copied := make([]string, len(models))
	copy(copied, models)
	session := llmModelSession{
		SessionID: sessionID,
		Models:    copied,
		ExpiresAt: time.Now().Add(llmModelTTL),
	}
	b.llmModelSessions[userID] = session
	return session
}

func (b *Bot) getLLMModelSession(userID int64, sessionID string) (llmModelSession, bool) {
	b.llmModelMu.Lock()
	defer b.llmModelMu.Unlock()

	session, ok := b.llmModelSessions[userID]
	if !ok {
		return llmModelSession{}, false
	}
	if strings.TrimSpace(sessionID) != "" && session.SessionID != strings.TrimSpace(sessionID) {
		return llmModelSession{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(b.llmModelSessions, userID)
		return llmModelSession{}, false
	}
	return session, true
}

func nextLLMModelSessionID(now time.Time, seq uint64) string {
	return strconv.FormatInt(now.Unix(), 36) + strconv.FormatUint(seq%1296, 36)
}

func (b *Bot) showLLMModelMenu(ctx context.Context, chatID int64, messageID int64, session llmModelSession, currentModel string, page int, notice string) {
	page = clampLLMModelPage(page, len(session.Models))
	text := buildLLMModelMenuText(notice, currentModel, len(session.Models), page)
	markup := llmModelMenuMarkup(session.SessionID, page, session.Models)
	if err := b.editMessageWithMarkup(ctx, chatID, messageID, text, markup); err != nil {
		b.logger.Warn("edit llm model menu failed", "error", err)
		_, _ = b.sendMessageWithMarkup(ctx, chatID, text, markup)
	}
}

func (b *Bot) fallbackLLMModelManualInput(ctx context.Context, callbackID string, chatID int64, userID int64) {
	b.setPendingAction(userID, pendingSetLLMModel)
	_ = b.answerCallbackQuery(ctx, callbackID, "拉取失败，改为手工输入", false)
	_, _ = b.sendMessage(ctx, chatID, "拉取模型列表失败，改为手工输入。\n请发送新的 LLM 模型。\n发送 /start 取消。")
}

func parseLLMModelSessionWithIndex(data string, prefix string) (sessionID string, index int, ok bool) {
	raw := strings.TrimSpace(strings.TrimPrefix(data, prefix))
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	sessionID = strings.TrimSpace(parts[0])
	if sessionID == "" {
		return "", 0, false
	}
	index, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", 0, false
	}
	if index < 0 {
		return "", 0, false
	}
	return sessionID, index, true
}

func parseLLMModelSessionID(data string, prefix string) (sessionID string, ok bool) {
	sessionID = strings.TrimSpace(strings.TrimPrefix(data, prefix))
	return sessionID, sessionID != ""
}

func buildLLMModelPickedNotice(model string) string {
	return fmt.Sprintf("LLM 模型已更新为 %s。", model)
}
