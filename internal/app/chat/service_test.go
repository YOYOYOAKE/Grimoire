package chat

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	conversationapp "grimoire/internal/app/conversation"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
)

type chatUserRepositoryStub struct {
	user          domainuser.User
	err           error
	gotTelegramID string
}

func (s *chatUserRepositoryStub) GetByTelegramID(_ context.Context, telegramID string) (domainuser.User, error) {
	s.gotTelegramID = telegramID
	if s.err != nil {
		return domainuser.User{}, s.err
	}
	return s.user, nil
}

type chatSessionServiceStub struct {
	session         domainsession.Session
	getOrCreateErr  error
	appendErr       error
	gotGetOrCreate  sessionapp.GetOrCreateCommand
	gotAppend       sessionapp.AppendMessageCommand
	appendCallCount int
}

func (s *chatSessionServiceStub) GetOrCreate(_ context.Context, command sessionapp.GetOrCreateCommand) (domainsession.Session, error) {
	s.gotGetOrCreate = command
	if s.getOrCreateErr != nil {
		return domainsession.Session{}, s.getOrCreateErr
	}
	return s.session, nil
}

func (s *chatSessionServiceStub) AppendUserMessage(_ context.Context, command sessionapp.AppendMessageCommand) (sessionapp.AppendMessageResult, error) {
	s.gotAppend = command
	s.appendCallCount++
	if s.appendErr != nil {
		return sessionapp.AppendMessageResult{}, s.appendErr
	}
	return sessionapp.AppendMessageResult{Session: s.session}, nil
}

type chatConversationServiceStub struct {
	result    conversationapp.ConverseResult
	err       error
	got       conversationapp.ConverseCommand
	callCount int
}

func (s *chatConversationServiceStub) Converse(_ context.Context, command conversationapp.ConverseCommand) (conversationapp.ConverseResult, error) {
	s.got = command
	s.callCount++
	if s.err != nil {
		return conversationapp.ConverseResult{}, s.err
	}
	return s.result, nil
}

type chatTaskServiceStub struct {
	got       taskapp.CreateCommand
	callCount int
	result    domaintask.Task
	err       error
}

func (s *chatTaskServiceStub) Create(_ context.Context, command taskapp.CreateCommand) (domaintask.Task, error) {
	s.got = command
	s.callCount++
	if s.err != nil {
		return domaintask.Task{}, s.err
	}
	if s.result.ID == "" {
		s.result = domaintask.Task{ID: "task-1"}
	}
	return s.result, nil
}

