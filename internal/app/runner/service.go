package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	domaindraw "grimoire/internal/domain/draw"
	domaintask "grimoire/internal/domain/task"
)

var (
	ErrTaskRepositoryRequired = errors.New("task repository is required")
	ErrTxRunnerRequired       = errors.New("tx runner is required")
	ErrTranslatorRequired     = errors.New("prompt translator is required")
	ErrGeneratorRequired      = errors.New("image generator is required")
	ErrImageStoreRequired     = errors.New("image store is required")
	ErrNotifierRequired       = errors.New("notifier is required")
)

type executionContext struct {
	Shape   domaindraw.Shape
	Artists string
}

type Service struct {
	tasks      TaskRepository
	txRunner   TxRunner
	translator PromptTranslator
	generator  ImageGenerator
	imageStore ImageStore
	notifier   Notifier
	now        func() time.Time
	logger     *slog.Logger
}

func NewService(
	tasks TaskRepository,
	txRunner TxRunner,
	translator PromptTranslator,
	generator ImageGenerator,
	imageStore ImageStore,
	notifier Notifier,
	now func() time.Time,
	logger *slog.Logger,
) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		tasks:      tasks,
		txRunner:   txRunner,
		translator: translator,
		generator:  generator,
		imageStore: imageStore,
		notifier:   notifier,
		now:        now,
		logger:     logger,
	}
}

func (s *Service) Get(ctx context.Context, taskID string) (domaintask.Task, error) {
	if s.tasks == nil {
		return domaintask.Task{}, ErrTaskRepositoryRequired
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return domaintask.Task{}, fmt.Errorf("task id is required")
	}
	return s.tasks.Get(ctx, taskID)
}

func (s *Service) Run(ctx context.Context, command RunCommand) error {
	s.logger.Info("runner run started", "task_id", command.TaskID)
	if err := s.requireRunDependencies(); err != nil {
		return err
	}

	task, err := s.Get(ctx, command.TaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.logger.Info("runner task missing, skipping", "task_id", command.TaskID)
			return nil
		}
		return err
	}
	s.logger.Info("runner task loaded", runnerTaskAttrs(task)...)
	switch task.Status {
	case domaintask.StatusCompleted, domaintask.StatusFailed:
		s.logger.Info("runner task already terminal, skipping", runnerTaskAttrs(task)...)
		return nil
	case domaintask.StatusStopped:
		s.upsertProgressBestEffort(ctx, &task, stoppedText(), false)
		s.logger.Info("runner task already stopped, skipping", runnerTaskAttrs(task)...)
		return nil
	}

	execContext, err := parseExecutionContext(task.Context)
	if err != nil {
		return s.failTask(ctx, task, "INVALID_CONTEXT", "preparing", err)
	}
	s.logger.Info(
		"runner execution context parsed",
		append(runnerTaskAttrs(task),
			"shape", execContext.Shape,
			"artists", execContext.Artists,
		)...,
	)

	if task.Status == domaintask.StatusQueued {
		s.upsertProgressBestEffort(ctx, &task, queuedText(), true)
		task, err = s.StartTranslating(ctx, RunCommand{TaskID: task.ID})
		if err != nil {
			return err
		}
		s.logger.Info("runner task entered translating", runnerTaskAttrs(task)...)
		s.upsertProgressBestEffort(ctx, &task, translatingText(), true)
	}

	var translation *domaindraw.Translation
	if task.Status == domaintask.StatusTranslating {
		if strings.TrimSpace(task.Prompt) == "" {
			s.logger.Info(
				"runner translate requested",
				append(runnerTaskAttrs(task),
					"shape", execContext.Shape,
				)...,
			)
			translated, err := s.translator.Translate(ctx, task.Request, execContext.Shape)
			if err != nil {
				return s.failTask(ctx, task, "PROMPT_TRANSLATE_FAILED", "translating", err)
			}
			if err := translated.Validate(); err != nil {
				return s.failTask(ctx, task, "PROMPT_TRANSLATE_FAILED", "translating", err)
			}
			s.logger.Info(
				"runner translate succeeded",
				append(runnerTaskAttrs(task),
					"translated_prompt", translated.Prompt,
					"negative_prompt", translated.NegativePrompt,
					"characters", translated.Characters,
				)...,
			)
			stopped, stopRequested, err := s.latestIfStopped(ctx, task.ID)
			if err != nil {
				return err
			}
			if stopRequested {
				s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
				s.logger.Info("runner task stop detected after translate", runnerTaskAttrs(stopped)...)
				return nil
			}

			task, err = s.StartDrawing(ctx, StartDrawingCommand{
				TaskID: task.ID,
				Prompt: mergeArtists(execContext.Artists, translated.Prompt),
			})
			if err != nil {
				return err
			}
			translation = &translated
		} else {
			s.logger.Info("runner drawing using existing prompt", runnerTaskAttrs(task)...)
			task, err = s.StartDrawing(ctx, StartDrawingCommand{TaskID: task.ID})
			if err != nil {
				return err
			}
		}
		s.logger.Info("runner task entered drawing", runnerTaskAttrs(task)...)
		s.upsertProgressBestEffort(ctx, &task, drawingText(), true)
	}

	if task.Status == domaintask.StatusStopped {
		s.upsertProgressBestEffort(ctx, &task, stoppedText(), false)
		s.logger.Info("runner task stopped before drawing request", runnerTaskAttrs(task)...)
		return nil
	}
	if task.Status != domaintask.StatusDrawing {
		s.logger.Info("runner task not in drawing status after preparation", runnerTaskAttrs(task)...)
		return nil
	}

	request := domaindraw.GenerateRequest{
		Prompt: task.Prompt,
		Shape:  execContext.Shape,
	}
	if translation != nil {
		request.NegativePrompt = translation.NegativePrompt
		request.Characters = translation.Characters
	}
	s.logger.Info(
		"runner image generation requested",
		append(runnerTaskAttrs(task),
			"generate_prompt", request.Prompt,
			"shape", request.Shape,
			"negative_prompt", request.NegativePrompt,
			"characters", request.Characters,
		)...,
	)

	imageContent, err := s.generator.Generate(ctx, request)
	if err != nil {
		return s.failTask(ctx, task, "IMAGE_GENERATE_FAILED", "drawing", err)
	}
	s.logger.Info(
		"runner image generation succeeded",
		append(runnerTaskAttrs(task),
			"image_bytes", len(imageContent),
		)...,
	)
	stopped, stopRequested, err := s.latestIfStopped(ctx, task.ID)
	if err != nil {
		return err
	}
	if stopRequested {
		s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
		s.logger.Info("runner task stop detected after image generation", runnerTaskAttrs(stopped)...)
		return nil
	}

	imagePath, err := s.imageStore.Save(ctx, task.UserID, task.ID, imageContent)
	if err != nil {
		return s.failTask(ctx, task, "IMAGE_STORE_FAILED", "drawing", err)
	}
	s.logger.Info(
		"runner image stored",
		append(runnerTaskAttrs(task),
			"image_path", imagePath,
		)...,
	)
	stopped, stopRequested, err = s.latestIfStopped(ctx, task.ID)
	if err != nil {
		return err
	}
	if stopRequested {
		s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
		s.logger.Info("runner task stop detected after image store", runnerTaskAttrs(stopped)...)
		return nil
	}

	resultMessageID, err := s.notifier.SendImage(ctx, task.UserID, imagePath, "", MessageOptions{
		TaskID:  task.ID,
		Variant: MessageVariantResult,
	})
	if err != nil {
		return s.failTask(ctx, task, "SEND_RESULT_FAILED", "notifying", err)
	}
	s.logger.Info(
		"runner result notification sent",
		append(runnerTaskAttrs(task),
			"image_path", imagePath,
			"sent_result_message_id", resultMessageID,
		)...,
	)

	task, err = s.completeResult(ctx, task.ID, imagePath, resultMessageID)
	if err != nil {
		return err
	}
	s.deleteProgressBestEffort(ctx, task)
	s.logger.Info("runner task completed", runnerTaskAttrs(task)...)
	return nil
}

