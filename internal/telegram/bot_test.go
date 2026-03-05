package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/store"
	"grimoire/internal/types"
)

func TestHandleMessageFreeTextEnqueuesTask(t *testing.T) {
	t.Parallel()

	bot, q, taskStore, _, cfg := newTestBot(t)
	msg := Message{
		MessageID: 11,
		From:      &User{ID: cfg.Snapshot().Telegram.AdminUserID},
		Chat:      Chat{ID: 1001},
		Text:      "来一张可爱猫娘",
	}
	bot.handleMessage(context.Background(), msg)

	tasks := q.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Prompt != "来一张可爱猫娘" {
		t.Fatalf("unexpected prompt: %q", tasks[0].Prompt)
	}
	if tasks[0].Shape != "square" {
		t.Fatalf("unexpected shape: %q", tasks[0].Shape)
	}
	if tasks[0].TaskID != "task-000001" {
		t.Fatalf("expected task id from store, got %q", tasks[0].TaskID)
	}
	if len(taskStore.inbound) != 1 {
		t.Fatalf("expected 1 inbound message, got %d", len(taskStore.inbound))
	}
}

func TestHandleMessageFreeTextRepliesToOriginalMessage(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	msg := Message{
		MessageID: 555,
		From:      &User{ID: cfg.Snapshot().Telegram.AdminUserID},
		Chat:      Chat{ID: 1001},
		Text:      "来一张可爱猫娘",
	}
	bot.handleMessage(context.Background(), msg)

	body := transport.LastBody("/sendMessage")
	if body == "" {
		t.Fatalf("expected sendMessage payload")
	}
	if !strings.Contains(body, `"reply_to_message_id":555`) {
		t.Fatalf("expected reply_to_message_id in payload, body=%s", body)
	}
}

func TestHandleMessageFreeTextBlockedWhenConfigMissing(t *testing.T) {
	t.Parallel()

	bot, q, _, transport, cfg := newTestBot(t)
	if err := cfg.SetByPath("llm.api_key", ""); err != nil {
		t.Fatalf("clear llm.api_key: %v", err)
	}

	msg := Message{
		MessageID: 556,
		From:      &User{ID: cfg.Snapshot().Telegram.AdminUserID},
		Chat:      Chat{ID: 1001},
		Text:      "来一张可爱猫娘",
	}
	bot.handleMessage(context.Background(), msg)

	if len(q.Tasks()) != 0 {
		t.Fatalf("expected no task enqueued when config missing")
	}
	body := transport.LastBody("/sendMessage")
	if body == "" || !strings.Contains(body, "缺少绘图配置") || !strings.Contains(body, "llm.api_key") {
		t.Fatalf("expected missing config message, body=%s", body)
	}
}

func TestStartClearsPendingAction(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.setPendingAction(adminID, pendingSetLLMAPIKey)
	if bot.getPendingAction(adminID) != pendingSetLLMAPIKey {
		t.Fatalf("expected pending action set")
	}

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "/start",
	})

	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected pending action cleared")
	}
	body := transport.LastBody("/sendMessage")
	if body == "" || !strings.Contains(body, "Grimoire Bot") {
		t.Fatalf("expected start intro message, body=%s", body)
	}
}

func TestCommandLLMSendsSettingsMenu(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "/llm",
	})

	body := transport.LastBody("/sendMessage")
	if body == "" {
		t.Fatalf("expected llm menu payload")
	}
	if !strings.Contains(body, "LLM 设置") {
		t.Fatalf("expected llm menu text, body=%s", body)
	}
	if !strings.Contains(body, cbSetLLMModel) {
		t.Fatalf("expected llm model button, body=%s", body)
	}
}

func TestCommandNAISendsSettingsMenu(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "/nai",
	})

	body := transport.LastBody("/sendMessage")
	if body == "" {
		t.Fatalf("expected nai menu payload")
	}
	if !strings.Contains(body, "NAI 设置") {
		t.Fatalf("expected nai menu text, body=%s", body)
	}
	if !strings.Contains(body, cbSetNAIModel) {
		t.Fatalf("expected nai model button, body=%s", body)
	}
}

