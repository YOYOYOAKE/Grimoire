package conversation

import domainsession "grimoire/internal/domain/session"

type ConverseCommand struct {
	SessionID string
}

type ConverseResult struct {
	Reply             string
	Summary           domainsession.Summary
	CreateDrawingTask *CreateDrawingTask
}
