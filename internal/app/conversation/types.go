package conversation

import (
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type ConverseCommand struct {
	SessionID  string
	Preference domainpreferences.Preference
}

type ConverseResult struct {
	Reply             string
	Summary           domainsession.Summary
	CreateDrawingTask *CreateDrawingTask
}
