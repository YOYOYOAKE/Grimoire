package app_test

import (
	"context"
	"testing"

	accessapp "grimoire/internal/app/access"
	conversationapp "grimoire/internal/app/conversation"
	recoveryapp "grimoire/internal/app/recovery"
	requestapp "grimoire/internal/app/request"
	runnerapp "grimoire/internal/app/runner"
	sessionapp "grimoire/internal/app/session"
	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainsession "grimoire/internal/domain/session"
	domaintask "grimoire/internal/domain/task"
	domainuser "grimoire/internal/domain/user"
)

type userRepositoryStub struct{}

func (userRepositoryStub) GetByTelegramID(context.Context, string) (domainuser.User, error) {
	return domainuser.User{}, nil
}

type sessionRepositoryStub struct{}

func (sessionRepositoryStub) GetOrCreateActiveByUserID(context.Context, string) (domainsession.Session, error) {
	return domainsession.Session{}, nil
}

func (sessionRepositoryStub) Get(context.Context, string) (domainsession.Session, error) {
	return domainsession.Session{}, nil
}

func (sessionRepositoryStub) Save(context.Context, domainsession.Session) error {
	return nil
}

type sessionMessageRepositoryStub struct{}

func (sessionMessageRepositoryStub) Append(context.Context, domainsession.Message) error {
	return nil
}

func (sessionMessageRepositoryStub) ListRecent(context.Context, string, int) ([]domainsession.Message, error) {
	return nil, nil
}

type taskRepositoryStub struct{}

func (taskRepositoryStub) Create(context.Context, domaintask.Task) error {
	return nil
}

func (taskRepositoryStub) Get(context.Context, string) (domaintask.Task, error) {
	return domaintask.Task{}, nil
}

func (taskRepositoryStub) Update(context.Context, domaintask.Task) error {
	return nil
}

func (taskRepositoryStub) ListRecoverable(context.Context) ([]domaintask.Task, error) {
	return nil, nil
}

func (taskRepositoryStub) ListBySourceTask(context.Context, string) ([]domaintask.Task, error) {
	return nil, nil
}

type txRunnerStub struct{}

func (txRunnerStub) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type schedulerStub struct{}

func (schedulerStub) Enqueue(string) error {
	return nil
}

type conversationModelStub struct{}

func (conversationModelStub) Converse(context.Context, conversationapp.ConversationInput) (conversationapp.ConversationOutput, error) {
	return conversationapp.ConversationOutput{}, nil
}

type requestGeneratorStub struct{}

func (requestGeneratorStub) Generate(context.Context, requestapp.GenerateInput) (string, error) {
	return "", nil
}

type promptTranslatorStub struct{}

func (promptTranslatorStub) Translate(context.Context, string, domaindraw.Shape) (domaindraw.Translation, error) {
	return domaindraw.Translation{}, nil
}

type imageGeneratorStub struct{}

func (imageGeneratorStub) Generate(context.Context, domaindraw.GenerateRequest) ([]byte, error) {
	return nil, nil
}

type imageStoreStub struct{}

func (imageStoreStub) Save(context.Context, string, string, []byte) (string, error) {
	return "", nil
}

type notifierStub struct{}

func (notifierStub) SendText(context.Context, string, string, runnerapp.MessageOptions) (string, error) {
	return "", nil
}

func (notifierStub) EditText(context.Context, string, string, string, runnerapp.MessageOptions) error {
	return nil
}

func (notifierStub) SendImage(context.Context, string, string, string, runnerapp.MessageOptions) (string, error) {
	return "", nil
}

func (notifierStub) DeleteMessage(context.Context, string, string) error {
	return nil
}

func TestPortStubsSatisfyContracts(t *testing.T) {
	var _ accessapp.UserRepository = userRepositoryStub{}
	var _ sessionapp.SessionRepository = sessionRepositoryStub{}
	var _ sessionapp.SessionMessageRepository = sessionMessageRepositoryStub{}
	var _ sessionapp.TxRunner = txRunnerStub{}
	var _ conversationapp.SessionRepository = sessionRepositoryStub{}
	var _ conversationapp.SessionMessageRepository = sessionMessageRepositoryStub{}
	var _ conversationapp.TxRunner = txRunnerStub{}
	var _ requestapp.SessionRepository = sessionRepositoryStub{}
	var _ requestapp.SessionMessageRepository = sessionMessageRepositoryStub{}
	var _ taskapp.TaskRepository = taskRepositoryStub{}
	var _ taskapp.TxRunner = txRunnerStub{}
	var _ taskapp.Scheduler = schedulerStub{}
	var _ runnerapp.TaskRepository = taskRepositoryStub{}
	var _ runnerapp.TxRunner = txRunnerStub{}
	var _ recoveryapp.TaskRepository = taskRepositoryStub{}
	var _ recoveryapp.Scheduler = schedulerStub{}
	var _ conversationapp.ConversationModel = conversationModelStub{}
	var _ requestapp.RequestGenerator = requestGeneratorStub{}
	var _ runnerapp.PromptTranslator = promptTranslatorStub{}
	var _ runnerapp.ImageGenerator = imageGeneratorStub{}
	var _ runnerapp.ImageStore = imageStoreStub{}
	var _ runnerapp.Notifier = notifierStub{}
}