func TestCommandImgSendsSettingsMenu(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "/img",
	})

	body := transport.LastBody("/sendMessage")
	if body == "" {
		t.Fatalf("expected image menu payload")
	}
	if !strings.Contains(body, "绘图设置") {
		t.Fatalf("expected image menu text, body=%s", body)
	}
	if !strings.Contains(body, cbSetImageSize) {
		t.Fatalf("expected image size button, body=%s", body)
	}
}

func TestCallbackAndPendingInputUpdatesLLMBaseURL(t *testing.T) {
	t.Parallel()

	bot, _, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 200,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMBaseURL,
	})

	if bot.getPendingAction(adminID) != pendingSetLLMBaseURL {
		t.Fatalf("expected pendingSetLLMBaseURL")
	}

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "https://new.example.com/v1/",
	})

	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected pending action cleared")
	}
	if got := cfg.Snapshot().LLM.BaseURL; got != "https://new.example.com/v1" {
		t.Fatalf("unexpected llm base url: %q", got)
	}
}

func TestCallbackAndPendingInputUpdatesLLMModel(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusServiceUnavailable, `{"error":"busy"}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-model",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 205,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	if bot.getPendingAction(adminID) != pendingSetLLMModel {
		t.Fatalf("expected pendingSetLLMModel")
	}

	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: "gpt-5-mini",
	})

	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected pending action cleared")
	}
	if got := cfg.Snapshot().LLM.Model; got != "gpt-5-mini" {
		t.Fatalf("unexpected llm model: %q", got)
	}
}

func TestCallbackSetLLMModelShowsFetchedModelButtons(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusOK, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1-mini"}]}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-menu",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 260,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected no pending action")
	}
	body := transport.LastBody("/editMessageText")
	if body == "" {
		t.Fatalf("expected llm model menu payload")
	}
	if !strings.Contains(body, "请选择 LLM 模型") {
		t.Fatalf("expected llm model menu text, body=%s", body)
	}
	if !strings.Contains(body, cbLLMModelPickPrefix) {
		t.Fatalf("expected model pick callback, body=%s", body)
	}
	if !strings.Contains(body, cbLLMModelManualPrefix) {
		t.Fatalf("expected manual callback, body=%s", body)
	}
}

func TestCallbackPickLLMModelUpdatesConfig(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusOK, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1-mini"}]}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-menu-2",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 261,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	session, ok := getLLMModelSessionForTest(bot, adminID)
	if !ok {
		t.Fatalf("expected model session")
	}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-pick-1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 261,
			Chat:      Chat{ID: 1001},
		},
		Data: fmt.Sprintf("%s%s:%d", cbLLMModelPickPrefix, session.SessionID, 1),
	})

	if got := cfg.Snapshot().LLM.Model; got != "gpt-4.1-mini" {
		t.Fatalf("unexpected llm model: %q", got)
	}
	body := transport.LastBody("/editMessageText")
	if body == "" || !strings.Contains(body, "LLM 模型已更新为 gpt-4.1-mini") {
		t.Fatalf("expected model update notice, body=%s", body)
	}
}

func TestCallbackSetLLMModelFallbackToManualOnFetchFailure(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusBadGateway, `{"error":"gateway down"}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-fallback",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 262,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	if bot.getPendingAction(adminID) != pendingSetLLMModel {
		t.Fatalf("expected pendingSetLLMModel")
	}
	body := transport.LastBody("/sendMessage")
	if body == "" || !strings.Contains(body, "拉取模型列表失败") {
		t.Fatalf("expected fallback prompt, body=%s", body)
	}
}

