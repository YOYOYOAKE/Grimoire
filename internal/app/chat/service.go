package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	conversationapp "grimoire/internal/app/conversation"
	sessionapp "grimoire/internal/app/session"
	domainpreferences "grimoire/internal/domain/preferences"
)

type Service struct {
	users         UserRepository
	sessions      SessionService
	conversations ConversationService
	tasks         TaskService
	logger        *slog.Logger
}

func NewService(
	users UserRepository,
	sessions SessionService,
	conversations ConversationService,
	tasks TaskService,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		users:         users,
		sessions:      sessions,
		conversations: conversations,
		tasks:         tasks,
		logger:        logger,
	}
}

func (s *Service) HandleText(ctx context.Context, command HandleTextCommand) (HandleTextResult, error) {
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return HandleTextResult{}, fmt.Errorf("user id is required")
	}
	messageID := strings.TrimSpace(command.MessageID)
	if messageID == "" {
		return HandleTextResult{}, fmt.Errorf("message id is required")
	}
	text := strings.TrimSpace(command.Text)
	if text == "" {
		return HandleTextResult{}, fmt.Errorf("text is required")
	}
	if command.CreatedAt.IsZero() {
		return HandleTextResult{}, fmt.Errorf("created at is required")
	}
	s.logger.Info(
		"chat handle text started",
		"user_id", userID,
		"message_id", messageID,
		"text", text,
		"created_at", command.CreatedAt,
	)

	user, err := s.users.GetByTelegramID(ctx, userID)
	if err != nil {
		return HandleTextResult{}, err
	}
	s.logger.Info(
		"chat user loaded",
		"user_id", userID,
		"preference_shape", user.Preference.Shape,
		"preference_artists", user.Preference.Artists,
		"preference_mode", user.Preference.Mode,
	)

	currentSession, err := s.sessions.GetOrCreate(ctx, sessionapp.GetOrCreateCommand{UserID: userID})
	if err != nil {
		return HandleTextResult{}, err
	}
	s.logger.Info(
		"chat session ready",
		"user_id", userID,
		"session_id", currentSession.ID,
	)

	if user.Preference.Mode == domainpreferences.ModeFast {
		if s.tasks == nil {
			return HandleTextResult{}, fmt.Errorf("task service is required")
		}
		taskContext, err := buildTaskContext(user.Preference)
		if err != nil {
			return HandleTextResult{}, err
		}
		task, err := s.tasks.Create(ctx, taskCreateCommand(
			userID,
			currentSession.ID,
			text,
			taskContext,
		))
		if err != nil {
			return HandleTextResult{}, err
		}
		s.logger.Info(
			"chat fast mode task created",
			"user_id", userID,
			"session_id", currentSession.ID,
			"request", text,
			"task_context", taskContext.Raw(),
			"task_id", task.ID,
		)
		return HandleTextResult{
			SessionID:     currentSession.ID,
			CreatedTaskID: task.ID,
		}, nil
	}

	if _, err := s.sessions.AppendUserMessage(ctx, sessionapp.AppendMessageCommand{
		SessionID: currentSession.ID,
		MessageID: messageID,
		Content:   text,
		CreatedAt: command.CreatedAt,
	}); err != nil {
		return HandleTextResult{}, err
	}
	s.logger.Info(
		"chat user message appended",
		"user_id", userID,
		"session_id", currentSession.ID,
		"message_id", messageID,
		"text", text,
	)

	result, err := s.conversations.Converse(ctx, conversationapp.ConverseCommand{
		SessionID: currentSession.ID,
	})
	if err != nil {
		return HandleTextResult{}, err
	}
	s.logger.Info(
		"chat conversation completed",
		"user_id", userID,
		"session_id", currentSession.ID,
		"reply", result.Reply,
		"create_drawing_task", result.CreateDrawingTask != nil,
		"request", createDrawingTaskRequest(result.CreateDrawingTask),
	)

	if result.CreateDrawingTask != nil {
		if s.tasks == nil {
			return HandleTextResult{}, fmt.Errorf("task service is required")
		}

		taskContext, err := buildTaskContext(user.Preference)
		if err != nil {
			return HandleTextResult{}, err
		}
		task, err := s.tasks.Create(ctx, taskCreateCommand(
			userID,
			currentSession.ID,
			result.CreateDrawingTask.Request,
			taskContext,
		))
		if err != nil {
			return HandleTextResult{}, err
		}
		s.logger.Info(
			"chat task created",
			"user_id", userID,
			"session_id", currentSession.ID,
			"request", result.CreateDrawingTask.Request,
			"task_context", taskContext.Raw(),
			"task_id", task.ID,
		)
		return HandleTextResult{
			SessionID:     currentSession.ID,
			CreatedTaskID: task.ID,
		}, nil
	}
	s.logger.Info(
		"chat reply returned without task creation",
		"user_id", userID,
		"session_id", currentSession.ID,
		"reply", result.Reply,
		"created_task", false,
	)

	return HandleTextResult{
		SessionID: currentSession.ID,
		Reply:     result.Reply,
	}, nil
}

func createDrawingTaskRequest(task *conversationapp.CreateDrawingTask) string {
	if task == nil {
		return ""
	}
	return task.Request
}
