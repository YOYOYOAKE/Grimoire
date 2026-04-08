package conversation

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	sqliterepo "grimoire/internal/adapters/repository/sqlite"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
	platformid "grimoire/internal/platform/id"
	sqlitefixture "grimoire/internal/testsupport/sqlitefixture"
)

type conversationSessionRepositoryStub struct {
	getSessions  []domainsession.Session
	getErr       error
	saveErr      error
	getCalls     int
	gotSessionID string
	savedSession domainsession.Session
}

func (s *conversationSessionRepositoryStub) Get(_ context.Context, sessionID string) (domainsession.Session, error) {
	s.gotSessionID = sessionID
	if s.getErr != nil {
		return domainsession.Session{}, s.getErr
	}
	if s.getCalls >= len(s.getSessions) {
		return domainsession.Session{}, errors.New("unexpected get call")
	}
	session := s.getSessions[s.getCalls]
	s.getCalls++
	return session, nil
}

func (s *conversationSessionRepositoryStub) Save(_ context.Context, session domainsession.Session) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.savedSession = session
	return nil
}

type conversationMessageRepositoryStub struct {
	recent        []domainsession.Message
	listRecentErr error
	appendErr     error
	gotSessionID  string
	gotLimit      int
	appended      []domainsession.Message
}

func (s *conversationMessageRepositoryStub) Append(_ context.Context, message domainsession.Message) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, message)
	return nil
}

func (s *conversationMessageRepositoryStub) ListRecent(_ context.Context, sessionID string, limit int) ([]domainsession.Message, error) {
	s.gotSessionID = sessionID
	s.gotLimit = limit
	if s.listRecentErr != nil {
		return nil, s.listRecentErr
	}
	return s.recent, nil
}

type conversationTxRunnerStub struct {
	calls int
}

func (s *conversationTxRunnerStub) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	s.calls++
	return fn(ctx)
}

type conversationModelStub struct {
	input  ConversationInput
	err    error
	output ConversationOutput
}

func (s *conversationModelStub) Converse(_ context.Context, input ConversationInput) (ConversationOutput, error) {
	s.input = input
	if s.err != nil {
		return ConversationOutput{}, s.err
	}
	return s.output, nil
}

func TestConverseLoadsRecentMessagesCallsModelAndPersistsReply(t *testing.T) {
	latestSession := mustSession(t, "session-1", "user-1", 1)
	recentMessage := mustMessage(t, "msg-1", "session-1", domainsession.MessageRoleUser, "hello", time.Unix(1, 0).UTC())
	sessions := &conversationSessionRepositoryStub{
		getSessions: []domainsession.Session{latestSession, latestSession},
	}
	messages := &conversationMessageRepositoryStub{
		recent: []domainsession.Message{recentMessage},
	}
	txRunner := &conversationTxRunnerStub{}
	model := &conversationModelStub{
		output: ConversationOutput{
			Reply: "  hi there  ",
		},
	}
	now := func() time.Time { return time.Unix(2, 0).UTC() }
	service := NewService(model, sessions, messages, txRunner, 15, now, func() string { return "assistant-1" }, nil)

	result, err := service.Converse(context.Background(), ConverseCommand{
		SessionID: " session-1 ",
	})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}

	if txRunner.calls != 1 {
		t.Fatalf("expected one transaction, got %d", txRunner.calls)
	}
	if sessions.gotSessionID != "session-1" {
		t.Fatalf("unexpected session id lookup: %q", sessions.gotSessionID)
	}
	if messages.gotSessionID != "session-1" {
		t.Fatalf("unexpected recent session id: %q", messages.gotSessionID)
	}
	if messages.gotLimit != 15 {
		t.Fatalf("unexpected recent limit: %d", messages.gotLimit)
	}
	if len(model.input.Messages) != 1 || model.input.Messages[0].ID != "msg-1" {
		t.Fatalf("unexpected model messages: %#v", model.input.Messages)
	}
	if result.Reply != "hi there" {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if result.CreateDrawingTask != nil {
		t.Fatalf("expected no create drawing task, got %#v", result.CreateDrawingTask)
	}
	if len(messages.appended) != 1 {
		t.Fatalf("expected one appended message, got %d", len(messages.appended))
	}
	if messages.appended[0].ID != "assistant-1" {
		t.Fatalf("unexpected assistant message id: %q", messages.appended[0].ID)
	}
	if messages.appended[0].Role != domainsession.MessageRoleAssistant {
		t.Fatalf("unexpected assistant role: %s", messages.appended[0].Role)
	}
	if sessions.savedSession.Length != 2 {
		t.Fatalf("expected saved session length 2, got %d", sessions.savedSession.Length)
	}
}