func TestCallbackLLMModelPageSwitch(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusOK, `{"data":[{"id":"model-00"},{"id":"model-01"},{"id":"model-02"},{"id":"model-03"},{"id":"model-04"},{"id":"model-05"},{"id":"model-06"},{"id":"model-07"},{"id":"model-08"},{"id":"model-09"},{"id":"model-10"},{"id":"model-11"}]}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-page-open",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 263,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	session, ok := getLLMModelSessionForTest(bot, adminID)
	if !ok {
		t.Fatalf("expected model session")
	}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-page-next",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 263,
			Chat:      Chat{ID: 1001},
		},
		Data: fmt.Sprintf("%s%s:%d", cbLLMModelPagePrefix, session.SessionID, 1),
	})

	body := transport.LastBody("/editMessageText")
	if body == "" {
		t.Fatalf("expected page switch payload")
	}
	if !strings.Contains(body, "model-10") || !strings.Contains(body, "model-11") {
		t.Fatalf("expected second page models, body=%s", body)
	}
}

func TestCallbackLLMModelSessionExpired(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	transport.SetModelsResponse(http.StatusOK, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1-mini"}]}`)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-expire-open",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 264,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetLLMModel,
	})

	session, ok := getLLMModelSessionForTest(bot, adminID)
	if !ok {
		t.Fatalf("expected model session")
	}
	expireLLMModelSessionForTest(bot, adminID)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-llm-expire-pick",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 264,
			Chat:      Chat{ID: 1001},
		},
		Data: fmt.Sprintf("%s%s:%d", cbLLMModelPickPrefix, session.SessionID, 1),
	})

	if got := cfg.Snapshot().LLM.Model; got != "gpt-4o-mini" {
		t.Fatalf("expected model unchanged, got %q", got)
	}
	body := transport.LastBody("/answerCallbackQuery")
	if body == "" || !strings.Contains(body, "模型列表已过期") {
		t.Fatalf("expected expired callback message, body=%s", body)
	}
}

func TestCallbackSetImageSize(t *testing.T) {
	t.Parallel()

	bot, _, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb2",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 201,
			Chat:      Chat{ID: 1001},
		},
		Data: "size:portrait",
	})

	if got := cfg.Snapshot().Generation.ShapeDefault; got != "portrait" {
		t.Fatalf("unexpected shape default: %q", got)
	}
}

func TestCallbackAndPendingInputUpdatesArtist(t *testing.T) {
	t.Parallel()

	bot, _, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-artist",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 210,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetArtist,
	})

	if bot.getPendingAction(adminID) != pendingSetArtist {
		t.Fatalf("expected pendingSetArtist")
	}

	value := "artist:foo, artist:bar"
	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: value,
	})

	if got := cfg.Snapshot().Generation.Artist; got != value {
		t.Fatalf("unexpected artist value: %q", got)
	}
	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected pending action cleared")
	}
}

func TestCallbackAndPendingInputUpdatesNAIModel(t *testing.T) {
	t.Parallel()

	bot, _, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-nai-model",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 212,
			Chat:      Chat{ID: 1001},
		},
		Data: cbSetNAIModel,
	})

	if bot.getPendingAction(adminID) != pendingSetNAIModel {
		t.Fatalf("expected pendingSetNAIModel")
	}

	value := "nai-diffusion-4-5-curated-preview"
	bot.handleMessage(context.Background(), Message{
		From: &User{ID: adminID},
		Chat: Chat{ID: 1001},
		Text: value,
	})

	if got := cfg.Snapshot().NAI.Model; got != value {
		t.Fatalf("unexpected nai model: %q", got)
	}
	if bot.getPendingAction(adminID) != pendingNone {
		t.Fatalf("expected pending action cleared")
	}
}

func TestSetMyCommandsRegistersBotCommands(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, _ := newTestBot(t)
	if err := bot.setMyCommands(context.Background()); err != nil {
		t.Fatalf("setMyCommands error: %v", err)
	}

	payload := transport.LastBody("/setMyCommands")
	if payload == "" {
		t.Fatalf("expected captured setMyCommands payload")
	}
	for _, command := range []string{`"command":"start"`, `"command":"llm"`, `"command":"nai"`, `"command":"img"`} {
		if !strings.Contains(payload, command) {
			t.Fatalf("expected payload includes %s, body=%s", command, payload)
		}
	}
}

