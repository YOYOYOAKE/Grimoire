package conversation

type Service struct {
	model ConversationModel
}

func NewService(model ConversationModel) *Service {
	return &Service{model: model}
}
