package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	preferencesapp "grimoire/internal/app/preferences"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type chatServiceMock struct {
	commands []chatapp.HandleTextCommand
	result   chatapp.HandleTextResult
	err      error
}

func (m *chatServiceMock) HandleText(_ context.Context, command chatapp.HandleTextCommand) (chatapp.HandleTextResult, error) {
	m.commands = append(m.commands, command)
	if m.err != nil {
		return chatapp.HandleTextResult{}, m.err
	}
	if strings.TrimSpace(m.result.Reply) == "" && strings.TrimSpace(m.result.CreatedTaskID) == "" {
		m.result = chatapp.HandleTextResult{
			SessionID: "session-1",
			Reply:     "请再补充一点构图方向。",
		}
	}
	return m.result, nil
}

type accessServiceMock struct {
	commands []accessapp.CheckCommand
	decision accessapp.Decision
	err      error
}

func (m *accessServiceMock) Check(_ context.Context, command accessapp.CheckCommand) (accessapp.Decision, error) {
	m.commands = append(m.commands, command)
	if m.err != nil {
		return accessapp.Decision{}, m.err
	}
	return m.decision, nil
}

type sessionServiceMock struct {
	commands []sessionapp.CreateNewCommand
	result   domainsession.Session
	err      error
}

func (m *sessionServiceMock) CreateNew(_ context.Context, command sessionapp.CreateNewCommand) (domainsession.Session, error) {
	m.commands = append(m.commands, command)
	if m.err != nil {
		return domainsession.Session{}, m.err
	}
	if strings.TrimSpace(m.result.ID) == "" {
		m.result = domainsession.Session{ID: "session-2", UserID: command.UserID}
	}
	return m.result, nil
}

type taskServiceMock struct {
	commands               []taskapp.CreateCommand
	stops                  []taskapp.StopCommand
	prompts                []taskapp.GetPromptCommand
	retryTranslateCommands []taskapp.RetryCommand
	retryDrawCommands      []taskapp.RetryCommand
	result                 domaintask.Task
	err                    error
	stopErr                error
	prompt                 taskapp.PromptDetails
	promptSet              bool
	promptErr              error
	retryTranslateErr      error
	retryDrawErr           error
}

func (m *taskServiceMock) Create(_ context.Context, command taskapp.CreateCommand) (domaintask.Task, error) {
	m.commands = append(m.commands, command)
	if m.err != nil {
		return domaintask.Task{}, m.err
	}
	if strings.TrimSpace(m.result.ID) == "" {
		m.result = domaintask.Task{ID: "task-1"}
	}
	return m.result, nil
}

func (m *taskServiceMock) Stop(_ context.Context, command taskapp.StopCommand) (domaintask.Task, error) {
	m.stops = append(m.stops, command)
	if m.stopErr != nil {
		return domaintask.Task{}, m.stopErr
	}
	if strings.TrimSpace(m.result.ID) == "" {
		m.result = domaintask.Task{ID: command.TaskID, Status: domaintask.StatusStopped}
	}
	return m.result, nil
}

func (m *taskServiceMock) GetPrompt(_ context.Context, command taskapp.GetPromptCommand) (taskapp.PromptDetails, error) {
	m.prompts = append(m.prompts, command)
	if m.promptErr != nil {
		return taskapp.PromptDetails{}, m.promptErr
	}
	if !m.promptSet {
		return taskapp.PromptDetails{Prompt: "masterpiece, moonlit_girl"}, nil
	}
	return m.prompt, nil
}

func (m *taskServiceMock) RetryTranslate(_ context.Context, command taskapp.RetryCommand) (domaintask.Task, error) {
	m.retryTranslateCommands = append(m.retryTranslateCommands, command)
	if m.retryTranslateErr != nil {
		return domaintask.Task{}, m.retryTranslateErr
	}
	if strings.TrimSpace(m.result.ID) == "" {
		m.result = domaintask.Task{ID: "task-2"}
	}
	return m.result, nil
}

