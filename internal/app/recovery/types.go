package recovery

type RecoverCommand struct{}

type RecoverResult struct {
	RequeuedTaskIDs []string
}
