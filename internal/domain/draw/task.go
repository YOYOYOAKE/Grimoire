package draw

import (
	"fmt"
	"strings"
	"time"
)

type Shape string

const (
	ShapeSquare    Shape = "square"
	ShapeLandscape Shape = "landscape"
	ShapePortrait  Shape = "portrait"
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusTranslating Status = "translating"
	StatusSubmitting  Status = "submitting"
	StatusPolling     Status = "polling"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
)

type JobStatus string

const (
	JobQueued     JobStatus = "queued"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
)

type Task struct {
	ID               string
	ChatID           int64
	RequestMessageID int64
	StatusMessageID  int64
	Status           Status
	RequestText      string
	Shape            Shape
	Artist           string
	Prompt           string
	NegativePrompt   string
	ProviderJobID    string
	ErrorText        string
	CreatedAt        time.Time
	StartedAt        time.Time
	CompletedAt      time.Time
}

type CharacterPrompt struct {
	Prompt         string
	NegativePrompt string
	Position       string
}

type Translation struct {
	Prompt         string
	NegativePrompt string
	Characters     []CharacterPrompt
}

type GenerateRequest struct {
	Prompt         string
	NegativePrompt string
	Characters     []CharacterPrompt
	Shape          Shape
	Artists        string
}

type JobUpdate struct {
	Status        JobStatus
	QueuePosition int
	Image         []byte
	Error         string
}

func NewTask(id string, chatID, requestMessageID int64, requestText string, shape Shape, artist string, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	requestText = strings.TrimSpace(requestText)
	artist = strings.TrimSpace(artist)
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
		Artist:           artist,
		CreatedAt:        now,
	}, nil
}

func (s Shape) Valid() bool {
	switch s {
	case ShapeSquare, ShapeLandscape, ShapePortrait:
		return true
	default:
		return false
	}
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

func (t *Task) MarkSubmitting(now time.Time) error {
	if t.Status != StatusTranslating {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusSubmitting)
	}
	t.Status = StatusSubmitting
	t.StartedAt = normalizeStartedAt(t.StartedAt, now)
	return nil
}

func (t *Task) MarkPolling(jobID string, now time.Time) error {
	if t.Status != StatusSubmitting {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusPolling)
	}
	t.Status = StatusPolling
	t.ProviderJobID = strings.TrimSpace(jobID)
	t.StartedAt = normalizeStartedAt(t.StartedAt, now)
	return nil
}

func (t *Task) MarkCompleted(now time.Time) error {
	if t.Status != StatusPolling {
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