func (m *taskServiceMock) RetryDraw(_ context.Context, command taskapp.RetryCommand) (domaintask.Task, error) {
	m.retryDrawCommands = append(m.retryDrawCommands, command)
	if m.retryDrawErr != nil {
		return domaintask.Task{}, m.retryDrawErr
	}
	if strings.TrimSpace(m.result.ID) == "" {
		m.result = domaintask.Task{ID: "task-3"}
	}
	return m.result, nil
}

type preferenceServiceMock struct {
	pref         domainpreferences.Preference
	updateModes  []preferencesapp.UpdateModeCommand
	updateModeErr error
}

type balanceServiceMock struct {
	balance domainnai.AccountBalance
	err     error
}

func (m *preferenceServiceMock) Get(_ context.Context, _ preferencesapp.GetCommand) (domainpreferences.Preference, error) {
	if !m.pref.Shape.Valid() {
		m.pref = domainpreferences.DefaultPreference()
	}
	return m.pref, nil
}

func (m *preferenceServiceMock) UpdateShape(_ context.Context, command preferencesapp.UpdateShapeCommand) (domainpreferences.Preference, error) {
	m.pref = domainpreferences.DefaultPreference()
	m.pref.Shape = command.Shape
	return m.pref, nil
}

func (m *preferenceServiceMock) UpdateArtists(_ context.Context, command preferencesapp.UpdateArtistsCommand) (domainpreferences.Preference, error) {
	m.pref = domainpreferences.DefaultPreference()
	m.pref.Artists = strings.TrimSpace(command.Artists)
	return m.pref, nil
}

func (m *preferenceServiceMock) ClearArtists(context.Context, preferencesapp.ClearArtistsCommand) (domainpreferences.Preference, error) {
	m.pref = domainpreferences.DefaultPreference()
	return m.pref, nil
}

func (m *preferenceServiceMock) UpdateMode(_ context.Context, command preferencesapp.UpdateModeCommand) (domainpreferences.Preference, error) {
	m.updateModes = append(m.updateModes, command)
	if m.updateModeErr != nil {
		return domainpreferences.Preference{}, m.updateModeErr
	}
	if !m.pref.Shape.Valid() {
		m.pref = domainpreferences.DefaultPreference()
	}
	if err := m.pref.SetMode(command.Mode); err != nil {
		return domainpreferences.Preference{}, err
	}
	return m.pref, nil
}

func (m *balanceServiceMock) GetBalance(_ context.Context) (domainnai.AccountBalance, error) {
	if m.err != nil {
		return domainnai.AccountBalance{}, m.err
	}
	return m.balance, nil
}

func newTestBot(t *testing.T) (*Bot, *accessServiceMock, *chatServiceMock, *taskServiceMock, *preferenceServiceMock, *balanceServiceMock, *bytes.Buffer) {
	t.Helper()
	return newTestBotWithLogger(t, nil)
}