func TestBuildTaskActionMarkupForFailedStatus(t *testing.T) {
	t.Parallel()

	bot, _, _, _, _ := newTestBot(t)
	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000111",
		Prompt: "test prompt",
		Shape:  "square",
	})

	markup := bot.buildTaskActionMarkup(context.Background(), 1001, 0, "任务 task-000111\n状态: failed\n错误: timeout")
	if markup == nil {
		t.Fatalf("expected retry markup")
	}
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected keyboard layout: %+v", markup.InlineKeyboard)
	}
	btn := markup.InlineKeyboard[0][0]
	if btn.Text != "重新生成" {
		t.Fatalf("unexpected button text: %s", btn.Text)
	}
	if btn.CallbackData != "regen:task-000111" {
		t.Fatalf("unexpected callback data: %s", btn.CallbackData)
	}
}

func TestBuildTaskActionMarkupForProcessingShowsStop(t *testing.T) {
	t.Parallel()

	bot, _, _, _, _ := newTestBot(t)
	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000301",
		Prompt: "test prompt",
		Shape:  "square",
	})

	markup := bot.buildTaskActionMarkup(context.Background(), 1001, 0, "任务 task-000301\n状态: processing\n阶段: 提示词翻译")
	if markup == nil {
		t.Fatalf("expected stop markup")
	}
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected keyboard layout: %+v", markup.InlineKeyboard)
	}
	btn := markup.InlineKeyboard[0][0]
	if btn.Text != "停止生成" {
		t.Fatalf("unexpected button text: %s", btn.Text)
	}
	if btn.CallbackData != "stop:task-000301" {
		t.Fatalf("unexpected callback data: %s", btn.CallbackData)
	}
}

func TestBuildTaskActionMarkupForCompletedWithGalleryPaging(t *testing.T) {
	t.Parallel()

	bot, _, taskStore, _, _ := newTestBot(t)
	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000211",
		Prompt: "test prompt",
		Shape:  "square",
	})
	_ = taskStore.AppendGalleryItem(context.Background(), 1001, 500, "task-000201", "job-1", "/tmp/1.png", "任务 task-000201 完成\nJob ID: job-1", time.Now())
	_ = taskStore.AppendGalleryItem(context.Background(), 1001, 500, "task-000211", "job-2", "/tmp/2.png", "任务 task-000211 完成\nJob ID: job-2", time.Now())

	markup := bot.buildTaskActionMarkup(context.Background(), 1001, 500, "任务 task-000211 完成\nJob ID: job-2")
	if markup == nil {
		t.Fatalf("expected completed markup")
	}
	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("expected two rows, got %+v", markup.InlineKeyboard)
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != "regen:task-000211" {
		t.Fatalf("unexpected regen callback: %s", got)
	}
	if got := markup.InlineKeyboard[1][0].CallbackData; got != "gallery_prev:task-000211" {
		t.Fatalf("unexpected prev callback: %s", got)
	}
	if got := markup.InlineKeyboard[1][1].CallbackData; got != "gallery_next:task-000211" {
		t.Fatalf("unexpected next callback: %s", got)
	}
}

func TestRegenCallbackEnqueuesTask(t *testing.T) {
	t.Parallel()

	bot, q, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000005",
		Prompt: "retry me",
		Shape:  "landscape",
	})

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "retry-cb-1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 301,
			Chat:      Chat{ID: 1001},
		},
		Data: "regen:task-000005",
	})

	tasks := q.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Prompt != "retry me" {
		t.Fatalf("unexpected prompt: %q", tasks[0].Prompt)
	}
	if tasks[0].Shape != "landscape" {
		t.Fatalf("unexpected shape: %q", tasks[0].Shape)
	}
	if tasks[0].StatusMessageID != 301 {
		t.Fatalf("expected status message id 301, got %d", tasks[0].StatusMessageID)
	}
	if tasks[0].RetryOfTaskID != "task-000005" {
		t.Fatalf("expected retry_of_task_id set, got %q", tasks[0].RetryOfTaskID)
	}
}

