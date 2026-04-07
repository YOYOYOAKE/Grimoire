package task

import (
	"fmt"
	"strings"
	"time"
)

type Task struct {
	ID                string
	UserID            string
	SessionID         string
	SourceTaskID      string
	Request           string
	Prompt            string
	Image             string
	Status            Status
	Error             *TaskError
	Timeline          Timeline
	Context           Context
	ProgressMessageID string
	ResultMessageID   string
}

func New(id string, userID string, sessionID string, request string, context Context, createdAt time.Time) (Task, error) {
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	request = strings.TrimSpace(request)

	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if userID == "" {
		return Task{}, fmt.Errorf("task user id is required")
	}
	if sessionID == "" {
		return Task{}, fmt.Errorf("task session id is required")
	}
	if request == "" {
		return Task{}, fmt.Errorf("task request is required")
	}
	if err := context.Validate(); err != nil {
		return Task{}, err
	}

	timeline, err := NewTimeline(createdAt)
	if err != nil {
		return Task{}, err
	}

	return Task{
		ID:        id,
		UserID:    userID,
		SessionID: sessionID,
		Request:   request,
		Status:    StatusQueued,
		Timeline:  timeline,
		Context:   context,
	}, nil
}

func Restore(
	id string,
	userID string,
	sessionID string,
	sourceTaskID string,
	request string,
	prompt string,
	image string,
	status Status,
	taskError *TaskError,
	timeline Timeline,
	context Context,
	progressMessageID string,
	resultMessageID string,
) (Task, error) {
	id = strings.TrimSpace(id)
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	request = strings.TrimSpace(request)
	prompt = strings.TrimSpace(prompt)
	image = strings.TrimSpace(image)
	progressMessageID = strings.TrimSpace(progressMessageID)
	resultMessageID = strings.TrimSpace(resultMessageID)

	if id == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	if userID == "" {
		return Task{}, fmt.Errorf("task user id is required")
	}
	if sessionID == "" {
		return Task{}, fmt.Errorf("task session id is required")
	}
	if request == "" {
		return Task{}, fmt.Errorf("task request is required")
	}
	if !status.Valid() {
		return Task{}, fmt.Errorf("invalid task status %q", status)
	}
	if err := context.Validate(); err != nil {
		return Task{}, err
	}
	if err := timeline.Validate(); err != nil {
		return Task{}, err
	}

	var normalizedError *TaskError
	if taskError != nil {
		errorCopy := *taskError
		if err := errorCopy.Validate(); err != nil {
			return Task{}, err
		}
		normalizedError = &errorCopy
	}

	task := Task{
		ID:                id,
		UserID:            userID,
		SessionID:         sessionID,
		Request:           request,
		Prompt:            prompt,
		Image:             image,
		Status:            status,
		Error:             normalizedError,
		Timeline:          timeline,
		Context:           context,
		ProgressMessageID: progressMessageID,
		ResultMessageID:   resultMessageID,
	}
	if err := task.SetSourceTask(sourceTaskID); err != nil {
		return Task{}, err
	}
	if err := task.validateRestoredState(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (t *Task) SetSourceTask(sourceTaskID string) error {
	sourceTaskID = strings.TrimSpace(sourceTaskID)
	if sourceTaskID == "" {
		t.SourceTaskID = ""
		return nil
	}
	if sourceTaskID == t.ID {
		return fmt.Errorf("source task id cannot equal task id")
	}
	t.SourceTaskID = sourceTaskID
	return nil
}

func (t *Task) SetPrompt(prompt string) error {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fmt.Errorf("task prompt is required")
	}
	t.Prompt = prompt
	return nil
}

func (t *Task) SetProgressMessageID(messageID string) {
	t.ProgressMessageID = strings.TrimSpace(messageID)
}

func (t *Task) SetResultMessageID(messageID string) {
	t.ResultMessageID = strings.TrimSpace(messageID)
}

func (t *Task) MarkTranslating(at time.Time) error {
	if t.Status != StatusQueued {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusTranslating)
	}
	if err := t.Timeline.MarkTranslating(at); err != nil {
		return err
	}
	t.Status = StatusTranslating
	return nil
}

func (t *Task) MarkDrawing(at time.Time) error {
	if t.Status != StatusTranslating {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusDrawing)
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return fmt.Errorf("task prompt is required before drawing")
	}
	if err := t.Timeline.MarkDrawing(at); err != nil {
		return err
	}
	t.Status = StatusDrawing
	return nil
}

func (t *Task) MarkCompleted(image string, at time.Time) error {
	image = strings.TrimSpace(image)
	if t.Status != StatusDrawing {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusCompleted)
	}
	if image == "" {
		return fmt.Errorf("task image is required")
	}
	if err := t.Timeline.MarkCompleted(at); err != nil {
		return err
	}
	t.Image = image
	t.Status = StatusCompleted
	return nil
}

