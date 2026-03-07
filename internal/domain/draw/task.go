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
	UserID           int64
	RequestMessageID int64
	StatusMessageID  int64
	Status           Status
	Prompt           string
	Shape            Shape
	Artist           string
	PositivePrompt   string
	NegativePrompt   string
	ProviderJobID    string
	ErrorText        string
	CreatedAt        time.Time
	StartedAt        time.Time
	CompletedAt      time.Time
}

type Translation struct {
	PositivePrompt string
	NegativePrompt string
}

type GenerateRequest struct {
	PositivePrompt string
	NegativePrompt string
	Shape          Shape
}

type JobUpdate struct {
	Status        JobStatus
	QueuePosition int
	Image         []byte
	Error         string
}

func NewTask(id string, chatID, userID, requestMessageID int64, prompt string, shape Shape, artist string, now time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	prompt = strings.TrimSpace(prompt)
	artist = strings.TrimSpace(artist)
	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if chatID == 0 {
		return Task{}, fmt.Errorf("chat id is required")
	}
	if userID == 0 {
		return Task{}, fmt.Errorf("user id is required")
	}
	if prompt == "" {
		return Task{}, fmt.Errorf("prompt is required")
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
		UserID:           userID,
		RequestMessageID: requestMessageID,
		Prompt:           prompt,
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

func (t *Task) SetTranslation(positive string, negative string) {
	t.PositivePrompt = strings.TrimSpace(positive)
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