func (s *Service) StartTranslating(ctx context.Context, command RunCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkTranslating(s.now())
	})
}

func (s *Service) StartDrawing(ctx context.Context, command StartDrawingCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		if prompt := strings.TrimSpace(command.Prompt); prompt != "" {
			if err := task.SetPrompt(prompt); err != nil {
				return err
			}
		}
		return task.MarkDrawing(s.now())
	})
}

func (s *Service) Complete(ctx context.Context, command CompleteCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkCompleted(command.Image, s.now())
	})
}

func (s *Service) Fail(ctx context.Context, command FailCommand) (domaintask.Task, error) {
	return s.updateTask(ctx, command.TaskID, func(task *domaintask.Task) error {
		return task.MarkFailed(command.Error, s.now())
	})
}

func (s *Service) updateTask(
	ctx context.Context,
	taskID string,
	update func(task *domaintask.Task) error,
) (domaintask.Task, error) {
	if s.tasks == nil {
		return domaintask.Task{}, ErrTaskRepositoryRequired
	}
	if s.txRunner == nil {
		return domaintask.Task{}, ErrTxRunnerRequired
	}

	var mutated domaintask.Task
	err := s.txRunner.WithinTx(ctx, func(txCtx context.Context) error {
		task, err := s.Get(txCtx, taskID)
		if err != nil {
			return err
		}
		if err := update(&task); err != nil {
			return err
		}
		if err := s.tasks.Update(txCtx, task); err != nil {
			return err
		}
		mutated = task
		return nil
	})
	if err != nil {
		return domaintask.Task{}, err
	}
	return mutated, nil
}