func TestConverseReloadsLatestSessionBeforePersisting(t *testing.T) {
	staleSession := mustSession(t, "session-1", "user-1", 1)
	latestSession := mustSession(t, "session-1", "user-1", 2)
	sessions := &conversationSessionRepositoryStub{
		getSessions: []domainsession.Session{staleSession, latestSession},
	}
	service := NewService(
		&conversationModelStub{
			output: ConversationOutput{
				Reply: "reply",
			},
		},
		sessions,
		&conversationMessageRepositoryStub{},
		&conversationTxRunnerStub{},
		10,
		func() time.Time { return time.Unix(3, 0).UTC() },
		func() string { return "assistant-1" },
		nil,
	)

	_, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if sessions.savedSession.Length != 3 {
		t.Fatalf("expected latest session length to increment to 3, got %d", sessions.savedSession.Length)
	}
}

func TestConverseSkipsAssistantReplyWhenCreatingTask(t *testing.T) {
	latestSession := mustSession(t, "session-1", "user-1", 1)
	sessions := &conversationSessionRepositoryStub{
		getSessions: []domainsession.Session{latestSession, latestSession},
	}
	messages := &conversationMessageRepositoryStub{}
	txRunner := &conversationTxRunnerStub{}
	model := &conversationModelStub{
		output: ConversationOutput{
			CreateDrawingTask: &CreateDrawingTask{Request: "draw a moonlit girl"},
		},
	}
	service := NewService(model, sessions, messages, txRunner, 15, func() time.Time { return time.Unix(2, 0).UTC() }, func() string { return "assistant-1" }, nil)

	result, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if result.Reply != "" {
		t.Fatalf("expected empty reply, got %q", result.Reply)
	}
	if result.CreateDrawingTask == nil || result.CreateDrawingTask.Request != "draw a moonlit girl" {
		t.Fatalf("unexpected create drawing task: %#v", result.CreateDrawingTask)
	}
	if len(messages.appended) != 0 {
		t.Fatalf("expected no assistant messages, got %#v", messages.appended)
	}
	if sessions.savedSession.Length != 1 {
		t.Fatalf("expected session length unchanged, got %d", sessions.savedSession.Length)
	}
}

func TestConverseReturnsModelErrorWithoutOpeningTransaction(t *testing.T) {
	session := mustSession(t, "session-1", "user-1", 0)
	modelErr := errors.New("llm unavailable")
	txRunner := &conversationTxRunnerStub{}
	service := NewService(
		&conversationModelStub{err: modelErr},
		&conversationSessionRepositoryStub{getSessions: []domainsession.Session{session}},
		&conversationMessageRepositoryStub{},
		txRunner,
		10,
		nil,
		nil,
		nil,
	)

	_, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"})
	if !errors.Is(err, modelErr) {
		t.Fatalf("expected model error, got %v", err)
	}
	if txRunner.calls != 0 {
		t.Fatalf("expected no transaction for model failure, got %d", txRunner.calls)
	}
}

