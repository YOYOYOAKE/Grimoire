package access

import "errors"

var ErrUserNotFound = errors.New("user not found")

const (
	ReasonUserNotFound = "user_not_found"
	ReasonUserBanned   = "user_banned"
)

type CheckCommand struct {
	TelegramID string
}

type Decision struct {
	Allowed bool
	Reason  string
}