func newTestBotWithLogger(t *testing.T, logger *slog.Logger) (*Bot, *accessServiceMock, *chatServiceMock, *taskServiceMock, *preferenceServiceMock, *balanceServiceMock, *bytes.Buffer) {
	t.Helper()
	buffer := &bytes.Buffer{}
	bot := NewBot(config.Config{
		Telegram: config.Telegram{
			BotToken:    "token",
			AdminUserID: 1,
		},
	}, logger)
	bot.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			buffer.WriteString(req.URL.Path)
			buffer.WriteByte('\n')
			buffer.Write(body)
			buffer.WriteString("\n---\n")

			response := `{"ok":true,"result":{"message_id":1}}`
			if strings.Contains(req.URL.Path, "getUpdates") {
				response = `{"ok":true,"result":[]}`
			} else if strings.Contains(req.URL.Path, "answerCallbackQuery") || strings.Contains(req.URL.Path, "editMessageText") || strings.Contains(req.URL.Path, "deleteMessage") || strings.Contains(req.URL.Path, "sendPhoto") || strings.Contains(req.URL.Path, "setMyCommands") {
				response = `{"ok":true,"description":""}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(response)),
			}, nil
		}),
	}

	accessService := &accessServiceMock{decision: accessapp.Decision{Allowed: true}}
	chatService := &chatServiceMock{}
	sessionService := &sessionServiceMock{}
	taskService := &taskServiceMock{}
	prefService := &preferenceServiceMock{}
	balanceService := &balanceServiceMock{
		balance: domainnai.AccountBalance{
			PurchasedTrainingSteps: 456,
			FixedTrainingStepsLeft: 23,
			TrialImagesLeft:        12,
			SubscriptionTier:       1,
			SubscriptionActive:     true,
		},
	}
	bot.SetAccessService(accessService)
	bot.SetChatService(chatService)
	bot.SetSessionService(sessionService)
	bot.SetTaskService(taskService)
	bot.SetPreferenceService(prefService)
	bot.SetBalanceService(balanceService)
	return bot, accessService, chatService, taskService, prefService, balanceService, buffer
}

func TestHandleMessageUsesChatService(t *testing.T) {
	bot, accessService, chatService, _, _, _, buffer := newTestBot(t)
	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "画一个月下的少女",
	})

	if len(chatService.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(chatService.commands))
	}
	if len(accessService.commands) != 1 || accessService.commands[0].TelegramID != "1" {
		t.Fatalf("unexpected access commands: %#v", accessService.commands)
	}
	if chatService.commands[0].UserID != "1" {
		t.Fatalf("unexpected user id: %q", chatService.commands[0].UserID)
	}
	if chatService.commands[0].MessageID != "10" {
		t.Fatalf("unexpected message id: %q", chatService.commands[0].MessageID)
	}
	if chatService.commands[0].Text != "画一个月下的少女" {
		t.Fatalf("unexpected text: %q", chatService.commands[0].Text)
	}
	if !strings.Contains(buffer.String(), `请再补充一点构图方向。`) {
		t.Fatalf("expected chat reply to be sent, got %s", buffer.String())
	}
	if strings.Contains(buffer.String(), `待确认 request`) {
		t.Fatalf("did not expect pending request message, got %s", buffer.String())
	}
	if strings.Contains(buffer.String(), `"reply_to_message_id":10`) {
		t.Fatalf("did not expect chat reply to quote the incoming message, got %s", buffer.String())
	}
}

func TestHandleMessageLogsInboundAndOutboundLifecycle(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	bot, _, chatService, _, _, _, _ := newTestBotWithLogger(t, logger)
	chatService.result = chatapp.HandleTextResult{
		SessionID: "session-1",
		Reply:     "开始绘图吧。",
	}

	bot.handleMessage(context.Background(), Message{
		MessageID: 15,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "开始绘图",
	})

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"telegram inbound message received",
		"chat_id=100",
		"telegram_user_id=1",
		"message_id=15",
		"text=开始绘图",
		"telegram outbound chat reply sent",
		"reply=开始绘图吧。",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
}

func TestHandleMessageLogsNoToolCallScenario(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	bot, _, chatService, _, _, _, _ := newTestBotWithLogger(t, logger)
	chatService.result = chatapp.HandleTextResult{
		SessionID: "session-1",
		Reply:     "已开始绘图，请稍等。",
	}

	bot.handleMessage(context.Background(), Message{
		MessageID: 16,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "开始绘图",
	})

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "telegram outbound chat reply sent") {
		t.Fatalf("expected outbound reply log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "created_task_id=\"\"") {
		t.Fatalf("expected empty created_task_id in logs, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "reply=已开始绘图，请稍等。") {
		t.Fatalf("expected reply content in logs, got %s", logOutput)
	}
}

func TestRouteUpdateDispatchesMessage(t *testing.T) {
	bot, _, chatService, _, _, _, _ := newTestBot(t)
	bot.routeUpdate(context.Background(), Update{
		Message: &Message{
			MessageID: 10,
			From:      &User{ID: 1},
			Chat:      Chat{ID: 100},
			Text:      "画一个月下的少女",
		},
	})

	if len(chatService.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(chatService.commands))
	}
}

func TestHandleMessageSendsErrorWhenChatServiceFails(t *testing.T) {
	bot, _, chatService, _, _, _, buffer := newTestBot(t)
	chatService.err = errors.New("boom")

	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "画一个月下的少女",
	})

	if !strings.Contains(buffer.String(), "处理消息失败: boom") {
		t.Fatalf("expected chat error message, got %s", buffer.String())
	}
}

func TestHandleStartCommandKeepsCommandFlow(t *testing.T) {
	bot, _, chatService, _, _, _, buffer := newTestBot(t)
	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/start",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	for _, expected := range []string{
		"发送文本即可进入需求对话，确认后再开始绘图。",
		"发送 /new 可新建一个会话并重新开始需求收敛。",
		"发送 /fast 可切换到快速模式，后续消息将直接开始绘图。",
		"发送 /expert 可切换到专家模式，恢复需求对话流程。",
	} {
		if !strings.Contains(buffer.String(), expected) {
			t.Fatalf("expected updated start text to include %q, got %s", expected, buffer.String())
		}
	}
}

func TestHandleImgCommandSendsImageMenu(t *testing.T) {
	bot, _, chatService, _, prefService, _, buffer := newTestBot(t)
	prefService.pref = domainpreferences.DefaultPreference()
	prefService.pref.Artists = "artist:foo"

	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/img",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	logOutput := buffer.String()
	for _, expected := range []string{
		"sendMessage",
		"全局绘图偏好",
		"当前尺寸: Small Square (640x640)",
		"当前画师串: artist:foo",
		`"callback_data":"request:shape:small-portrait"`,
		`"callback_data":"request:artists:set"`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in output, got %s", expected, logOutput)
		}
	}
}

func TestHandleNewCommandCreatesNewSession(t *testing.T) {
	bot, _, chatService, _, _, _, buffer := newTestBot(t)

	bot.handleMessage(context.Background(), Message{
		MessageID: 17,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/new",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	sessionService, ok := bot.sessionService.(*sessionServiceMock)
	if !ok {
		t.Fatalf("expected session service mock, got %#v", bot.sessionService)
	}
	if len(sessionService.commands) != 1 {
		t.Fatalf("expected one create new command, got %d", len(sessionService.commands))
	}
	if sessionService.commands[0].UserID != "1" {
		t.Fatalf("unexpected create new user id: %q", sessionService.commands[0].UserID)
	}
	if !strings.Contains(buffer.String(), buildNewSessionText()) {
		t.Fatalf("expected new session text, got %s", buffer.String())
	}
}

func TestHandleNewCommandClearsPendingArtists(t *testing.T) {
	bot, _, _, _, _, _, _ := newTestBot(t)
	bot.setPendingArtists()

	bot.handleMessage(context.Background(), Message{
		MessageID: 18,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/new",
	})

	if bot.isPendingArtists() {
		t.Fatal("expected pending artists cleared after /new")
	}
}

func TestHandleFastCommandUpdatesMode(t *testing.T) {
	bot, _, chatService, _, prefService, _, buffer := newTestBot(t)

	bot.handleMessage(context.Background(), Message{
		MessageID: 19,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/fast",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	if len(prefService.updateModes) != 1 {
		t.Fatalf("expected one update mode command, got %d", len(prefService.updateModes))
	}
	if prefService.updateModes[0].Mode != domainpreferences.ModeFast {
		t.Fatalf("unexpected mode: %q", prefService.updateModes[0].Mode)
	}
	if !strings.Contains(buffer.String(), buildFastModeText()) {
		t.Fatalf("expected fast mode text, got %s", buffer.String())
	}
}

func TestHandleExpertCommandUpdatesMode(t *testing.T) {
	bot, _, chatService, _, prefService, _, buffer := newTestBot(t)

	bot.handleMessage(context.Background(), Message{
		MessageID: 20,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/expert",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	if len(prefService.updateModes) != 1 {
		t.Fatalf("expected one update mode command, got %d", len(prefService.updateModes))
	}
	if prefService.updateModes[0].Mode != domainpreferences.ModeExpert {
		t.Fatalf("unexpected mode: %q", prefService.updateModes[0].Mode)
	}
	if !strings.Contains(buffer.String(), buildExpertModeText()) {
		t.Fatalf("expected expert mode text, got %s", buffer.String())
	}
}

func TestHandleFastCommandClearsPendingArtists(t *testing.T) {
	bot, _, _, _, _, _, _ := newTestBot(t)
	bot.setPendingArtists()

	bot.handleMessage(context.Background(), Message{
		MessageID: 21,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/fast",
	})

	if bot.isPendingArtists() {
		t.Fatal("expected pending artists cleared after /fast")
	}
}

func TestImgCallbackUpdatesShape(t *testing.T) {
	bot, _, _, _, prefService, _, buffer := newTestBot(t)
	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-1",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 20,
			Chat:      Chat{ID: 100},
		},
		Data: requestShapeCallback(domaindraw.ShapePortrait),
	})

	if prefService.pref.Shape != domaindraw.ShapePortrait {
		t.Fatalf("unexpected shape: %s", prefService.pref.Shape)
	}
	if !strings.Contains(buffer.String(), "editMessageText") {
		t.Fatalf("expected edit message request, got %s", buffer.String())
	}
}

func TestHandleCallbackQueryRejectsInvalidData(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)
	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-invalid",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 21,
			Chat:      Chat{ID: 100},
		},
		Data: "unknown:callback",
	})

	logOutput := buffer.String()
	if !strings.Contains(logOutput, "answerCallbackQuery") {
		t.Fatalf("expected answerCallbackQuery request, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `"text":"操作无效"`) {
		t.Fatalf("expected invalid callback text, got %s", logOutput)
	}
}

func TestPendingArtistFlow(t *testing.T) {
	bot, _, _, _, prefService, _, _ := newTestBot(t)
	bot.setPendingArtists()
	bot.handleMessage(context.Background(), Message{
		MessageID: 11,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      " artist:foo ",
	})

	if prefService.pref.Artists != "artist:foo" {
		t.Fatalf("unexpected artist: %q", prefService.pref.Artists)
	}
	if bot.isPendingArtists() {
		t.Fatal("expected pending artist cleared")
	}
}

func TestResultMessageHasNoRetryButtons(t *testing.T) {
	pref := domainpreferences.DefaultPreference()
	text := buildImageMenuText("", pref)
	if strings.Contains(text, "重新生成") {
		t.Fatalf("unexpected retry text: %s", text)
	}
	markup, err := json.Marshal(imageMenuMarkup())
	if err != nil {
		t.Fatalf("marshal markup: %v", err)
	}
	if strings.Contains(string(markup), "retry") {
		t.Fatalf("unexpected retry callback: %s", string(markup))
	}
	if strings.Contains(string(markup), "\"img:") {
		t.Fatalf("unexpected legacy callback protocol: %s", string(markup))
	}
}

func TestSendPhotoIncludesReplyToMessage(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if err := bot.SendPhoto(context.Background(), 100, 20, "task.png", "", []byte("png")); err != nil {
		t.Fatalf("send photo: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, "sendPhoto") {
		t.Fatalf("expected sendPhoto request, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "name=\"reply_to_message_id\"") {
		t.Fatalf("expected reply_to_message_id field, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "\r\n20\r\n") && !strings.Contains(logOutput, "\n20\n") {
		t.Fatalf("expected reply target 20, got %s", logOutput)
	}
}

func TestSendTextUsesSendMessageEndpoint(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if _, err := bot.SendText(context.Background(), 100, 20, "hello"); err != nil {
		t.Fatalf("send text: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, "sendMessage") || !strings.Contains(logOutput, `"text":"hello"`) {
		t.Fatalf("expected sendMessage payload, got %s", logOutput)
	}
}

func TestEditTextUsesEditMessageTextEndpoint(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if err := bot.EditText(context.Background(), 100, 21, "updated"); err != nil {
		t.Fatalf("edit text: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, "editMessageText") || !strings.Contains(logOutput, `"text":"updated"`) {
		t.Fatalf("expected editMessageText payload, got %s", logOutput)
	}
}

func TestSendPhotoMessageUsesSendPhotoEndpoint(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if _, err := bot.SendPhotoMessage(context.Background(), 100, 20, "task.png", "", []byte("png")); err != nil {
		t.Fatalf("send photo message: %v", err)
	}

	if !strings.Contains(buffer.String(), "sendPhoto") {
		t.Fatalf("expected sendPhoto request, got %s", buffer.String())
	}
}

func TestSendProgressTextIncludesStopButton(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if _, err := bot.SendProgressText(context.Background(), 100, 20, "已入队", "task-1"); err != nil {
		t.Fatalf("send progress text: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, `"callback_data":"task:stop:task-1"`) {
		t.Fatalf("expected stop callback in output, got %s", logOutput)
	}
}

func TestEditProgressTextIncludesStopButton(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if err := bot.EditProgressText(context.Background(), 100, 20, "正在绘图", "task-1"); err != nil {
		t.Fatalf("edit progress text: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, `"callback_data":"task:stop:task-1"`) {
		t.Fatalf("expected stop callback in edit output, got %s", logOutput)
	}
}

func TestSendResultPhotoMessageIncludesResultButtons(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if _, err := bot.SendResultPhotoMessage(context.Background(), 100, 20, "task.png", "", []byte("png"), "task-1"); err != nil {
		t.Fatalf("send result photo: %v", err)
	}

	logOutput := buffer.String()
	for _, expected := range []string{
		`task:prompt:task-1`,
		`task:retry:translate:task-1`,
		`task:retry:draw:task-1`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in output, got %s", expected, logOutput)
		}
	}
}

func TestDeleteMessageUsesDeleteMessageEndpoint(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if err := bot.DeleteMessage(context.Background(), 100, 20); err != nil {
		t.Fatalf("delete message: %v", err)
	}

	logOutput := buffer.String()
	if !strings.Contains(logOutput, "deleteMessage") {
		t.Fatalf("expected deleteMessage request, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `"message_id":20`) {
		t.Fatalf("expected message id payload, got %s", logOutput)
	}
}

func TestHandleBalanceCommandSendsBalance(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)
	bot.handleMessage(context.Background(), Message{
		MessageID: 12,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/balance",
	})

	logOutput := buffer.String()
	for _, expected := range []string{
		"sendMessage",
		`"text":"NAI 余额`,
		`购买余额: 456`,
		`月度余额: 23`,
		`试用剩余图片: 12`,
		`订阅: 已激活 (tier=1)`,
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in output, got %s", expected, logOutput)
		}
	}
}

