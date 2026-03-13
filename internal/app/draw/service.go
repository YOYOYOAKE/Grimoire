package draw

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domaindraw "grimoire/internal/domain/draw"
)

type SubmitCommand struct {
	ChatID           int64
	Prompt           string
	RequestMessageID int64
}

type Service struct {
	tasks       TaskRepository
	preferences PreferenceRepository
	translator  PromptTranslator
	generator   ImageGenerator
	notifier    Notifier
	now         func() time.Time
	idGenerator func() string
	logger      *slog.Logger
	scheduler   Scheduler
}

func NewService(
	tasks TaskRepository,
	preferences PreferenceRepository,
	translator PromptTranslator,
	generator ImageGenerator,
	notifier Notifier,
	now func() time.Time,
	idGenerator func() string,
	logger *slog.Logger,
) *Service {
	if now == nil {
		now = time.Now
	}
	if idGenerator == nil {
		idGenerator = func() string { return "" }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		tasks:       tasks,
		preferences: preferences,
		translator:  translator,
		generator:   generator,
		notifier:    notifier,
		now:         now,
		idGenerator: idGenerator,
		logger:      logger,
	}
}

func (s *Service) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
}

func (s *Service) Submit(ctx context.Context, command SubmitCommand) (domaindraw.Task, error) {
	if s.scheduler == nil {
		return domaindraw.Task{}, fmt.Errorf("scheduler is not configured")
	}

	shape, artists, err := s.preferenceSnapshot()
	if err != nil {
		return domaindraw.Task{}, err
	}

	task, err := domaindraw.NewTask(
		s.idGenerator(),
		command.ChatID,
		command.RequestMessageID,
		command.Prompt,
		shape,
		artists,
		s.now(),
	)
	if err != nil {
		return domaindraw.Task{}, err
	}
	task.Status = domaindraw.StatusQueued

	if err := s.tasks.Create(ctx, task); err != nil {
		return domaindraw.Task{}, err
	}

	s.logger.Info(
		"task queueing",
		"task_id", task.ID,
		"chat_id", task.ChatID,
		"prompt", task.RequestText,
		"shape", task.Shape,
		"artists", task.Artists,
	)

	statusMessageID, err := s.notifier.SendText(ctx, task.ChatID, task.RequestMessageID, queuedText())
	if err != nil {
		s.logger.Warn("send queued status failed", "task_id", task.ID, "error", err)
		s.enqueueTask(task)
		return task, nil
	}

	task.SetStatusMessageID(statusMessageID)
	if err := s.tasks.Update(ctx, task); err != nil {
		s.logger.Warn("update queued task status message failed", "task_id", task.ID, "error", err)
	}
	s.enqueueTask(task)
	return task, nil
}

func (s *Service) Process(ctx context.Context, taskID string) error {
	task, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil
		}
		return err
	}

	if err := task.MarkTranslating(s.now()); err != nil {
		return err
	}
	if err := s.persistAndNotify(ctx, &task, translatingText()); err != nil {
		return err
	}

	translation, err := s.translator.Translate(ctx, task.RequestText, task.Shape)
	if err != nil {
		return s.failTask(ctx, &task, fmt.Sprintf("LLM 翻译失败: %v", err))
	}
	task.SetTranslation(mergeArtists(task.Artists, translation.Prompt), translation.NegativePrompt)

	if err := task.MarkGenerating(s.now()); err != nil {
		return err
	}
	if err := s.persistAndNotify(ctx, &task, drawingText()); err != nil {
		return err
	}

	image, err := s.generator.Generate(ctx, domaindraw.GenerateRequest{
		Prompt:         task.Prompt,
		NegativePrompt: task.NegativePrompt,
		Characters:     translation.Characters,
		Shape:          task.Shape,
	})
	if err != nil {
		return s.failTask(ctx, &task, fmt.Sprintf("生成图像失败: %v", err))
	}

	if err := s.notifier.SendPhoto(ctx, task.ChatID, task.RequestMessageID, task.ID+".png", "", image); err != nil {
		return s.failTask(ctx, &task, fmt.Sprintf("发送图片失败: %v", err))
	}
	s.logger.Info(
		"task image sent",
		"task_id", task.ID,
		"chat_id", task.ChatID,
		"reply_to_message_id", task.RequestMessageID,
	)
	s.deleteStatusMessage(ctx, task)
	if err := task.MarkCompleted(s.now()); err != nil {
		return err
	}
	return s.deleteTask(ctx, task.ID)
}

func (s *Service) preferenceSnapshot() (domaindraw.Shape, string, error) {
	preference, err := s.preferences.Get()
	if err != nil {
		return "", "", err
	}
	return preference.Shape, preference.Artists, nil
}

func (s *Service) enqueueTask(task domaindraw.Task) {
	position := s.scheduler.Enqueue(task.ID)
	s.logger.Info("task enqueued", "task_id", task.ID, "queue_position", position)
}

func (s *Service) persistAndNotify(ctx context.Context, task *domaindraw.Task, text string) error {
	if err := s.tasks.Update(ctx, *task); err != nil {
		return err
	}
	if err := s.upsertStatus(ctx, task, text); err != nil {
		s.logger.Warn("notify task status failed", "task_id", task.ID, "status", task.Status, "error", err)
	}
	return nil
}

func (s *Service) upsertStatus(ctx context.Context, task *domaindraw.Task, text string) error {
	if task.StatusMessageID > 0 {
		if err := s.notifier.EditText(ctx, task.ChatID, task.StatusMessageID, text); err != nil {
			s.logger.Warn("edit status message failed", "task_id", task.ID, "message_id", task.StatusMessageID, "error", err)
		}
		return nil
	}

	messageID, err := s.notifier.SendText(ctx, task.ChatID, task.RequestMessageID, text)
	if err != nil {
		return err
	}
	task.SetStatusMessageID(messageID)
	return s.tasks.Update(ctx, *task)
}

func (s *Service) failTask(ctx context.Context, task *domaindraw.Task, reason string) error {
	if err := task.MarkFailed(reason, s.now()); err != nil {
		return err
	}
	s.logger.Error(
		"task failed",
		"task_id", task.ID,
		"chat_id", task.ChatID,
		"request_message_id", task.RequestMessageID,
		"reason", task.ErrorText,
	)
	if err := s.upsertStatus(ctx, task, failedText(task.ErrorText)); err != nil {
		s.logger.Warn("send failed status failed", "task_id", task.ID, "error", err)
	}
	return s.deleteTask(ctx, task.ID)
}

func (s *Service) deleteTask(ctx context.Context, taskID string) error {
	if err := s.tasks.Delete(ctx, taskID); err != nil && !errors.Is(err, ErrTaskNotFound) {
		return err
	}
	return nil
}

func (s *Service) deleteStatusMessage(ctx context.Context, task domaindraw.Task) {
	if task.StatusMessageID == 0 {
		return
	}
	if err := s.notifier.DeleteMessage(ctx, task.ChatID, task.StatusMessageID); err != nil {
		s.logger.Warn("delete status message failed", "task_id", task.ID, "message_id", task.StatusMessageID, "error", err)
	}
}

func mergeArtists(artists string, prompt string) string {
	artists = strings.TrimSpace(artists)
	prompt = strings.TrimSpace(prompt)
	switch {
	case artists == "":
		return prompt
	case prompt == "":
		return artists
	default:
		return artists + ", " + prompt
	}
}