func (s *Service) requireRunDependencies() error {
	switch {
	case s.tasks == nil:
		return ErrTaskRepositoryRequired
	case s.txRunner == nil:
		return ErrTxRunnerRequired
	case s.translator == nil:
		return ErrTranslatorRequired
	case s.generator == nil:
		return ErrGeneratorRequired
	case s.imageStore == nil:
		return ErrImageStoreRequired
	case s.notifier == nil:
		return ErrNotifierRequired
	default:
		return nil
	}
}

func parseExecutionContext(context domaintask.Context) (executionContext, error) {
	var payload struct {
		Shape   string `json:"shape"`
		Artists string `json:"artists"`
	}
	if err := json.Unmarshal([]byte(context.Raw()), &payload); err != nil {
		return executionContext{}, fmt.Errorf("decode task context: %w", err)
	}

	shape := domaindraw.Shape(strings.TrimSpace(payload.Shape))
	if !shape.Valid() {
		return executionContext{}, fmt.Errorf("task context shape is required")
	}
	return executionContext{
		Shape:   shape,
		Artists: strings.TrimSpace(payload.Artists),
	}, nil
}

func (s *Service) latestIfStopped(ctx context.Context, taskID string) (domaintask.Task, bool, error) {
	task, err := s.Get(ctx, taskID)
	if err != nil {
		return domaintask.Task{}, false, err
	}
	return task, task.Status == domaintask.StatusStopped, nil
}

func (s *Service) failTask(ctx context.Context, task domaintask.Task, code string, stage string, cause error) error {
	taskError, err := domaintask.NewError(code, stage, cause.Error())
	if err != nil {
		return err
	}

	failedTask, err := s.Fail(ctx, FailCommand{
		TaskID: task.ID,
		Error:  taskError,
	})
	if err != nil {
		return err
	}
	s.upsertProgressBestEffort(ctx, &failedTask, failedText(taskError.Message), false)
	s.logger.Error(
		"runner task failed",
		append(runnerTaskAttrs(failedTask),
			"error_code", taskError.Code,
			"error_stage", taskError.Stage,
			"error_message", taskError.Message,
		)...,
	)
	return nil
}

func (s *Service) completeResult(ctx context.Context, taskID string, imagePath string, resultMessageID string) (domaintask.Task, error) {
	return s.updateTask(ctx, taskID, func(task *domaintask.Task) error {
		if err := task.MarkCompleted(imagePath, s.now()); err != nil {
			return err
		}
		task.SetResultMessageID(resultMessageID)
		return nil
	})
}

func (s *Service) upsertProgressBestEffort(ctx context.Context, task *domaintask.Task, text string, enableStop bool) {
	if task == nil {
		return
	}
	options := MessageOptions{}
	if enableStop {
		options.TaskID = task.ID
		options.Variant = MessageVariantProgress
	}
	if strings.TrimSpace(task.ProgressMessageID) != "" {
		if err := s.notifier.EditText(ctx, task.UserID, task.ProgressMessageID, text, options); err != nil {
			s.logger.Warn("runner progress update failed", "task_id", task.ID, "progress_message_id", task.ProgressMessageID, "error", err)
			return
		}
		s.logger.Info("runner progress updated", "task_id", task.ID, "progress_message_id", task.ProgressMessageID, "text", text, "enable_stop", enableStop)
		return
	}

	messageID, err := s.notifier.SendText(ctx, task.UserID, text, options)
	if err != nil {
		s.logger.Warn("runner progress create failed", "task_id", task.ID, "error", err)
		return
	}
	task.SetProgressMessageID(messageID)
	updated, err := s.updateTask(ctx, task.ID, func(current *domaintask.Task) error {
		current.SetProgressMessageID(messageID)
		return nil
	})
	if err == nil {
		*task = updated
		s.logger.Info("runner progress created", "task_id", task.ID, "progress_message_id", messageID, "text", text, "enable_stop", enableStop)
		return
	}
	s.logger.Warn("runner progress persist failed", "task_id", task.ID, "progress_message_id", messageID, "error", err)
}

func (s *Service) deleteProgressBestEffort(ctx context.Context, task domaintask.Task) {
	if strings.TrimSpace(task.ProgressMessageID) == "" {
		return
	}
	if err := s.notifier.DeleteMessage(ctx, task.UserID, task.ProgressMessageID); err != nil {
		s.logger.Warn("runner progress delete failed", "task_id", task.ID, "progress_message_id", task.ProgressMessageID, "error", err)
		return
	}
	s.logger.Info("runner progress deleted", "task_id", task.ID, "progress_message_id", task.ProgressMessageID)
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

func runnerTaskAttrs(task domaintask.Task) []any {
	return []any{
		"task_id", task.ID,
		"user_id", task.UserID,
		"session_id", task.SessionID,
		"source_task_id", task.SourceTaskID,
		"status", task.Status,
		"request", task.Request,
		"prompt", task.Prompt,
		"image", task.Image,
		"task_context", task.Context.Raw(),
		"progress_message_id", task.ProgressMessageID,
		"result_message_id", task.ResultMessageID,
	}
}
