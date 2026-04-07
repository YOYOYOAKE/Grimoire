package task

type Status string

const (
	StatusQueued      Status = "queued"
	StatusTranslating Status = "translating"
	StatusDrawing     Status = "drawing"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusStopped     Status = "stopped"
)

func (s Status) Valid() bool {
	switch s {
	case StatusQueued, StatusTranslating, StatusDrawing, StatusCompleted, StatusFailed, StatusStopped:
		return true
	default:
		return false
	}
}

func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusStopped:
		return true
	default:
		return false
	}
}
