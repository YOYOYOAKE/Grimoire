package task

import (
	"fmt"
	"strings"
)

type TaskError struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

func NewError(code string, stage string, message string) (TaskError, error) {
	err := TaskError{
		Code:    strings.TrimSpace(code),
		Stage:   strings.TrimSpace(stage),
		Message: strings.TrimSpace(message),
	}
	if validateErr := err.Validate(); validateErr != nil {
		return TaskError{}, validateErr
	}
	return err, nil
}

func (e TaskError) Validate() error {
	if strings.TrimSpace(e.Code) == "" {
		return fmt.Errorf("task error code is required")
	}
	if strings.TrimSpace(e.Stage) == "" {
		return fmt.Errorf("task error stage is required")
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Errorf("task error message is required")
	}
	return nil
}