func TestLegacyRetryCallbackStillWorks(t *testing.T) {
	t.Parallel()

	bot, q, _, _, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000006",
		Prompt: "legacy retry",
		Shape:  "portrait",
	})

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "legacy-retry-1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 302,
			Chat:      Chat{ID: 1001},
		},
		Data: "retry:task-000006",
	})

	tasks := q.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Prompt != "legacy retry" {
		t.Fatalf("unexpected prompt: %q", tasks[0].Prompt)
	}
}

func TestRegenCallbackBlockedWhenConfigMissing(t *testing.T) {
	t.Parallel()

	bot, q, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	if err := cfg.SetByPath("nai.model", ""); err != nil {
		t.Fatalf("clear nai.model: %v", err)
	}

	bot.rememberRetryTask(types.DrawTask{
		TaskID: "task-000007",
		Prompt: "retry me",
		Shape:  "square",
	})

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "retry-cb-missing",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 303,
			Chat:      Chat{ID: 1001},
		},
		Data: "regen:task-000007",
	})

	if len(q.Tasks()) != 0 {
		t.Fatalf("expected no task enqueued when config missing")
	}
	body := transport.LastBody("/sendMessage")
	if body == "" || !strings.Contains(body, "nai.model") {
		t.Fatalf("expected missing nai.model message, body=%s", body)
	}
}

func TestStopCallbackCancelsTask(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID
	ctrl := &mockTaskController{}
	bot.SetTaskController(ctrl)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "stop-cb-1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 399,
			Chat:      Chat{ID: 1001},
		},
		Data: "stop:task-000399",
	})

	if !ctrl.called {
		t.Fatalf("expected cancel controller called")
	}
	if ctrl.taskID != "task-000399" {
		t.Fatalf("unexpected canceled task id: %s", ctrl.taskID)
	}
	if body := transport.LastBody("/editMessageText"); body == "" || !strings.Contains(body, "cancelling") {
		t.Fatalf("expected cancelling editMessageText payload, body=%s", body)
	}
}

func TestGalleryNextCallbackSwitchesPhoto(t *testing.T) {
	t.Parallel()

	bot, _, taskStore, transport, cfg := newTestBot(t)
	adminID := cfg.Snapshot().Telegram.AdminUserID

	dir := t.TempDir()
	p1 := filepath.Join(dir, "1.png")
	p2 := filepath.Join(dir, "2.png")
	if err := os.WriteFile(p1, []byte("img1"), 0o600); err != nil {
		t.Fatalf("write p1: %v", err)
	}
	if err := os.WriteFile(p2, []byte("img2"), 0o600); err != nil {
		t.Fatalf("write p2: %v", err)
	}

	if err := taskStore.CreateTask(context.Background(), types.DrawTask{TaskID: "task-000001", Prompt: "p1", Shape: "square"}); err != nil {
		t.Fatalf("create task1: %v", err)
	}
	if err := taskStore.CreateTask(context.Background(), types.DrawTask{TaskID: "task-000002", Prompt: "p2", Shape: "square"}); err != nil {
		t.Fatalf("create task2: %v", err)
	}
	if err := taskStore.AppendGalleryItem(context.Background(), 1001, 777, "task-000001", "job-1", p1, "任务 task-000001 完成\nJob ID: job-1", time.Now()); err != nil {
		t.Fatalf("append gallery1: %v", err)
	}
	if err := taskStore.AppendGalleryItem(context.Background(), 1001, 777, "task-000002", "job-2", p2, "任务 task-000002 完成\nJob ID: job-2", time.Now()); err != nil {
		t.Fatalf("append gallery2: %v", err)
	}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "gallery-next-1",
		From: User{ID: adminID},
		Message: &Message{
			MessageID: 777,
			Chat:      Chat{ID: 1001},
		},
		Data: "gallery_next:task-000001",
	})

	body := transport.LastBody("/editMessageMedia")
	if body == "" {
		t.Fatalf("expected editMessageMedia payload")
	}
	if !strings.Contains(body, "task-000002") {
		t.Fatalf("expected switched to task-000002 caption, body=%s", body)
	}
}

