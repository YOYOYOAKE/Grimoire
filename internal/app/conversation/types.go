package conversation

import (
	domainpreferences "grimoire/internal/domain/preferences"
	domainsession "grimoire/internal/domain/session"
)

type ConverseCommand struct {
	Summary    domainsession.Summary
	Messages   []domainsession.Message
	Preference domainpreferences.Preference
}

type ConverseResult struct {
	Reply   string
	Summary domainsession.Summary
}
