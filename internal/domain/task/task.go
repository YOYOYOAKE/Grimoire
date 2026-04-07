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