func TestEditTextFallsBackToCaptionOnPhotoMessage(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, _ := newTestBot(t)
	transport.SetEditMessageTextResponse(http.StatusBadRequest, `{"ok":false,"description":"Bad Request: there is no text in the message to edit"}`)

	if err := bot.EditText(context.Background(), 1001, 222, "任务 task-000001\n状态: queued"); err != nil {
		t.Fatalf("EditText fallback error: %v", err)
	}
	if body := transport.LastBody("/editMessageCaption"); body == "" {
		t.Fatalf("expected editMessageCaption fallback payload")
	}
}

func TestNewTelegramHTTPClientWithProxy(t *testing.T) {
	t.Parallel()

	client := newTelegramHTTPClient("http://127.0.0.1:7890", slog.New(slog.NewTextHandler(io.Discard, nil)))
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.telegram.org", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("transport.Proxy error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://127.0.0.1:7890" {
		t.Fatalf("unexpected proxy url: %v", proxyURL)
	}
	if client.Timeout != 60*time.Second {
		t.Fatalf("unexpected timeout: %s", client.Timeout)
	}
}

func TestNewDirectHTTPClientDisablesProxy(t *testing.T) {
	t.Parallel()

	client := newDirectHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatalf("expected direct transport with no proxy, got non-nil proxy function")
	}
	if client.Timeout != 60*time.Second {
		t.Fatalf("unexpected timeout: %s", client.Timeout)
	}
}

func TestIsAdminUser(t *testing.T) {
	t.Parallel()

	if !isAdminUser(100, 100) {
		t.Fatalf("expected true")
	}
	if isAdminUser(100, 101) {
		t.Fatalf("expected false")
	}
	if isAdminUser(0, 0) {
		t.Fatalf("expected false when admin id invalid")
	}
}

func TestEditPhotoUsesEditMessageMedia(t *testing.T) {
	t.Parallel()

	bot, _, _, transport, _ := newTestBot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "x.png")
	if err := os.WriteFile(path, []byte("png-bytes"), 0o600); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	if err := bot.EditPhoto(context.Background(), 1001, 222, path, "done"); err != nil {
		t.Fatalf("EditPhoto error: %v", err)
	}

	body := transport.LastBody("/editMessageMedia")
	if body == "" {
		t.Fatalf("expected editMessageMedia payload")
	}
	if !strings.Contains(body, "attach://photo") {
		t.Fatalf("expected attach://photo in payload")
	}
}

func newTestBot(t *testing.T) (*Bot, *mockQueue, *mockTaskStore, *mockTelegramTransport, *config.Manager) {
	t.Helper()
	ensureTestTelegramEnv()

	dbPath := filepath.Join(t.TempDir(), "grimoire.db")
	cfgManager, err := config.NewManager(dbPath)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = cfgManager.Close()
	})
	if err := cfgManager.SetByPath("llm.base_url", "https://api.openai.com/v1"); err != nil {
		t.Fatalf("set llm.base_url: %v", err)
	}
	if err := cfgManager.SetByPath("llm.api_key", "llm-key"); err != nil {
		t.Fatalf("set llm.api_key: %v", err)
	}
	if err := cfgManager.SetByPath("llm.model", "gpt-4o-mini"); err != nil {
		t.Fatalf("set llm.model: %v", err)
	}
	if err := cfgManager.SetByPath("nai.api_key", "nai-key"); err != nil {
		t.Fatalf("set nai.api_key: %v", err)
	}
	if err := cfgManager.SetByPath("nai.model", "nai-model"); err != nil {
		t.Fatalf("set nai.model: %v", err)
	}

	q := &mockQueue{}
	taskStore := newMockTaskStore()
	transport := newMockTelegramTransport()

	bot := NewBot(cfgManager, q, taskStore, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bot.httpClient = &http.Client{Transport: transport}
	bot.llmHTTPClient = &http.Client{Transport: transport}
	return bot, q, taskStore, transport, cfgManager
}

