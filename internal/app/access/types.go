package access

type CheckCommand struct {
	TelegramID string
}

type Decision struct {
	Allowed bool
	Reason  string
}
