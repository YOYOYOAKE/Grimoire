package access

type Service struct {
	users UserRepository
}

func NewService(users UserRepository) *Service {
	return &Service{users: users}
}
