package request

import (
	"strings"

	domainpreferences "grimoire/internal/domain/preferences"
)

const (
	RequestConfirmPrefix = "request:confirm:"
	RequestRevisePrefix  = "request:revise:"
)

type DecisionKind string

const (
	DecisionKindNone    DecisionKind = ""
	DecisionKindConfirm DecisionKind = "confirm"
	DecisionKindRevise  DecisionKind = "revise"
)

type GenerateCommand struct {
	SessionID  string
	Preference domainpreferences.Preference
}

type PendingRequest struct {
	Request       string
	ConfirmAction Action
	ReviseAction  Action
}

type Action struct {
	Kind         DecisionKind
	SessionID    string
	CallbackData string
}

type ResolveDecisionCommand struct {
	SessionID    string
	CallbackData string
	Text         string
}

type Decision struct {
	Kind      DecisionKind
	SessionID string
}

func newAction(kind DecisionKind, sessionID string) Action {
	sessionID = strings.TrimSpace(sessionID)
	prefix := RequestConfirmPrefix
	if kind == DecisionKindRevise {
		prefix = RequestRevisePrefix
	}
	return Action{
		Kind:         kind,
		SessionID:    sessionID,
		CallbackData: prefix + sessionID,
	}
}
