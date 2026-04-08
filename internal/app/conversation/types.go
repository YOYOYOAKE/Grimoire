package conversation

type ConverseCommand struct {
	SessionID string
}

type ConverseResult struct {
	Reply             string
	CreateDrawingTask *CreateDrawingTask
}
