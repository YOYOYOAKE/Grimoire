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
	tasks        TaskRepository
	preferences  PreferenceRepository
	translator   PromptTranslator
	generator    ImageGenerator
	notifier     Notifier
	now          func() time.Time
	idGenerator  func() string
	pollInterval time.Duration
	logger       *slog.Logger
	scheduler    Scheduler
}

func NewService(
	tasks TaskRepository,
	preferences PreferenceRepository,
	translator PromptTranslator,
	generator ImageGenerator,
	notifier Notifier,
	now func() time.Time,
	idGenerator func() string,
	pollInterval time.Duration,
	logger *slog.Logger,
) *Service {
	if now == nil {
		now = time.Now
	}
	if idGenerator == nil {
		idGenerator = func() string { return "" }
	}
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		tasks:        tasks,
		preferences:  preferences,
		translator:   translator,
		generator:    generator,
		notifier:     notifier,
		now:          now,
		idGenerator:  idGenerator,
		pollInterval: pollInterval,
		logger:       logger,
	}
}

func (s *Service) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
}

func (s *Service) Submit(ctx context.Context, command SubmitCommand) (domaindraw.Task, error) {
	if s.scheduler == nil {
		return domaindraw.Task{}, fmt.Errorf("scheduler is not configured")
	}

	shape, artist, err := s.preferenceSnapshot()
	if err != nil {
		return domaindraw.Task{}, err
	}

	task, err := domaindraw.NewTask(
		s.idGenerator(),
		command.ChatID,
		command.RequestMessageID,
		command.Prompt,
		shape,
		artist,
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
		"artists", task.Artist,
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
	task.SetTranslation(mergeArtist(task.Artist, translation.Prompt), translation.NegativePrompt)

	if err := task.MarkSubmitting(s.now()); err != nil {
		return err
	}
	if err := s.tasks.Update(ctx, task); err != nil {
		return err
	}

	jobID, err := s.generator.Submit(ctx, domaindraw.GenerateRequest{
		Prompt:         task.Prompt,
		NegativePrompt: task.NegativePrompt,
		Characters:     translation.Characters,
		Shape:          task.Shape,
		Artists:        task.Artist,
	})
	if err != nil {
		return s.failTask(ctx, &task, fmt.Sprintf("提交绘图任务失败: %v", err))
	}

	if err := task.MarkPolling(jobID, s.now()); err != nil {
		return err
	}
	if err := s.persistAndNotify(ctx, &task, drawingText(0)); err != nil {
		return err
	}

	lastDetail := ""
	for {
		update, err := s.generator.Poll(ctx, task.ProviderJobID)
		if err != nil {
			return s.failTask(ctx, &task, fmt.Sprintf("轮询失败: %v", err))
		}
		s.logger.Info(
			"task poll updated",
			"task_id", task.ID,
			"provider_job_id", task.ProviderJobID,
			"status", update.Status,
			"queue_position", update.QueuePosition,
		)

		switch update.Status {
		case domaindraw.JobQueued:
			detail := fmt.Sprintf("queued:%d", update.QueuePosition)
			if detail != lastDetail {
				lastDetail = detail
				if err := s.upsertStatus(ctx, &task, drawingText(update.QueuePosition)); err != nil {
					s.logger.Warn("update queued poll status failed", "task_id", task.ID, "error", err)
				}
			}
		case domaindraw.JobProcessing:
			if lastDetail != "processing" {
				lastDetail = "processing"
				if err := s.upsertStatus(ctx, &task, drawingText(update.QueuePosition)); err != nil {
					s.logger.Warn("update processing poll status failed", "task_id", task.ID, "error", err)
				}
			}
		case domaindraw.JobCompleted:
			if err := s.notifier.SendPhoto(ctx, task.ChatID, task.RequestMessageID, task.ID+".png", "", update.Image); err != nil {
				return s.failTask(ctx, &task, fmt.Sprintf("发送图片失败: %v", err))
			}
			s.logger.Info(
				"task image sent",
				"task_id", task.ID,
				"chat_id", task.ChatID,
				"reply_to_message_id", task.RequestMessageID,
				"provider_job_id", task.ProviderJobID,
			)
			s.deleteStatusMessage(ctx, task)
			if err := task.MarkCompleted(s.now()); err != nil {
				return err
			}
			return s.deleteTask(ctx, task.ID)
		case domaindraw.JobFailed:
			reason := strings.TrimSpace(update.Error)
			if reason == "" {
				reason = "图像生成失败"
			}
			return s.failTask(ctx, &task, reason)
		default:
			return s.failTask(ctx, &task, fmt.Sprintf("未知任务状态: %s", update.Status))
		}

		select {
		case <-ctx.Done():
			return s.failTask(context.Background(), &task, "任务处理中断")
		case <-time.After(s.pollInterval):
		}
	}
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

func mergeArtist(artist string, prompt string) string {
	artist = strings.TrimSpace(artist)
	prompt = strings.TrimSpace(prompt)
	switch {
	case artist == "":
		return prompt
	case prompt == "":
		return artist
	default:
		return artist + ", " + prompt
	}
}