var telegramTestEnvOnce sync.Once

func ensureTestTelegramEnv() {
	telegramTestEnvOnce.Do(func() {
		_ = os.Setenv(config.EnvTelegramBotToken, "token")
		_ = os.Setenv(config.EnvTelegramAdminUserID, "1")
		_ = os.Setenv(config.EnvTelegramProxyURL, "")
	})
}

func getLLMModelSessionForTest(bot *Bot, userID int64) (llmModelSession, bool) {
	bot.llmModelMu.Lock()
	defer bot.llmModelMu.Unlock()
	session, ok := bot.llmModelSessions[userID]
	return session, ok
}

func expireLLMModelSessionForTest(bot *Bot, userID int64) {
	bot.llmModelMu.Lock()
	defer bot.llmModelMu.Unlock()
	session, ok := bot.llmModelSessions[userID]
	if !ok {
		return
	}
	session.ExpiresAt = time.Now().Add(-time.Minute)
	bot.llmModelSessions[userID] = session
}

type inboundRecord struct {
	chatID    int64
	userID    int64
	messageID int64
	text      string
	createdAt time.Time
}

type mockTaskStore struct {
	mu        sync.Mutex
	next      int
	inbound   []inboundRecord
	tasks     map[string]types.DrawTask
	recover   []types.DrawTask
	galleries []store.GalleryItem
}

func newMockTaskStore() *mockTaskStore {
	return &mockTaskStore{
		tasks: make(map[string]types.DrawTask),
	}
}

func (m *mockTaskStore) Init(ctx context.Context) error {
	return nil
}

func (m *mockTaskStore) NextTaskID(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	return fmt.Sprintf("task-%06d", m.next), nil
}

func (m *mockTaskStore) CreateInboundMessage(ctx context.Context, chatID, userID, messageID int64, text string, createdAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inbound = append(m.inbound, inboundRecord{
		chatID:    chatID,
		userID:    userID,
		messageID: messageID,
		text:      text,
		createdAt: createdAt,
	})
	return nil
}

func (m *mockTaskStore) CreateTask(ctx context.Context, task types.DrawTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.TaskID] = task
	return nil
}

func (m *mockTaskStore) UpdateTaskStatus(ctx context.Context, taskID string, status string, stage string, errMsg string) error {
	return nil
}

func (m *mockTaskStore) SetTaskJobID(ctx context.Context, taskID string, jobID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok {
		return store.ErrNotFound
	}
	task.ResumeJobID = jobID
	m.tasks[taskID] = task
	return nil
}

func (m *mockTaskStore) SaveTaskResult(ctx context.Context, taskID string, jobID string, filePath string, completedAt time.Time) error {
	return nil
}

func (m *mockTaskStore) GetTaskByID(ctx context.Context, taskID string) (types.DrawTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[taskID]
	if !ok {
		return types.DrawTask{}, store.ErrNotFound
	}
	return task, nil
}

func (m *mockTaskStore) ListRecoverableTasks(ctx context.Context) ([]types.DrawTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]types.DrawTask, len(m.recover))
	copy(out, m.recover)
	return out, nil
}

func (m *mockTaskStore) AppendGalleryItem(ctx context.Context, chatID, messageID int64, taskID, jobID, filePath, caption string, createdAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.galleries = append(m.galleries, store.GalleryItem{
		ChatID:    chatID,
		MessageID: messageID,
		TaskID:    taskID,
		JobID:     jobID,
		FilePath:  filePath,
		Caption:   caption,
		CreatedAt: createdAt,
	})
	return nil
}

func (m *mockTaskStore) ListGalleryItems(ctx context.Context, chatID, messageID int64) ([]store.GalleryItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.GalleryItem, 0)
	for _, item := range m.galleries {
		if item.ChatID == chatID && item.MessageID == messageID {
			out = append(out, item)
		}
	}
	return out, nil
}

