package request

import (
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type GenerateCommand struct {
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type PendingRequest struct {
	Request string
}