func TestHandleBalanceCommandSendsError(t *testing.T) {
	bot, _, _, _, _, balanceService, buffer := newTestBot(t)
	balanceService.err = errors.New("boom")

	bot.handleMessage(context.Background(), Message{
		MessageID: 12,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/balance",
	})

	if !strings.Contains(buffer.String(), `查询余额失败: boom`) {
		t.Fatalf("expected balance error in output, got %s", buffer.String())
	}
}

func TestSetMyCommandsIncludesModeAndBalanceCommands(t *testing.T) {
	bot, _, _, _, _, _, buffer := newTestBot(t)

	if err := bot.setMyCommands(context.Background()); err != nil {
		t.Fatalf("set commands: %v", err)
	}

	if !strings.Contains(buffer.String(), `"command":"fast"`) {
		t.Fatalf("expected fast command in payload, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"command":"expert"`) {
		t.Fatalf("expected expert command in payload, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"command":"balance"`) {
		t.Fatalf("expected balance command in payload, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"command":"new"`) {
		t.Fatalf("expected new command in payload, got %s", buffer.String())
	}
}

func TestHandleMessageRejectsUnauthorizedUser(t *testing.T) {
	bot, accessService, chatService, _, _, _, buffer := newTestBot(t)
	accessService.decision = accessapp.Decision{Allowed: false, Reason: accessapp.ReasonUserNotFound}

	bot.handleMessage(context.Background(), Message{
		MessageID: 13,
		From:      &User{ID: 2},
		Chat:      Chat{ID: 100},
		Text:      "hello",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	if !strings.Contains(buffer.String(), `无权限`) {
		t.Fatalf("expected unauthorized message, got %s", buffer.String())
	}
}

func TestHandleCallbackQueryRejectsUnauthorizedUser(t *testing.T) {
	bot, accessService, _, _, _, _, buffer := newTestBot(t)
	accessService.decision = accessapp.Decision{Allowed: false, Reason: accessapp.ReasonUserBanned}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-no-access",
		From: User{ID: 2},
		Message: &Message{
			MessageID: 21,
			Chat:      Chat{ID: 100},
		},
		Data: requestShapeCallback(domaindraw.ShapePortrait),
	})

	if !strings.Contains(buffer.String(), "answerCallbackQuery") {
		t.Fatalf("expected callback answer request, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"text":"无权限"`) {
		t.Fatalf("expected unauthorized callback text, got %s", buffer.String())
	}
}

func TestHandleMessageSendsTaskStartedTextWhenChatCreatesTask(t *testing.T) {
	bot, _, chatService, _, _, _, buffer := newTestBot(t)
	chatService.result = chatapp.HandleTextResult{
		SessionID:     "session-1",
		CreatedTaskID: "task-1",
	}

	bot.handleMessage(context.Background(), Message{
		MessageID: 14,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "开始绘图",
	})

	if !strings.Contains(buffer.String(), `"text":"已开始绘图"`) {
		t.Fatalf("expected task started text, got %s", buffer.String())
	}
}

func TestHandleStopTaskCallbackStopsTaskAndEditsProgressMessage(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-stop",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 32,
			Chat:      Chat{ID: 100},
		},
		Data: "task:stop:task-1",
	})

	if len(taskService.stops) != 1 || taskService.stops[0].TaskID != "task-1" || taskService.stops[0].UserID != "1" {
		t.Fatalf("unexpected stop commands: %#v", taskService.stops)
	}
	logOutput := buffer.String()
	if !strings.Contains(logOutput, `"text":"已停止任务"`) {
		t.Fatalf("expected stopped task text, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "editMessageText") {
		t.Fatalf("expected edit message request, got %s", logOutput)
	}
}

func TestHandleStopTaskCallbackReportsFailure(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.stopErr = errors.New("boom")

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-stop-fail",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 33,
			Chat:      Chat{ID: 100},
		},
		Data: "task:stop:task-1",
	})

	if !strings.Contains(buffer.String(), `"text":"停止任务失败"`) {
		t.Fatalf("expected stop failure callback response, got %s", buffer.String())
	}
}

