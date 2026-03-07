package memory

import "github.com/google/uuid"

func NewTaskID() string {
	return uuid.NewString()
}