func TestHandleTextGetsSessionAppendsMessageAndConverse(t *testing.T) {
	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	user, err := domainuser.New("user-1", domainuser.RoleNormal, preference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	users := &chatUserRepositoryStub{user: user}
	sessions := &chatSessionServiceStub{session: session}
	conversations := &chatConversationServiceStub{
		result: conversationapp.ConverseResult{Reply: "需要补充一下光线方向。"},
	}
	service := NewService(users, sessions, conversations, &chatTaskServiceStub{}, nil)
	createdAt := time.Unix(10, 0).UTC()

	result, err := service.HandleText(context.Background(), HandleTextCommand{
		UserID:    " user-1 ",
		MessageID: " msg-1 ",
		Text:      " 画一个月下少女 ",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("handle text: %v", err)
	}

	if users.gotTelegramID != "user-1" {
		t.Fatalf("unexpected telegram id lookup: %q", users.gotTelegramID)
	}
	if sessions.gotGetOrCreate.UserID != "user-1" {
		t.Fatalf("unexpected get or create user id: %q", sessions.gotGetOrCreate.UserID)
	}
	if sessions.gotAppend.SessionID != "session-1" {
		t.Fatalf("unexpected append session id: %q", sessions.gotAppend.SessionID)
	}
	if sessions.gotAppend.MessageID != "msg-1" {
		t.Fatalf("unexpected append message id: %q", sessions.gotAppend.MessageID)
	}
	if sessions.gotAppend.Content != "画一个月下少女" {
		t.Fatalf("unexpected append content: %q", sessions.gotAppend.Content)
	}
	if !sessions.gotAppend.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected append created at: %v", sessions.gotAppend.CreatedAt)
	}
	if conversations.callCount != 1 {
		t.Fatalf("expected one converse call, got %d", conversations.callCount)
	}
	if conversations.got.SessionID != "session-1" {
		t.Fatalf("unexpected conversation session id: %q", conversations.got.SessionID)
	}
	if conversations.got.Preference.Artists != "artist:foo" {
		t.Fatalf("unexpected conversation preference artists: %q", conversations.got.Preference.Artists)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("unexpected result session id: %q", result.SessionID)
	}
	if result.Reply != "需要补充一下光线方向。" {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if result.CreatedTaskID != "" {
		t.Fatalf("expected no created task id, got %q", result.CreatedTaskID)
	}
}

func TestHandleTextReturnsAppendErrorWithoutCallingConversation(t *testing.T) {
	user := newChatTestUser(t, "user-1")
	session := newChatTestSession(t, "session-1", "user-1")
	appendErr := errors.New("append failed")
	conversations := &chatConversationServiceStub{}
	service := NewService(
		&chatUserRepositoryStub{user: user},
		&chatSessionServiceStub{session: session, appendErr: appendErr},
		conversations,
		&chatTaskServiceStub{},
		nil,
	)

	_, err := service.HandleText(context.Background(), HandleTextCommand{
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "hello",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	if !errors.Is(err, appendErr) {
		t.Fatalf("expected append error, got %v", err)
	}
	if conversations.callCount != 0 {
		t.Fatalf("expected no conversation call, got %d", conversations.callCount)
	}
}

func TestHandleTextRejectsInvalidCommand(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	tests := []HandleTextCommand{
		{MessageID: "msg-1", Text: "hello", CreatedAt: time.Unix(1, 0).UTC()},
		{UserID: "user-1", Text: "hello", CreatedAt: time.Unix(1, 0).UTC()},
		{UserID: "user-1", MessageID: "msg-1", CreatedAt: time.Unix(1, 0).UTC()},
		{UserID: "user-1", MessageID: "msg-1", Text: "hello"},
	}

	for _, command := range tests {
		if _, err := service.HandleText(context.Background(), command); err == nil {
			t.Fatalf("expected validation error for command %#v", command)
		}
	}
}

func TestHandleTextCreatesTaskWhenConversationRequestsDrawing(t *testing.T) {
	preference, err := domainpreferences.New(domaindraw.ShapeSquare, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	user, err := domainuser.New("user-1", domainuser.RoleNormal, preference)
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	session := newChatTestSession(t, "session-1", "user-1")
	tasks := &chatTaskServiceStub{}
	service := NewService(
		&chatUserRepositoryStub{user: user},
		&chatSessionServiceStub{session: session},
		&chatConversationServiceStub{
			result: conversationapp.ConverseResult{
				Summary:           domainsession.NewSummary(`{"topic":"moon","step":"draw"}`),
				CreateDrawingTask: &conversationapp.CreateDrawingTask{Request: "draw a moonlit girl"},
			},
		},
		tasks,
		nil,
	)

	result, err := service.HandleText(context.Background(), HandleTextCommand{
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "开始绘图",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("handle text: %v", err)
	}
	if result.Reply != "" {
		t.Fatalf("expected empty reply, got %q", result.Reply)
	}
	if result.CreatedTaskID != "task-1" {
		t.Fatalf("unexpected created task id: %q", result.CreatedTaskID)
	}
	if tasks.callCount != 1 {
		t.Fatalf("expected one task create call, got %d", tasks.callCount)
	}
	if tasks.got.UserID != "user-1" || tasks.got.SessionID != "session-1" {
		t.Fatalf("unexpected task create command: %#v", tasks.got)
	}
	if tasks.got.Request != "draw a moonlit girl" {
		t.Fatalf("unexpected task request: %q", tasks.got.Request)
	}
	if tasks.got.Context.Raw() != `{"version":1,"shape":"square","artists":"artist:foo"}` {
		t.Fatalf("unexpected task context: %q", tasks.got.Context.Raw())
	}
}

func TestHandleTextReturnsTaskCreateError(t *testing.T) {
	user := newChatTestUser(t, "user-1")
	session := newChatTestSession(t, "session-1", "user-1")
	createErr := errors.New("create failed")
	service := NewService(
		&chatUserRepositoryStub{user: user},
		&chatSessionServiceStub{session: session},
		&chatConversationServiceStub{
			result: conversationapp.ConverseResult{
				Summary:           domainsession.NewSummary(`{"topic":"moon"}`),
				CreateDrawingTask: &conversationapp.CreateDrawingTask{Request: "draw a moonlit girl"},
			},
		},
		&chatTaskServiceStub{err: createErr},
		nil,
	)

	_, err := service.HandleText(context.Background(), HandleTextCommand{
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "开始绘图",
		CreatedAt: time.Unix(1, 0).UTC(),
	})
	if !errors.Is(err, createErr) {
		t.Fatalf("expected task create error, got %v", err)
	}
}

func TestHandleTextLogsLifecycle(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	user := newChatTestUser(t, "user-1")
	session := newChatTestSession(t, "session-1", "user-1")
	service := NewService(
		&chatUserRepositoryStub{user: user},
		&chatSessionServiceStub{session: session},
		&chatConversationServiceStub{
			result: conversationapp.ConverseResult{
				Reply:   "需要补充一下光线方向。",
				Summary: domainsession.NewSummary(`{"topic":"moon"}`),
			},
		},
		&chatTaskServiceStub{},
		logger,
	)

	if _, err := service.HandleText(context.Background(), HandleTextCommand{
		UserID:    "user-1",
		MessageID: "msg-1",
		Text:      "开始绘图",
		CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("handle text: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"chat handle text started",
		"user_id=user-1",
		"message_id=msg-1",
		"chat conversation completed",
		"create_drawing_task=false",
		"chat reply returned without task creation",
		"reply=需要补充一下光线方向。",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
}

func newChatTestUser(t *testing.T, userID string) domainuser.User {
	t.Helper()
	user, err := domainuser.New(userID, domainuser.RoleNormal, domainpreferences.DefaultPreference())
	if err != nil {
		t.Fatalf("new user: %v", err)
	}
	return user
}

func newChatTestSession(t *testing.T, sessionID string, userID string) domainsession.Session {
	t.Helper()
	session, err := domainsession.New(sessionID, userID)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	return session
}
