package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	accessapp "grimoire/internal/app/access"
	chatapp "grimoire/internal/app/chat"
	preferencesapp "grimoire/internal/app/preferences"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
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
	if strings.TrimSpace(m.result.Reply) == "" {
		m.result = chatapp.HandleTextResult{Reply: "请再补充一点构图方向。"}
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

type preferenceServiceMock struct {
	pref domainpreferences.Preference
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

func (m *balanceServiceMock) GetBalance(_ context.Context) (domainnai.AccountBalance, error) {
	if m.err != nil {
		return domainnai.AccountBalance{}, m.err
	}
	return m.balance, nil
}

func newTestBot(t *testing.T) (*Bot, *accessServiceMock, *chatServiceMock, *preferenceServiceMock, *balanceServiceMock, *bytes.Buffer) {
	t.Helper()
	buffer := &bytes.Buffer{}
	bot := NewBot(config.Config{
		Telegram: config.Telegram{
			BotToken:    "token",
			AdminUserID: 1,
		},
	}, nil)
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
	bot.SetPreferenceService(prefService)
	bot.SetBalanceService(balanceService)
	return bot, accessService, chatService, prefService, balanceService, buffer
}

func TestHandleMessageUsesChatService(t *testing.T) {
	bot, accessService, chatService, _, _, buffer := newTestBot(t)
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
	if !strings.Contains(buffer.String(), `"reply_to_message_id":10`) {
		t.Fatalf("expected reply target in output, got %s", buffer.String())
	}
}

func TestRouteUpdateDispatchesMessage(t *testing.T) {
	bot, _, chatService, _, _, _ := newTestBot(t)
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
	bot, _, chatService, _, _, buffer := newTestBot(t)
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
	bot, _, chatService, _, _, buffer := newTestBot(t)
	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "/start",
	})

	if len(chatService.commands) != 0 {
		t.Fatalf("expected no chat command, got %d", len(chatService.commands))
	}
	if !strings.Contains(buffer.String(), "发送文本即可进入需求对话，确认后再开始绘图。") {
		t.Fatalf("expected updated start text, got %s", buffer.String())
	}
}

func TestImgCallbackUpdatesShape(t *testing.T) {
	bot, _, _, prefService, _, buffer := newTestBot(t)
	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-1",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 20,
			Chat:      Chat{ID: 100},
		},
		Data: cbShapePortrait,
	})

	if prefService.pref.Shape != domaindraw.ShapePortrait {
		t.Fatalf("unexpected shape: %s", prefService.pref.Shape)
	}
	if !strings.Contains(buffer.String(), "editMessageText") {
		t.Fatalf("expected edit message request, got %s", buffer.String())
	}
}

func TestHandleCallbackQueryRejectsInvalidData(t *testing.T) {
	bot, _, _, _, _, buffer := newTestBot(t)
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
	bot, _, _, prefService, _, _ := newTestBot(t)
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
}

func TestSendPhotoIncludesReplyToMessage(t *testing.T) {
	bot, _, _, _, _, buffer := newTestBot(t)

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

func TestDeleteMessageUsesDeleteMessageEndpoint(t *testing.T) {
	bot, _, _, _, _, buffer := newTestBot(t)

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
	bot, _, _, _, _, buffer := newTestBot(t)
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
	bot, _, _, _, balanceService, buffer := newTestBot(t)
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

func TestSetMyCommandsIncludesBalance(t *testing.T) {
	bot, _, _, _, _, buffer := newTestBot(t)

	if err := bot.setMyCommands(context.Background()); err != nil {
		t.Fatalf("set commands: %v", err)
	}

	if !strings.Contains(buffer.String(), `"command":"balance"`) {
		t.Fatalf("expected balance command in payload, got %s", buffer.String())
	}
}

func TestHandleMessageRejectsUnauthorizedUser(t *testing.T) {
	bot, accessService, chatService, _, _, buffer := newTestBot(t)
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
	bot, accessService, _, _, _, buffer := newTestBot(t)
	accessService.decision = accessapp.Decision{Allowed: false, Reason: accessapp.ReasonUserBanned}

	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-no-access",
		From: User{ID: 2},
		Message: &Message{
			MessageID: 21,
			Chat:      Chat{ID: 100},
		},
		Data: cbShapePortrait,
	})

	if !strings.Contains(buffer.String(), "answerCallbackQuery") {
		t.Fatalf("expected callback answer request, got %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), `"text":"无权限"`) {
		t.Fatalf("expected unauthorized callback text, got %s", buffer.String())
	}
}
