package session

import (
	"context"
	"errors"
	"testing"
	"time"

	domainsession "grimoire/internal/domain/session"
)

type sessionRepositoryStub struct {
	session      domainsession.Session
	getErr       error
	saveErr      error
	createNewErr error
	gotUserID    string
	gotSessionID string
	savedSession domainsession.Session
	createdNew   domainsession.Session
}

func (s *sessionRepositoryStub) GetOrCreateActiveByUserID(_ context.Context, userID string) (domainsession.Session, error) {
	s.gotUserID = userID
	if s.getErr != nil {
		return domainsession.Session{}, s.getErr
	}
	return s.session, nil
}

func (s *sessionRepositoryStub) Save(_ context.Context, session domainsession.Session) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.savedSession = session
	s.session = session
	return nil
}

func (s *sessionRepositoryStub) CreateNewActiveByUserID(_ context.Context, userID string) (domainsession.Session, error) {
	s.gotUserID = userID
	if s.createNewErr != nil {
		return domainsession.Session{}, s.createNewErr
	}
	if s.createdNew.ID != "" {
		return s.createdNew, nil
	}
	return s.session, nil
}

func (s *sessionRepositoryStub) Get(_ context.Context, sessionID string) (domainsession.Session, error) {
	s.gotSessionID = sessionID
	if s.getErr != nil {
		return domainsession.Session{}, s.getErr
	}
	return s.session, nil
}

type sessionMessageRepositoryStub struct {
	appended      []domainsession.Message
	appendErr     error
	recent        []domainsession.Message
	listRecentErr error
	gotSessionID  string
	gotLimit      int
}

func (s *sessionMessageRepositoryStub) Append(_ context.Context, message domainsession.Message) error {
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appended = append(s.appended, message)
	return nil
}

func (s *sessionMessageRepositoryStub) ListRecent(_ context.Context, sessionID string, limit int) ([]domainsession.Message, error) {
	s.gotSessionID = sessionID
	s.gotLimit = limit
	if s.listRecentErr != nil {
		return nil, s.listRecentErr
	}
	return s.recent, nil
}

type txRunnerStub struct {
	calls int
}

func (s *txRunnerStub) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	s.calls++
	return fn(ctx)
}

func TestGetOrCreateReturnsActiveSession(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	sessions := &sessionRepositoryStub{session: session}
	service := NewService(sessions, &sessionMessageRepositoryStub{}, nil)

	got, err := service.GetOrCreate(context.Background(), GetOrCreateCommand{UserID: " user-1 "})
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if got.ID != "session-1" {
		t.Fatalf("unexpected session id: %q", got.ID)
	}
	if sessions.gotUserID != "user-1" {
		t.Fatalf("expected trimmed user id, got %q", sessions.gotUserID)
	}
}

func TestCreateNewReturnsFreshActiveSession(t *testing.T) {
	session, err := domainsession.New("session-2", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	sessions := &sessionRepositoryStub{createdNew: session}
	txRunner := &txRunnerStub{}
	service := NewService(sessions, &sessionMessageRepositoryStub{}, txRunner)

	got, err := service.CreateNew(context.Background(), CreateNewCommand{UserID: " user-1 "})
	if err != nil {
		t.Fatalf("create new session: %v", err)
	}
	if txRunner.calls != 1 {
		t.Fatalf("expected one transaction, got %d", txRunner.calls)
	}
	if got.ID != "session-2" {
		t.Fatalf("unexpected session id: %q", got.ID)
	}
	if sessions.gotUserID != "user-1" {
		t.Fatalf("expected trimmed user id, got %q", sessions.gotUserID)
	}
}

func TestCreateNewRequiresTxRunner(t *testing.T) {
	service := NewService(&sessionRepositoryStub{}, &sessionMessageRepositoryStub{}, nil)

	if _, err := service.CreateNew(context.Background(), CreateNewCommand{UserID: "user-1"}); !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected ErrTxRunnerRequired, got %v", err)
	}
}

