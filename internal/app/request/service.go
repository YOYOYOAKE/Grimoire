package request

type Service struct {
	generator RequestGenerator
}

func NewService(generator RequestGenerator) *Service {
	return &Service{generator: generator}
}