func TestConverseRejectsReplyAndCreateDrawingTaskTogether(t *testing.T) {
	session := mustSession(t, "session-1", "user-1", 0)
	service := NewService(
		&conversationModelStub{
			output: ConversationOutput{
				Reply:             "reply",
				CreateDrawingTask: &CreateDrawingTask{Request: "draw a moonlit girl"},
			},
		},
		&conversationSessionRepositoryStub{getSessions: []domainsession.Session{session}},
		&conversationMessageRepositoryStub{},
		&conversationTxRunnerStub{},
		10,
		nil,
		nil,
		nil,
	)

	if _, err := service.Converse(context.Background(), ConverseCommand{
		SessionID: "session-1",
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestConverseRequiresTxRunner(t *testing.T) {
	service := NewService(nil, nil, nil, nil, 10, nil, nil, nil)

	_, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"})
	if !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected tx runner required error, got %v", err)
	}
}

func TestConverseRejectsNonPositiveRecentMessageLimit(t *testing.T) {
	service := NewService(nil, nil, nil, &conversationTxRunnerStub{}, 0, nil, nil, nil)

	_, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConverseRollsBackAssistantReplyWhenSaveFails(t *testing.T) {
	ctx := context.Background()
	db := sqlitefixture.OpenDB(t)
	sessionRepo := sqliterepo.NewSessionRepository(db, platformid.NewStaticGenerator("session-1"))
	messageRepo := sqliterepo.NewSessionMessageRepository(db)
	txRunner := sqliterepo.NewTxRunner(db)

	session := sqlitefixture.CreateUserAndSession(t, db, "user-1", "session-1", domainpreferences.DefaultPreference())
	sqlitefixture.AppendMessage(t, db, session.ID, "user-msg-1", domainsession.MessageRoleUser, "hello", time.Unix(1, 0).UTC())

	saveErr := errors.New("save failed")
	service := NewService(
		&conversationModelStub{
			output: ConversationOutput{
				Reply: "reply",
			},
		},
		&failingConversationSessionRepository{
			delegate: sessionRepo,
			saveErr:  saveErr,
		},
		messageRepo,
		txRunner,
		15,
		func() time.Time { return time.Unix(2, 0).UTC() },
		func() string { return "assistant-msg-1" },
		nil,
	)

	_, err := service.Converse(ctx, ConverseCommand{SessionID: session.ID})
	if !errors.Is(err, saveErr) {
		t.Fatalf("expected save error, got %v", err)
	}

	reloaded, err := sessionRepo.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.Length != 1 {
		t.Fatalf("expected session length unchanged after rollback, got %d", reloaded.Length)
	}

	recent, err := messageRepo.ListRecent(ctx, session.ID, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected only original user message after rollback, got %d", len(recent))
	}
	if recent[0].ID != "user-msg-1" {
		t.Fatalf("unexpected persisted message id: %q", recent[0].ID)
	}
}

type failingConversationSessionRepository struct {
	delegate *sqliterepo.SessionRepository
	saveErr  error
}

func (r *failingConversationSessionRepository) Get(ctx context.Context, sessionID string) (domainsession.Session, error) {
	return r.delegate.Get(ctx, sessionID)
}

func (r *failingConversationSessionRepository) Save(context.Context, domainsession.Session) error {
	return r.saveErr
}

func TestConverseLogsConversationLifecycle(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	latestSession := mustSession(t, "session-1", "user-1", 1)
	recentMessage := mustMessage(t, "msg-1", "session-1", domainsession.MessageRoleUser, "开始绘图", time.Unix(1, 0).UTC())
	service := NewService(
		&conversationModelStub{
			output: ConversationOutput{
				CreateDrawingTask: &CreateDrawingTask{Request: "draw a moonlit girl"},
			},
		},
		&conversationSessionRepositoryStub{getSessions: []domainsession.Session{latestSession, latestSession}},
		&conversationMessageRepositoryStub{recent: []domainsession.Message{recentMessage}},
		&conversationTxRunnerStub{},
		10,
		func() time.Time { return time.Unix(2, 0).UTC() },
		func() string { return "assistant-1" },
		logger,
	)

	if _, err := service.Converse(context.Background(), ConverseCommand{SessionID: "session-1"}); err != nil {
		t.Fatalf("converse: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"conversation requested",
		"session_id=session-1",
		"conversation model returned",
		"create_drawing_task=true",
		"request=\"draw a moonlit girl\"",
		"conversation persisted",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
}

func mustSession(t *testing.T, id string, userID string, length int) domainsession.Session {
	t.Helper()
	session, err := domainsession.Restore(id, userID, length)
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}
	return session
}

func mustMessage(t *testing.T, id string, sessionID string, role domainsession.MessageRole, content string, createdAt time.Time) domainsession.Message {
	t.Helper()
	message, err := domainsession.NewMessage(id, sessionID, role, content, createdAt)
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return message
}