func TestHandleTaskPromptCallbackSendsPromptMessage(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.prompt = taskapp.PromptDetails{
		Prompt:         "masterpiece, moonlit_girl",
		NegativePrompt: "blurry",
		Characters: []domaindraw.CharacterPrompt{
			{Prompt: "kinich_(genshin_impact)", NegativePrompt: "extra_arms", Position: "C3"},
		},
	}
	taskService.promptSet = true

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-prompt",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 34,
			Chat:      Chat{ID: 100},
		},
		Data: "task:prompt:task-1",
	})

	if len(taskService.prompts) != 1 || taskService.prompts[0].TaskID != "task-1" || taskService.prompts[0].UserID != "1" {
		t.Fatalf("unexpected prompt calls: %#v", taskService.prompts)
	}
	if !strings.Contains(buffer.String(), `Global Prompt`) || !strings.Contains(buffer.String(), `masterpiece, moonlit_girl`) || !strings.Contains(buffer.String(), `Negative Prompt`) || !strings.Contains(buffer.String(), `Character 1`) {
		t.Fatalf("expected prompt message in output, got %s", buffer.String())
	}
	if strings.Contains(buffer.String(), `"reply_to_message_id":34`) {
		t.Fatalf("did not expect prompt message to quote the callback message, got %s", buffer.String())
	}
}

