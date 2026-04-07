package chat

type HandleTextCommand struct {
	UserID string
	Text   string
}

type HandleTextResult struct {
	Reply string
}
