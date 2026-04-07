package request

import (
	"context"
	"errors"
	"testing"
	"time"

	domaindraw "grimoire/internal/domain/draw"
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type requestSessionRepositoryStub struct {
	session      domainsession.Session
	err          error
	gotSessionID string
}

func (s *requestSessionRepositoryStub) Get(_ context.Context, sessionID string) (domainsession.Session, error) {
	s.gotSessionID = sessionID
	if s.err != nil {
		return domainsession.Session{}, s.err
	}
	return s.session, nil
}

type requestMessageRepositoryStub struct {
	recent       []domainsession.Message
	err          error
	gotSessionID string
	gotLimit     int
}

func (s *requestMessageRepositoryStub) ListRecent(_ context.Context, sessionID string, limit int) ([]domainsession.Message, error) {
	s.gotSessionID = sessionID
	s.gotLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.recent, nil
}

type requestGeneratorStub struct {
	input  GenerateInput
	err    error
	output string
}

func (s *requestGeneratorStub) Generate(_ context.Context, input GenerateInput) (string, error) {
	s.input = input
	if s.err != nil {
		return "", s.err
	}
	return s.output, nil
}

func TestGenerateBuildsPendingRequestWithConfirmAndReviseActions(t *testing.T) {
	session := mustRequestSession(t, "session-1", "user-1", domainsession.NewSummary(`{"topic":"moon"}`))
	message := mustRequestMessage(t, "msg-1", "session-1", "hello")
	generator := &requestGeneratorStub{output: "  draw a moonlit girl  "}
	messages := &requestMessageRepositoryStub{recent: []domainsession.Message{message}}
	service := NewService(
		generator,
		&requestSessionRepositoryStub{session: session},
		messages,
		15,
	)

	preference, err := domainpreferences.New(domaindraw.ShapePortrait, "artist:foo")
	if err != nil {
		t.Fatalf("new preference: %v", err)
	}
	pending, err := service.Generate(context.Background(), GenerateCommand{
		SessionID:  " session-1 ",
		Preference: preference,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if pending.Request != "draw a moonlit girl" {
		t.Fatalf("unexpected request: %q", pending.Request)
	}
	if pending.ConfirmAction.CallbackData != "request:confirm:session-1" {
		t.Fatalf("unexpected confirm callback: %q", pending.ConfirmAction.CallbackData)
	}
	if pending.ReviseAction.CallbackData != "request:revise:session-1" {
		t.Fatalf("unexpected revise callback: %q", pending.ReviseAction.CallbackData)
	}
	if generator.input.Summary.Content() != `{"topic":"moon"}` {
		t.Fatalf("unexpected generator summary: %q", generator.input.Summary.Content())
	}
	if len(generator.input.Messages) != 1 || generator.input.Messages[0].ID != "msg-1" {
		t.Fatalf("unexpected generator messages: %#v", generator.input.Messages)
	}
	if messages.gotLimit != 15 {
		t.Fatalf("unexpected recent limit: %d", messages.gotLimit)
	}
}

func TestGenerateReturnsGeneratorError(t *testing.T) {
	session := mustRequestSession(t, "session-1", "user-1", domainsession.EmptySummary())
	generatorErr := errors.New("generator failed")
	service := NewService(
		&requestGeneratorStub{err: generatorErr},
		&requestSessionRepositoryStub{session: session},
		&requestMessageRepositoryStub{},
		15,
	)

	_, err := service.Generate(context.Background(), GenerateCommand{
		SessionID:  "session-1",
		Preference: domainpreferences.DefaultPreference(),
	})
	if !errors.Is(err, generatorErr) {
		t.Fatalf("expected generator error, got %v", err)
	}
}

func TestGenerateRejectsBlankGeneratedRequest(t *testing.T) {
	session := mustRequestSession(t, "session-1", "user-1", domainsession.EmptySummary())
	service := NewService(
		&requestGeneratorStub{output: " \t "},
		&requestSessionRepositoryStub{session: session},
		&requestMessageRepositoryStub{},
		15,
	)

	_, err := service.Generate(context.Background(), GenerateCommand{
		SessionID:  "session-1",
		Preference: domainpreferences.DefaultPreference(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateRejectsNonPositiveRecentMessageLimit(t *testing.T) {
	service := NewService(nil, nil, nil, 0)

	_, err := service.Generate(context.Background(), GenerateCommand{
		SessionID:  "session-1",
		Preference: domainpreferences.DefaultPreference(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveDecisionFromCallback(t *testing.T) {
	service := NewService(nil, nil, nil, 15)

	confirm := service.ResolveDecision(ResolveDecisionCommand{
		CallbackData: "request:confirm:session-1",
	})
	if confirm.Kind != DecisionKindConfirm || confirm.SessionID != "session-1" {
		t.Fatalf("unexpected confirm decision: %#v", confirm)
	}

	revise := service.ResolveDecision(ResolveDecisionCommand{
		CallbackData: "request:revise:session-2",
	})
	if revise.Kind != DecisionKindRevise || revise.SessionID != "session-2" {
		t.Fatalf("unexpected revise decision: %#v", revise)
	}
}

func TestResolveDecisionFromFallbackText(t *testing.T) {
	service := NewService(nil, nil, nil, 15)

	confirm := service.ResolveDecision(ResolveDecisionCommand{
		SessionID: "session-1",
		Text:      "确认执行",
	})
	if confirm.Kind != DecisionKindConfirm || confirm.SessionID != "session-1" {
		t.Fatalf("unexpected confirm decision: %#v", confirm)
	}

	revise := service.ResolveDecision(ResolveDecisionCommand{
		SessionID: "session-1",
		Text:      "继续修改",
	})
	if revise.Kind != DecisionKindRevise || revise.SessionID != "session-1" {
		t.Fatalf("unexpected revise decision: %#v", revise)
	}
}

func TestResolveDecisionRejectsAmbiguousText(t *testing.T) {
	service := NewService(nil, nil, nil, 15)

	decision := service.ResolveDecision(ResolveDecisionCommand{
		SessionID: "session-1",
		Text:      "好的，那就这样吧",
	})
	if decision != (Decision{}) {
		t.Fatalf("expected empty decision, got %#v", decision)
	}
}

func mustRequestSession(t *testing.T, id string, userID string, summary domainsession.Summary) domainsession.Session {
	t.Helper()
	session, err := domainsession.Restore(id, userID, 1, summary)
	if err != nil {
		t.Fatalf("restore session: %v", err)
	}
	return session
}

func mustRequestMessage(t *testing.T, id string, sessionID string, content string) domainsession.Message {
	t.Helper()
	message, err := domainsession.NewMessage(id, sessionID, domainsession.MessageRoleUser, content, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	return message
}
