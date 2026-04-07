package runner

type Service struct {
	translator PromptTranslator
	generator  ImageGenerator
	imageStore ImageStore
	notifier   Notifier
}

func NewService(
	translator PromptTranslator,
	generator ImageGenerator,
	imageStore ImageStore,
	notifier Notifier,
) *Service {
	return &Service{
		translator: translator,
		generator:  generator,
		imageStore: imageStore,
		notifier:   notifier,
	}
}
