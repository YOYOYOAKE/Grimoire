package draw

import (
	"fmt"
	"strings"
	"time"
)

type Shape string

const (
	ShapeSmallPortrait  Shape = "small-portrait"
	ShapeSmallLandscape Shape = "small-landscape"
	ShapeSmallSquare    Shape = "small-square"
	ShapePortrait       Shape = "portrait"
	ShapeLandscape      Shape = "landscape"
	ShapeSquare         Shape = "square"
	ShapeLargePortrait  Shape = "large-portrait"
	ShapeLargeLandscape Shape = "large-landscape"
	ShapeLargeSquare    Shape = "large-square"
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
	case ShapeSmallPortrait, ShapeSmallLandscape, ShapeSmallSquare,
		ShapePortrait, ShapeLandscape, ShapeSquare,
		ShapeLargePortrait, ShapeLargeLandscape, ShapeLargeSquare:
		return true
	default:
		return false
	}
}

func (s Shape) Label() string {
	switch s {
	case ShapeSmallPortrait:
		return "Small Portrait (512x768)"
	case ShapeSmallLandscape:
		return "Small Landscape (768x512)"
	case ShapeSmallSquare:
		return "Small Square (640x640)"
	case ShapePortrait:
		return "Normal Portrait (832x1216)"
	case ShapeLandscape:
		return "Normal Landscape (1216x832)"
	case ShapeSquare:
		return "Normal Square (1024x1024)"
	case ShapeLargePortrait:
		return "Large Portrait (1014x1536)"
	case ShapeLargeLandscape:
		return "Large Landscape (1536x1024)"
	case ShapeLargeSquare:
		return "Large Square (1472x1472)"
	default:
		return string(s)
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
