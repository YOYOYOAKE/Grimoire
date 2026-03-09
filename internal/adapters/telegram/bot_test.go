package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	drawapp "grimoire/internal/app/draw"
	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type drawServiceMock struct {
	commands []drawapp.SubmitCommand
}

func (m *drawServiceMock) Submit(_ context.Context, command drawapp.SubmitCommand) (domaindraw.Task, error) {
	m.commands = append(m.commands, command)
	return domaindraw.Task{ID: "task-1"}, nil
}

type preferenceServiceMock struct {
	pref domainpreferences.UserPreference
}

func (m *preferenceServiceMock) GetOrCreate(_ context.Context, userID int64) (domainpreferences.UserPreference, error) {
	if m.pref.UserID == 0 {
		m.pref = domainpreferences.NewUserPreference(userID, time.Now())
	}
	return m.pref, nil
}

func (m *preferenceServiceMock) UpdateShape(_ context.Context, userID int64, shape domaindraw.Shape) (domainpreferences.UserPreference, error) {
	m.pref = domainpreferences.NewUserPreference(userID, time.Now())
	m.pref.DefaultShape = shape
	return m.pref, nil
}

func (m *preferenceServiceMock) UpdateArtist(_ context.Context, userID int64, artist string) (domainpreferences.UserPreference, error) {
	m.pref = domainpreferences.NewUserPreference(userID, time.Now())
	m.pref.Artist = strings.TrimSpace(artist)
	return m.pref, nil
}

func (m *preferenceServiceMock) ClearArtist(_ context.Context, userID int64) (domainpreferences.UserPreference, error) {
	m.pref = domainpreferences.NewUserPreference(userID, time.Now())
	return m.pref, nil
}

func newTestBot(t *testing.T) (*Bot, *drawServiceMock, *preferenceServiceMock, *bytes.Buffer) {
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

	drawService := &drawServiceMock{}
	prefService := &preferenceServiceMock{}
	bot.SetDrawService(drawService)
	bot.SetPreferenceService(prefService)
	return bot, drawService, prefService, buffer
}

func TestHandleMessageSubmitsDrawTask(t *testing.T) {
	bot, drawService, _, _ := newTestBot(t)
	bot.handleMessage(context.Background(), Message{
		MessageID: 10,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      "画一个月下的少女",
	})

	if len(drawService.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(drawService.commands))
	}
	if drawService.commands[0].Prompt != "画一个月下的少女" {
		t.Fatalf("unexpected prompt: %q", drawService.commands[0].Prompt)
	}
}

func TestImgCallbackUpdatesShape(t *testing.T) {
	bot, _, prefService, buffer := newTestBot(t)
	bot.handleCallbackQuery(context.Background(), CallbackQuery{
		ID:   "cb-1",
		From: User{ID: 1},
		Message: &Message{
			MessageID: 20,
			Chat:      Chat{ID: 100},
		},
		Data: cbShapePortrait,
	})

	if prefService.pref.DefaultShape != domaindraw.ShapePortrait {
		t.Fatalf("unexpected shape: %s", prefService.pref.DefaultShape)
	}
	if !strings.Contains(buffer.String(), "editMessageText") {
		t.Fatalf("expected edit message request, got %s", buffer.String())
	}
}

func TestPendingArtistFlow(t *testing.T) {
	bot, _, prefService, _ := newTestBot(t)
	bot.setPendingArtist(1)
	bot.handleMessage(context.Background(), Message{
		MessageID: 11,
		From:      &User{ID: 1},
		Chat:      Chat{ID: 100},
		Text:      " artist:foo ",
	})

	if prefService.pref.Artist != "artist:foo" {
		t.Fatalf("unexpected artist: %q", prefService.pref.Artist)
	}
	if bot.isPendingArtist(1) {
		t.Fatal("expected pending artist cleared")
	}
}

func TestResultMessageHasNoRetryButtons(t *testing.T) {
	pref := domainpreferences.NewUserPreference(1, time.Now())
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
	bot, _, _, buffer := newTestBot(t)

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
	bot, _, _, buffer := newTestBot(t)

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