func TestAppendUserMessagePersistsMessageAndSessionLength(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	sessions := &sessionRepositoryStub{session: session}
	messages := &sessionMessageRepositoryStub{}
	txRunner := &txRunnerStub{}
	service := NewService(sessions, messages, txRunner)
	createdAt := time.Now().UTC()

	result, err := service.AppendUserMessage(context.Background(), AppendMessageCommand{
		SessionID: "session-1",
		MessageID: "msg-1",
		Content:   "hello",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if txRunner.calls != 1 {
		t.Fatalf("expected one transaction, got %d", txRunner.calls)
	}
	if result.Message.Role != domainsession.MessageRoleUser {
		t.Fatalf("unexpected role: %s", result.Message.Role)
	}
	if result.Message.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %q", result.Message.SessionID)
	}
	if len(messages.appended) != 1 {
		t.Fatalf("expected one appended message, got %d", len(messages.appended))
	}
	if sessions.savedSession.Length != 1 {
		t.Fatalf("expected session length 1, got %d", sessions.savedSession.Length)
	}
}

func TestAppendAssistantMessageUsesAssistantRole(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	service := NewService(
		&sessionRepositoryStub{session: session},
		&sessionMessageRepositoryStub{},
		&txRunnerStub{},
	)

	result, err := service.AppendAssistantMessage(context.Background(), AppendMessageCommand{
		SessionID: "session-1",
		MessageID: "msg-1",
		Content:   "reply",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("append assistant message: %v", err)
	}
	if result.Message.Role != domainsession.MessageRoleAssistant {
		t.Fatalf("unexpected role: %s", result.Message.Role)
	}
}

func TestListRecentMessagesReturnsSessionAndMessages(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	message, err := domainsession.NewMessage("msg-1", "session-1", domainsession.MessageRoleUser, "hello", time.Now().UTC())
	if err != nil {
		t.Fatalf("new message: %v", err)
	}

	messages := &sessionMessageRepositoryStub{recent: []domainsession.Message{message}}
	service := NewService(&sessionRepositoryStub{session: session}, messages, nil)

	result, err := service.ListRecentMessages(context.Background(), ListRecentMessagesCommand{
		SessionID: "session-1",
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("list recent messages: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(result.Messages))
	}
	if messages.gotSessionID != "session-1" {
		t.Fatalf("unexpected list session id: %q", messages.gotSessionID)
	}
	if messages.gotLimit != 5 {
		t.Fatalf("unexpected limit: %d", messages.gotLimit)
	}
}

func TestListRecentMessagesRejectsNonPositiveLimit(t *testing.T) {
	service := NewService(&sessionRepositoryStub{}, &sessionMessageRepositoryStub{}, nil)

	_, err := service.ListRecentMessages(context.Background(), ListRecentMessagesCommand{
		SessionID: "session-1",
		Limit:     0,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAppendUserMessageReturnsRepositoryError(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	appendErr := errors.New("append failed")
	service := NewService(
		&sessionRepositoryStub{session: session},
		&sessionMessageRepositoryStub{appendErr: appendErr},
		&txRunnerStub{},
	)

	_, err = service.AppendUserMessage(context.Background(), AppendMessageCommand{
		SessionID: "session-1",
		MessageID: "msg-1",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, appendErr) {
		t.Fatalf("expected append error, got %v", err)
	}
}

func TestAppendUserMessageRequiresTxRunner(t *testing.T) {
	session, err := domainsession.New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	service := NewService(&sessionRepositoryStub{session: session}, &sessionMessageRepositoryStub{}, nil)
	_, err = service.AppendUserMessage(context.Background(), AppendMessageCommand{
		SessionID: "session-1",
		MessageID: "msg-1",
		Content:   "hello",
		CreatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, ErrTxRunnerRequired) {
		t.Fatalf("expected tx runner required error, got %v", err)
	}
}
