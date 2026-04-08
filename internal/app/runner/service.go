package runner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
}

func NewService(
	tasks TaskRepository,
	txRunner TxRunner,
	translator PromptTranslator,
	generator ImageGenerator,
	imageStore ImageStore,
	notifier Notifier,
	now func() time.Time,
) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		tasks:      tasks,
		txRunner:   txRunner,
		translator: translator,
		generator:  generator,
		imageStore: imageStore,
		notifier:   notifier,
		now:        now,
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
	if err := s.requireRunDependencies(); err != nil {
		return err
	}

	task, err := s.Get(ctx, command.TaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	switch task.Status {
	case domaintask.StatusCompleted, domaintask.StatusFailed:
		return nil
	case domaintask.StatusStopped:
		s.upsertProgressBestEffort(ctx, &task, stoppedText(), false)
		return nil
	}

	execContext, err := parseExecutionContext(task.Context)
	if err != nil {
		return s.failTask(ctx, task.ID, "INVALID_CONTEXT", "preparing", err)
	}

	if task.Status == domaintask.StatusQueued {
		s.upsertProgressBestEffort(ctx, &task, queuedText(), true)
		task, err = s.StartTranslating(ctx, RunCommand{TaskID: task.ID})
		if err != nil {
			return err
		}
		s.upsertProgressBestEffort(ctx, &task, translatingText(), true)
	}

	var translation *domaindraw.Translation
	if task.Status == domaintask.StatusTranslating {
		if strings.TrimSpace(task.Prompt) == "" {
			translated, err := s.translator.Translate(ctx, task.Request, execContext.Shape)
			if err != nil {
				return s.failTask(ctx, task.ID, "PROMPT_TRANSLATE_FAILED", "translating", err)
			}
			if err := translated.Validate(); err != nil {
				return s.failTask(ctx, task.ID, "PROMPT_TRANSLATE_FAILED", "translating", err)
			}
			stopped, stopRequested, err := s.latestIfStopped(ctx, task.ID)
			if err != nil {
				return err
			}
			if stopRequested {
				s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
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
			task, err = s.StartDrawing(ctx, StartDrawingCommand{TaskID: task.ID})
			if err != nil {
				return err
			}
		}
		s.upsertProgressBestEffort(ctx, &task, drawingText(), true)
	}

	if task.Status == domaintask.StatusStopped {
		s.upsertProgressBestEffort(ctx, &task, stoppedText(), false)
		return nil
	}
	if task.Status != domaintask.StatusDrawing {
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

	imageContent, err := s.generator.Generate(ctx, request)
	if err != nil {
		return s.failTask(ctx, task.ID, "IMAGE_GENERATE_FAILED", "drawing", err)
	}
	stopped, stopRequested, err := s.latestIfStopped(ctx, task.ID)
	if err != nil {
		return err
	}
	if stopRequested {
		s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
		return nil
	}

	imagePath, err := s.imageStore.Save(ctx, task.UserID, task.ID, imageContent)
	if err != nil {
		return s.failTask(ctx, task.ID, "IMAGE_STORE_FAILED", "drawing", err)
	}
	stopped, stopRequested, err = s.latestIfStopped(ctx, task.ID)
	if err != nil {
		return err
	}
	if stopRequested {
		s.upsertProgressBestEffort(ctx, &stopped, stoppedText(), false)
		return nil
	}

	resultMessageID, err := s.notifier.SendImage(ctx, task.UserID, imagePath, "", MessageOptions{})
	if err != nil {
		return s.failTask(ctx, task.ID, "SEND_RESULT_FAILED", "notifying", err)
	}

	task, err = s.completeResult(ctx, task.ID, imagePath, resultMessageID)
	if err != nil {
		return err
	}
	s.deleteProgressBestEffort(ctx, task)
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

func (s *Service) failTask(ctx context.Context, taskID string, code string, stage string, cause error) error {
	taskError, err := domaintask.NewError(code, stage, cause.Error())
	if err != nil {
		return err
	}

	task, err := s.Fail(ctx, FailCommand{
		TaskID: taskID,
		Error:  taskError,
	})
	if err != nil {
		return err
	}
	s.upsertProgressBestEffort(ctx, &task, failedText(taskError.Message), false)
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
		_ = s.notifier.EditText(ctx, task.UserID, task.ProgressMessageID, text, options)
		return
	}

	messageID, err := s.notifier.SendText(ctx, task.UserID, text, options)
	if err != nil {
		return
	}
	task.SetProgressMessageID(messageID)
	updated, err := s.updateTask(ctx, task.ID, func(current *domaintask.Task) error {
		current.SetProgressMessageID(messageID)
		return nil
	})
	if err == nil {
		*task = updated
	}
}

func (s *Service) deleteProgressBestEffort(ctx context.Context, task domaintask.Task) {
	if strings.TrimSpace(task.ProgressMessageID) == "" {
		return
	}
	_ = s.notifier.DeleteMessage(ctx, task.UserID, task.ProgressMessageID)
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