func TestHandleRetryTranslateCallbackCreatesRetryTask(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-retry-translate",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 35,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:translate:task-1",
	})

	if len(taskService.retryTranslateCommands) != 1 || taskService.retryTranslateCommands[0].TaskID != "task-1" || taskService.retryTranslateCommands[0].UserID != "1" {
		t.Fatalf("unexpected retry translate commands: %#v", taskService.retryTranslateCommands)
	}
	if len(taskService.retryDrawCommands) != 0 {
		t.Fatalf("expected no retry draw commands, got %#v", taskService.retryDrawCommands)
	}
	if !strings.Contains(buffer.String(), `"text":"已重新翻译并开始绘图"`) {
		t.Fatalf("expected retry translate acknowledgement, got %s", buffer.String())
	}
}

func TestHandleRetryDrawCallbackCreatesRetryTask(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-retry-draw",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 36,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:draw:task-1",
	})

	if len(taskService.retryDrawCommands) != 1 || taskService.retryDrawCommands[0].TaskID != "task-1" || taskService.retryDrawCommands[0].UserID != "1" {
		t.Fatalf("unexpected retry draw commands: %#v", taskService.retryDrawCommands)
	}
	if len(taskService.retryTranslateCommands) != 0 {
		t.Fatalf("expected no retry translate commands, got %#v", taskService.retryTranslateCommands)
	}
	if !strings.Contains(buffer.String(), `"text":"已开始重新绘图"`) {
		t.Fatalf("expected retry draw acknowledgement, got %s", buffer.String())
	}
}

