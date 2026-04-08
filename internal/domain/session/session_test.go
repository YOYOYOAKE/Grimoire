package session

import (
	"testing"
	"time"
)

func TestNewSessionStartsEmpty(t *testing.T) {
	s, err := New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if s.Length != 0 {
		t.Fatalf("unexpected length: %d", s.Length)
	}
}

func TestRestoreRejectsNegativeLength(t *testing.T) {
	if _, err := Restore("session-1", "user-1", -1); err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordMessageIncrementsLength(t *testing.T) {
	s, err := New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	message, err := NewMessage("message-1", "session-1", MessageRoleUser, "hello", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}

	if err := s.RecordMessage(message); err != nil {
		t.Fatalf("record message: %v", err)
	}
	if s.Length != 1 {
		t.Fatalf("unexpected length: %d", s.Length)
	}
}

func TestRecordMessageRejectsOtherSession(t *testing.T) {
	s, err := New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	message, err := NewMessage("message-1", "session-2", MessageRoleUser, "hello", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("new message: %v", err)
	}

	if err := s.RecordMessage(message); err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordMessageRejectsInvalidMessage(t *testing.T) {
	s, err := New("session-1", "user-1")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	if err := s.RecordMessage(Message{SessionID: "session-1"}); err == nil {
		t.Fatal("expected error")
	}
	if s.Length != 0 {
		t.Fatalf("unexpected length after invalid message: %d", s.Length)
	}
}

func TestNewMessageRejectsInvalidRole(t *testing.T) {
	if _, err := NewMessage("message-1", "session-1", MessageRole("system"), "hello", time.Unix(1, 0)); err == nil {
		t.Fatal("expected error")
	}
}