func (t *Task) MarkFailed(taskError TaskError, at time.Time) error {
	if t.Status.IsTerminal() {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusFailed)
	}
	if err := taskError.Validate(); err != nil {
		return err
	}
	if err := t.Timeline.MarkFailed(at); err != nil {
		return err
	}
	t.Status = StatusFailed
	t.Error = &taskError
	return nil
}

func (t *Task) MarkStopped(at time.Time) error {
	if t.Status.IsTerminal() {
		return fmt.Errorf("cannot move from %s to %s", t.Status, StatusStopped)
	}
	if err := t.Timeline.MarkStopped(at); err != nil {
		return err
	}
	t.Status = StatusStopped
	return nil
}

func (t Task) validateRestoredState() error {
	if t.Timeline.DrawingStartedAt != nil && t.Timeline.TranslatingStartedAt == nil {
		return fmt.Errorf("drawing_started_at requires translating_started_at")
	}
	if t.Timeline.CompletedAt != nil && (t.Timeline.TranslatingStartedAt == nil || t.Timeline.DrawingStartedAt == nil) {
		return fmt.Errorf("completed_at requires translating_started_at and drawing_started_at")
	}

	switch {
	case t.Status == StatusCompleted && strings.TrimSpace(t.Image) == "":
		return fmt.Errorf("task image is required")
	case t.Status != StatusCompleted && strings.TrimSpace(t.Image) != "":
		return fmt.Errorf("task image is only allowed for completed tasks")
	}

	switch {
	case (t.Status == StatusDrawing || t.Status == StatusCompleted) && strings.TrimSpace(t.Prompt) == "":
		return fmt.Errorf("task prompt is required before drawing")
	case t.Timeline.DrawingStartedAt != nil && strings.TrimSpace(t.Prompt) == "":
		return fmt.Errorf("task prompt is required once drawing has started")
	}

	switch {
	case t.Status == StatusFailed && t.Error == nil:
		return fmt.Errorf("task error is required for failed tasks")
	case t.Status != StatusFailed && t.Error != nil:
		return fmt.Errorf("task error is only allowed for failed tasks")
	}

	if err := validateTimelineForStatus(t.Status, t.Timeline); err != nil {
		return err
	}
	return nil
}

func validateTimelineForStatus(status Status, timeline Timeline) error {
	switch status {
	case StatusQueued:
		if timeline.TranslatingStartedAt != nil || timeline.DrawingStartedAt != nil || timeline.CompletedAt != nil || timeline.FailedAt != nil || timeline.StoppedAt != nil {
			return fmt.Errorf("queued task timeline must not contain transition timestamps")
		}
	case StatusTranslating:
		if timeline.TranslatingStartedAt == nil {
			return fmt.Errorf("translating_started_at is required")
		}
		if timeline.DrawingStartedAt != nil || timeline.CompletedAt != nil || timeline.FailedAt != nil || timeline.StoppedAt != nil {
			return fmt.Errorf("translating task timeline contains invalid terminal timestamps")
		}
	case StatusDrawing:
		if timeline.TranslatingStartedAt == nil {
			return fmt.Errorf("translating_started_at is required")
		}
		if timeline.DrawingStartedAt == nil {
			return fmt.Errorf("drawing_started_at is required")
		}
		if timeline.CompletedAt != nil || timeline.FailedAt != nil || timeline.StoppedAt != nil {
			return fmt.Errorf("drawing task timeline contains invalid terminal timestamps")
		}
	case StatusCompleted:
		if timeline.TranslatingStartedAt == nil {
			return fmt.Errorf("translating_started_at is required")
		}
		if timeline.DrawingStartedAt == nil {
			return fmt.Errorf("drawing_started_at is required")
		}
		if timeline.CompletedAt == nil {
			return fmt.Errorf("completed_at is required")
		}
		if timeline.FailedAt != nil || timeline.StoppedAt != nil {
			return fmt.Errorf("completed task timeline contains invalid terminal timestamps")
		}
	case StatusFailed:
		if timeline.FailedAt == nil {
			return fmt.Errorf("failed_at is required")
		}
		if timeline.CompletedAt != nil || timeline.StoppedAt != nil {
			return fmt.Errorf("failed task timeline contains invalid terminal timestamps")
		}
	case StatusStopped:
		if timeline.StoppedAt == nil {
			return fmt.Errorf("stopped_at is required")
		}
		if timeline.CompletedAt != nil || timeline.FailedAt != nil {
			return fmt.Errorf("stopped task timeline contains invalid terminal timestamps")
		}
	}
	return nil
}