func TestHandleTaskPromptCallbackReportsFailure(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.promptErr = errors.New("boom")

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-prompt-fail",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 37,
			Chat:      Chat{ID: 100},
		},
		Data: "task:prompt:task-1",
	})

	if !strings.Contains(buffer.String(), `"text":"查看 prompt 失败"`) {
		t.Fatalf("expected prompt failure callback response, got %s", buffer.String())
	}
}

func TestHandleTaskPromptCallbackReportsEmptyPrompt(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.promptSet = true
	taskService.prompt = taskapp.PromptDetails{Prompt: "   "}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-prompt-empty",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 38,
			Chat:      Chat{ID: 100},
		},
		Data: "task:prompt:task-1",
	})

	if !strings.Contains(buffer.String(), `"text":"当前任务没有 prompt"`) {
		t.Fatalf("expected empty prompt callback response, got %s", buffer.String())
	}
}

func TestHandleRetryTranslateCallbackReportsFailure(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.retryTranslateErr = errors.New("boom")

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-retry-translate-fail",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 39,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:translate:task-1",
	})

	if !strings.Contains(buffer.String(), `"text":"重新翻译失败"`) {
		t.Fatalf("expected retry translate failure response, got %s", buffer.String())
	}
}

func TestHandleRetryDrawCallbackReportsFailure(t *testing.T) {
	bot, _, _, taskService, _, _, buffer := newTestBot(t)
	taskService.retryDrawErr = errors.New("boom")

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-retry-draw-fail",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 40,
			Chat:      Chat{ID: 100},
		},
		Data: "task:retry:draw:task-1",
	})

	if !strings.Contains(buffer.String(), `"text":"重新绘图失败"`) {
		t.Fatalf("expected retry draw failure response, got %s", buffer.String())
	}
}
