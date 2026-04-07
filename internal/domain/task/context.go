package task

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Context struct {
	raw string
}

func NewContext(raw string) (Context, error) {
	raw = strings.TrimSpace(raw)
	context := Context{raw: raw}
	if err := context.Validate(); err != nil {
		return Context{}, err
	}
	return context, nil
}

func (c Context) Raw() string {
	return c.raw
}

func (c Context) Validate() error {
	if strings.TrimSpace(c.raw) == "" {
		return fmt.Errorf("task context is required")
	}
	if !json.Valid([]byte(c.raw)) {
		return fmt.Errorf("task context must be valid json")
	}
	return nil
}
