package draw

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusTranslating Status = "translating"
	StatusGenerating  Status = "generating"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

type Task struct {
	ID               string
	ChatID           int64
	RequestMessageID int64
	StatusMessageID  int64
	Status           Status
	RequestText      string
	Shape            Shape
	Artists          string
	Prompt           string
	NegativePrompt   string
	ErrorText        string
	CreatedAt        time.Time
	StartedAt        time.Time
	CompletedAt      time.Time
}

func NewTask(id string, chatID, requestMessageID int64, requestText string, shape Shape, artists string, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	requestText = strings.TrimSpace(requestText)
	artists = strings.TrimSpace(artists)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if chatID == 0 {
		return Task{}, fmt.Errorf("chat id is required")
	}
	if requestText == "" {
		return Task{}, fmt.Errorf("request text is required")
	}
	if !shape.Valid() {
		return Task{}, fmt.Errorf("invalid shape %q", shape)
	}
	if now.IsZero() {
		now = time.Now()
	}

	return Task{
		ID:               id,
		ChatID:           chatID,
		RequestMessageID: requestMessageID,
		RequestText:      requestText,
		Shape:            shape,
		Artists:          artists,
		CreatedAt:        now,
	}, nil
}

func (t *Task) SetStatusMessageID(messageID int64) {
	t.StatusMessageID = messageID
}

func (t *Task) MarkTranslating(now time.Time) error {
	if t.Status != "" && t.Status != StatusQueued {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusTranslating)
	}
	t.Status = StatusTranslating
	t.StartedAt = normalizeStartedAt(t.StartedAt, now)
	return nil
}

func (t *Task) SetTranslation(prompt string, negative string) {
	t.Prompt = strings.TrimSpace(prompt)
	t.NegativePrompt = strings.TrimSpace(negative)
}

func (t *Task) MarkGenerating(now time.Time) error {
	if t.Status != StatusTranslating {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusGenerating)
	}
	t.Status = StatusGenerating
	t.StartedAt = normalizeStartedAt(t.StartedAt, now)
	return nil
}

func (t *Task) MarkCompleted(now time.Time) error {
	if t.Status != StatusGenerating {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusCompleted)
	}
	t.Status = StatusCompleted
	t.CompletedAt = normalizeTerminalAt(now)
	return nil
}

func (t *Task) MarkFailed(reason string, now time.Time) error {
	if t.Status == StatusCompleted || t.Status == StatusFailed {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusFailed)
	}
	t.Status = StatusFailed
	t.ErrorText = strings.TrimSpace(reason)
	t.CompletedAt = normalizeTerminalAt(now)
	return nil
}

func normalizeStartedAt(startedAt time.Time, now time.Time) time.Time {
	if !startedAt.IsZero() {
		return startedAt
	}
	if now.IsZero() {
		return time.Now()
	}
	return now
}

func normalizeTerminalAt(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now()
	}
	return now
}
