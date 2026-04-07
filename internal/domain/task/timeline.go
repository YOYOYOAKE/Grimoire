package task

import (
	"fmt"
	"time"
)

type Timeline struct {
	CreatedAt            time.Time  `json:"created_at"`
	TranslatingStartedAt *time.Time `json:"translating_started_at,omitempty"`
	DrawingStartedAt     *time.Time `json:"drawing_started_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	FailedAt             *time.Time `json:"failed_at,omitempty"`
	StoppedAt            *time.Time `json:"stopped_at,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func NewTimeline(createdAt time.Time) (Timeline, error) {
	if createdAt.IsZero() {
		return Timeline{}, fmt.Errorf("task created_at is required")
	}
	return Timeline{
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}, nil
}

func (t *Timeline) MarkTranslating(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("translating_started_at is required")
	}
	t.TranslatingStartedAt = timePtr(at)
	t.UpdatedAt = at
	return nil
}

func (t *Timeline) MarkDrawing(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("drawing_started_at is required")
	}
	t.DrawingStartedAt = timePtr(at)
	t.UpdatedAt = at
	return nil
}

func (t *Timeline) MarkCompleted(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("completed_at is required")
	}
	t.CompletedAt = timePtr(at)
	t.UpdatedAt = at
	return nil
}

func (t *Timeline) MarkFailed(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("failed_at is required")
	}
	t.FailedAt = timePtr(at)
	t.UpdatedAt = at
	return nil
}

func (t *Timeline) MarkStopped(at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("stopped_at is required")
	}
	t.StoppedAt = timePtr(at)
	t.UpdatedAt = at
	return nil
}

func timePtr(value time.Time) *time.Time {
	return &value
}
