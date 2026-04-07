package session

type Service struct {
	sessions SessionRepository
	messages SessionMessageRepository
}

func NewService(sessions SessionRepository, messages SessionMessageRepository) *Service {
	return &Service{
		sessions: sessions,
		messages: messages,
	}
}
