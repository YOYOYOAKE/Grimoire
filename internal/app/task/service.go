package task

type Service struct {
	tasks    TaskRepository
	txRunner TxRunner
}

func NewService(tasks TaskRepository, txRunner TxRunner) *Service {
	return &Service{
		tasks:    tasks,
		txRunner: txRunner,
	}
}
