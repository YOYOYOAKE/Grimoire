package chat

import (
	"encoding/json"
	"fmt"

	taskapp "grimoire/internal/app/task"
	domainpreferences "grimoire/internal/domain/preferences"
	domaintask "grimoire/internal/domain/task"
)

func buildTaskContext(preference domainpreferences.Preference) (domaintask.Context, error) {
	payload, err := json.Marshal(struct {
		Version int    `json:"version"`
		Shape   string `json:"shape"`
		Artists string `json:"artists,omitempty"`
	}{
		Version: 1,
		Shape:   string(preference.Shape),
		Artists: preference.Artists,
	})
	if err != nil {
		return domaintask.Context{}, fmt.Errorf("marshal task context: %w", err)
	}
	return domaintask.NewContext(string(payload))
}

func taskCreateCommand(userID string, sessionID string, request string, context domaintask.Context) taskapp.CreateCommand {
	return taskapp.CreateCommand{
		UserID:    userID,
		SessionID: sessionID,
		Request:   request,
		Context:   context,
	}
}