type mockQueue struct {
	mu    sync.Mutex
	tasks []types.DrawTask
	seq   int
}

type mockTaskController struct {
	called bool
	taskID string
}

func (m *mockTaskController) CancelTask(taskID string) bool {
	m.called = true
	m.taskID = taskID
	return true
}

func (q *mockQueue) Enqueue(task types.DrawTask) (string, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seq++
	taskID := task.TaskID
	if taskID == "" {
		taskID = fmt.Sprintf("task-%06d", q.seq)
	}
	task.TaskID = taskID
	q.tasks = append(q.tasks, task)
	return taskID, len(q.tasks)
}

func (q *mockQueue) Stats() types.QueueStats {
	return types.QueueStats{}
}

func (q *mockQueue) Tasks() []types.DrawTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]types.DrawTask, len(q.tasks))
	copy(out, q.tasks)
	return out
}

type mockTelegramTransport struct {
	mu             sync.Mutex
	messageID      int64
	bodies         map[string][]string
	editTextStatus int
	editTextBody   string
	modelsStatus   int
	modelsBody     string
}

func newMockTelegramTransport() *mockTelegramTransport {
	return &mockTelegramTransport{bodies: make(map[string][]string)}
}

func (m *mockTelegramTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		body = string(bodyBytes)
	}
	path := req.URL.Path

	m.mu.Lock()
	for _, suffix := range []string{"/sendMessage", "/editMessageText", "/editMessageCaption", "/editMessageMedia", "/answerCallbackQuery", "/setMyCommands", "/models"} {
		if strings.HasSuffix(path, suffix) {
			m.bodies[suffix] = append(m.bodies[suffix], body)
		}
	}
	editTextStatus := m.editTextStatus
	editTextBody := m.editTextBody
	modelsStatus := m.modelsStatus
	modelsBody := m.modelsBody
	m.mu.Unlock()

	switch {
	case strings.HasSuffix(path, "/sendMessage"):
		m.mu.Lock()
		m.messageID++
		id := m.messageID
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, fmt.Sprintf(`{"ok":true,"result":{"message_id":%d}}`, id)), nil
	case strings.HasSuffix(path, "/editMessageText"):
		if editTextStatus != 0 {
			if editTextBody == "" {
				editTextBody = `{"ok":false,"description":"Bad Request: there is no text in the message to edit"}`
			}
			return jsonResponse(editTextStatus, editTextBody), nil
		}
		return jsonResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
	case strings.HasSuffix(path, "/editMessageCaption"):
		return jsonResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
	case strings.HasSuffix(path, "/editMessageMedia"):
		return jsonResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
	case strings.HasSuffix(path, "/answerCallbackQuery"):
		return jsonResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
	case strings.HasSuffix(path, "/setMyCommands"):
		return jsonResponse(http.StatusOK, `{"ok":true,"result":true}`), nil
	case strings.HasSuffix(path, "/models"):
		if modelsStatus != 0 {
			if modelsBody == "" {
				modelsBody = `{"data":[]}`
			}
			return jsonResponse(modelsStatus, modelsBody), nil
		}
		if strings.TrimSpace(modelsBody) != "" {
			return jsonResponse(http.StatusOK, modelsBody), nil
		}
		return jsonResponse(http.StatusOK, `{"data":[{"id":"gpt-4o-mini"}]}`), nil
	default:
		return jsonResponse(http.StatusOK, `{"ok":true,"result":[]}`), nil
	}
}

func (m *mockTelegramTransport) LastBody(pathSuffix string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.bodies[pathSuffix]
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

func (m *mockTelegramTransport) SetEditMessageTextResponse(status int, body string) {
	m.mu.Lock()
	m.editTextStatus = status
	m.editTextBody = body
	m.mu.Unlock()
}

func (m *mockTelegramTransport) SetModelsResponse(status int, body string) {
	m.mu.Lock()
	m.modelsStatus = status
	m.modelsBody = body
	m.mu.Unlock()
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
