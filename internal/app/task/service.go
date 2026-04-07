package task

type Service struct {
	tasks    TaskRepository
	txRunner TxRunner
	schedule Scheduler
}

func NewService(tasks TaskRepository, txRunner TxRunner, scheduler Scheduler) *Service {
	return &Service{
		tasks:    tasks,
		txRunner: txRunner,
		schedule: scheduler,
	}
}
