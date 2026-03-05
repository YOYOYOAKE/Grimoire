package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/types"
)

func (o *Orchestrator) persistStatus(ctx context.Context, task types.DrawTask, statusMessageID *int64, status string, stage string, errMsg string, notifyText string) bool {
	if err := o.taskStore.UpdateTaskStatus(ctx, task.TaskID, status, stage, errMsg); err != nil {
		o.logger.Error("persist task status failed",
			"task_id", task.TaskID,
			"status", status,
			"stage", stage,
			"error", err,
		)
		o.upsertStatus(ctx, task.ChatID, statusMessageID, fmt.Sprintf("任务 %s\n状态: failed\n原因: 持久化失败\n错误: %v", task.TaskID, err))
		return false
	}
	if strings.TrimSpace(notifyText) != "" {
		o.upsertStatus(ctx, task.ChatID, statusMessageID, notifyText)
	}
	return true
}

func (o *Orchestrator) failTask(ctx context.Context, task types.DrawTask, statusMessageID *int64, jobID string, reason string, cause error) {
	causeText := "未知错误"
	if cause != nil {
		causeText = cause.Error()
	}
	if err := o.taskStore.UpdateTaskStatus(ctx, task.TaskID, types.StatusFailed, "failed", causeText); err != nil {
		o.logger.Error("persist failed status failed", "task_id", task.TaskID, "error", err)
	}

	text := fmt.Sprintf("任务 %s\n状态: failed\n", task.TaskID)
	if strings.TrimSpace(jobID) != "" {
		text += fmt.Sprintf("Job ID: %s\n", jobID)
	}
	text += fmt.Sprintf("原因: %s\n错误: %s", reason, causeText)
	o.upsertStatus(ctx, task.ChatID, statusMessageID, text)
}

func (o *Orchestrator) appendGalleryIndex(ctx context.Context, task types.DrawTask, messageID int64, jobID string, filePath string, caption string) {
	if err := o.taskStore.AppendGalleryItem(ctx, task.ChatID, messageID, task.TaskID, jobID, filePath, caption, time.Now()); err != nil {
		o.logger.Warn("append gallery item failed",
			"task_id", task.TaskID,
			"job_id", jobID,
			"chat_id", task.ChatID,
			"message_id", messageID,
			"error", err,
		)
	}
}

func (o *Orchestrator) markCancelled(task types.DrawTask, statusMessageID *int64, reason string) {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		return
	}

	_ = o.taskStore.UpdateTaskStatus(context.Background(), taskID, "cancelled", "failed", reason)
	text := fmt.Sprintf("任务 %s\n状态: cancelled\n原因: %s", taskID, reason)
	o.upsertStatus(context.Background(), task.ChatID, statusMessageID, text)
}

func (o *Orchestrator) upsertStatus(ctx context.Context, chatID int64, messageID *int64, text string) {
	if messageID != nil && *messageID > 0 {
		if err := o.notifier.EditText(ctx, chatID, *messageID, text); err == nil {
			return
		} else {
			o.logger.Warn("edit task status failed", "chat_id", chatID, "message_id", *messageID, "error", err)
		}
	}

	msgID, err := o.notifier.NotifyText(ctx, chatID, text)
	if err != nil {
		o.logger.Warn("send task status failed", "chat_id", chatID, "error", err)
		return
	}
	if messageID != nil {
		*messageID = msgID
	}
}
