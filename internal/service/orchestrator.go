package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"grimoire/internal/config"
	"grimoire/internal/store"
	"grimoire/internal/types"
)

type Orchestrator struct {
	translator types.Translator
	generator  types.ImageGenerator
	notifier   types.Notifier
	cfg        *config.Manager
	taskStore  store.TaskStore
	logger     *slog.Logger

	mu             sync.Mutex
	activeCancels  map[string]context.CancelFunc
	pendingCancels map[string]struct{}
}

func NewOrchestrator(
	translator types.Translator,
	generator types.ImageGenerator,
	notifier types.Notifier,
	cfg *config.Manager,
	taskStore store.TaskStore,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		translator:     translator,
		generator:      generator,
		notifier:       notifier,
		cfg:            cfg,
		taskStore:      taskStore,
		logger:         logger,
		activeCancels:  make(map[string]context.CancelFunc),
		pendingCancels: make(map[string]struct{}),
	}
}

func (o *Orchestrator) ProcessTask(ctx context.Context, task types.DrawTask) {
	taskCtx, cancel := context.WithCancel(ctx)
	o.registerCancel(task.TaskID, cancel)
	defer o.unregisterCancel(task.TaskID)

	task.StartedAt = time.Now()
	o.logger.Info("task processing started",
		"task_id", task.TaskID,
		"chat_id", task.ChatID,
		"user_id", task.UserID,
		"shape", task.Shape,
		"prompt", task.Prompt,
	)
	statusMessageID := task.StatusMessageID
	if o.consumePendingCancel(task.TaskID) {
		o.logger.Info("task pre-cancelled before execution", "task_id", task.TaskID)
		o.markCancelled(task, &statusMessageID, "已取消（开始执行前）")
		return
	}
	var jobID string
	if strings.TrimSpace(task.ResumeJobID) != "" {
		jobID = strings.TrimSpace(task.ResumeJobID)
		o.logger.Info("task resume polling",
			"task_id", task.TaskID,
			"job_id", jobID,
		)
		if !o.persistStatus(taskCtx, task, &statusMessageID, types.StatusProcessing, "polling", "",
			fmt.Sprintf("任务 %s\n状态: processing\n阶段: 恢复轮询\nJob ID: %s", task.TaskID, jobID)) {
			return
		}
	} else {
		if !o.persistStatus(taskCtx, task, &statusMessageID, types.StatusProcessing, "translating", "",
			fmt.Sprintf("任务 %s\n状态: processing\n阶段: 提示词翻译", task.TaskID)) {
			return
		}

		translateStart := time.Now()
		o.logger.Info("llm translate started", "task_id", task.TaskID, "shape", task.Shape)
		translation, err := o.translator.Translate(taskCtx, task.Prompt, task.Shape)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(taskCtx.Err(), context.Canceled) {
				o.logger.Info("llm translate cancelled", "task_id", task.TaskID)
				o.markCancelled(task, &statusMessageID, "已取消（提示词翻译）")
				return
			}
			o.logger.Error("llm translate failed", "task_id", task.TaskID, "error", err)
			o.failTask(taskCtx, task, &statusMessageID, "", "LLM 翻译失败", err)
			return
		}
		o.logger.Info("llm translate completed",
			"task_id", task.TaskID,
			"duration_ms", time.Since(translateStart).Milliseconds(),
			"positive_prompt", translation.PositivePrompt,
			"negative_prompt", translation.NegativePrompt,
			"character_count", len(translation.Characters),
		)
		artist := o.cfg.Snapshot().Generation.Artist
		finalPositive := mergeArtistPrompt(artist, translation.PositivePrompt)
		o.logger.Info("prompt composed for generation",
			"task_id", task.TaskID,
			"artist", artist,
			"positive_prompt", finalPositive,
			"character_count", len(translation.Characters),
		)

		if !o.persistStatus(taskCtx, task, &statusMessageID, types.StatusProcessing, "submitting", "",
			fmt.Sprintf("任务 %s\n状态: processing\n阶段: 提交绘图任务", task.TaskID)) {
			return
		}

		submitStart := time.Now()
		o.logger.Info("nai submit started", "task_id", task.TaskID, "shape", task.Shape)
		jobID, err = o.generator.Submit(taskCtx, types.GenerateRequest{
			PositivePrompt: finalPositive,
			NegativePrompt: translation.NegativePrompt,
			Shape:          task.Shape,
			Seed:           task.Seed,
			Characters:     translation.Characters,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(taskCtx.Err(), context.Canceled) {
				o.logger.Info("nai submit cancelled", "task_id", task.TaskID)
				o.markCancelled(task, &statusMessageID, "已取消（任务提交）")
				return
			}
			o.logger.Error("nai submit failed", "task_id", task.TaskID, "error", err)
			o.failTask(taskCtx, task, &statusMessageID, "", "提交绘图任务失败", err)
			return
		}
		o.logger.Info("nai submit completed",
			"task_id", task.TaskID,
			"job_id", jobID,
			"duration_ms", time.Since(submitStart).Milliseconds(),
		)
		if err := o.taskStore.SetTaskJobID(taskCtx, task.TaskID, jobID); err != nil {
			o.logger.Error("persist task job id failed", "task_id", task.TaskID, "job_id", jobID, "error", err)
			o.failTask(taskCtx, task, &statusMessageID, jobID, "持久化任务失败", err)
			return
		}

		if !o.persistStatus(taskCtx, task, &statusMessageID, types.StatusQueued, "polling", "",
			fmt.Sprintf("任务 %s\n状态: queued\nJob ID: %s\n队列位置: 等待更新", task.TaskID, jobID)) {
			return
		}
	}

	lastQueuePos := -1
	lastStatus := ""

	pollEvery := time.Duration(o.cfg.Snapshot().NAI.PollIntervalSec) * time.Second
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}

	for {
		select {
		case <-taskCtx.Done():
			o.logger.Warn("task context cancelled", "task_id", task.TaskID, "job_id", jobID)
			o.markCancelled(task, &statusMessageID, "已取消（生成中）")
			return
		default:
		}

		result, err := o.generator.Poll(taskCtx, jobID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(taskCtx.Err(), context.Canceled) {
				o.logger.Info("nai poll cancelled", "task_id", task.TaskID, "job_id", jobID)
				o.markCancelled(task, &statusMessageID, "已取消（轮询）")
				return
			}
			o.logger.Error("nai poll failed", "task_id", task.TaskID, "job_id", jobID, "error", err)
			o.failTask(taskCtx, task, &statusMessageID, jobID, "轮询失败", err)
			return
		}

		switch strings.ToLower(result.Status) {
		case types.StatusCompleted:
			o.logger.Info("nai generation completed", "task_id", task.TaskID, "job_id", jobID)
			o.upsertStatus(taskCtx, task.ChatID, &statusMessageID, fmt.Sprintf("任务 %s\n状态: completed\nJob ID: %s\n阶段: 正在保存并发送图片", task.TaskID, jobID))
			saveStart := time.Now()
			filePath, err := o.saveImage(result.ImageBase64, task.TaskID, jobID)
			if err != nil {
				o.logger.Error("save image failed", "task_id", task.TaskID, "job_id", jobID, "error", err)
				o.failTask(taskCtx, task, &statusMessageID, jobID, "保存图片失败", err)
				return
			}
			if err := o.taskStore.SaveTaskResult(taskCtx, task.TaskID, jobID, filePath, time.Now()); err != nil {
				o.logger.Error("persist task result failed", "task_id", task.TaskID, "job_id", jobID, "error", err)
				o.failTask(taskCtx, task, &statusMessageID, jobID, "持久化结果失败", err)
				return
			}
			o.logger.Info("image saved",
				"task_id", task.TaskID,
				"job_id", jobID,
				"file_path", filePath,
				"duration_ms", time.Since(saveStart).Milliseconds(),
			)
			caption := fmt.Sprintf("任务 %s 完成\nJob ID: %s", task.TaskID, jobID)
			resultMessageID := int64(0)
			if statusMessageID > 0 {
				resultMessageID = statusMessageID
			}
			o.appendGalleryIndex(taskCtx, task, resultMessageID, jobID, filePath, caption)
			if statusMessageID > 0 {
				sendStart := time.Now()
				if err := o.notifier.EditPhoto(taskCtx, task.ChatID, statusMessageID, filePath, caption); err == nil {
					o.logger.Info("photo edited into status message",
						"task_id", task.TaskID,
						"job_id", jobID,
						"message_id", statusMessageID,
						"duration_ms", time.Since(sendStart).Milliseconds(),
					)
					return
				} else {
					o.logger.Warn("edit photo failed, fallback to send new photo",
						"task_id", task.TaskID,
						"job_id", jobID,
						"message_id", statusMessageID,
						"error", err,
					)
				}
			}

			sendStart := time.Now()
			if err := o.notifier.NotifyPhoto(taskCtx, task.ChatID, filePath, caption); err != nil {
				o.logger.Error("send photo failed", "task_id", task.TaskID, "job_id", jobID, "error", err)
				o.upsertStatus(taskCtx, task.ChatID, &statusMessageID, fmt.Sprintf("任务 %s\n状态: completed_with_warning\nJob ID: %s\n警告: 发图失败\n错误: %v\n图片路径: %s", task.TaskID, jobID, err, filePath))
				return
			}
			o.logger.Info("photo sent as new message",
				"task_id", task.TaskID,
				"job_id", jobID,
				"duration_ms", time.Since(sendStart).Milliseconds(),
			)
			o.upsertStatus(taskCtx, task.ChatID, &statusMessageID, fmt.Sprintf("任务 %s\n状态: completed\nJob ID: %s\n结果: 图片已发送（编辑状态消息失败，已另发）", task.TaskID, jobID))
			return

		case types.StatusFailed:
			errMsg := result.Error
			if errMsg == "" {
				errMsg = "未知错误"
			}
			o.logger.Error("nai generation failed", "task_id", task.TaskID, "job_id", jobID, "error", errMsg)
			o.failTask(taskCtx, task, &statusMessageID, jobID, "绘图失败", fmt.Errorf("%s", errMsg))
			return

		case types.StatusQueued, types.StatusProcessing:
			currentStatus := strings.ToLower(result.Status)
			if result.QueuePosition != lastQueuePos || currentStatus != lastStatus {
				lastQueuePos = result.QueuePosition
				lastStatus = currentStatus
				o.logger.Info("nai status updated",
					"task_id", task.TaskID,
					"job_id", jobID,
					"status", currentStatus,
					"queue_position", result.QueuePosition,
				)
				if !o.persistStatus(taskCtx, task, &statusMessageID, currentStatus, "polling", "",
					fmt.Sprintf("任务 %s\n状态: %s\nJob ID: %s\n队列位置: %d", task.TaskID, currentStatus, jobID, result.QueuePosition)) {
					return
				}
			}

		default:
			o.logger.Info("unknown status", "task_id", task.TaskID, "job_id", jobID, "status", result.Status)
		}

		select {
		case <-taskCtx.Done():
			return
		case <-time.After(pollEvery):
		}
	}
}

func (o *Orchestrator) CancelTask(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if cancel, ok := o.activeCancels[taskID]; ok {
		cancel()
		return true
	}
	o.pendingCancels[taskID] = struct{}{}
	return true
}

func (o *Orchestrator) registerCancel(taskID string, cancel context.CancelFunc) {
	o.mu.Lock()
	o.activeCancels[taskID] = cancel
	o.mu.Unlock()
}

func (o *Orchestrator) unregisterCancel(taskID string) {
	o.mu.Lock()
	delete(o.activeCancels, taskID)
	delete(o.pendingCancels, taskID)
	o.mu.Unlock()
}

func (o *Orchestrator) consumePendingCancel(taskID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.pendingCancels[taskID]
	if ok {
		delete(o.pendingCancels, taskID)
	}
	return ok
}

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

func (o *Orchestrator) saveImage(imageB64 string, taskID string, jobID string) (string, error) {
	if strings.TrimSpace(imageB64) == "" {
		return "", fmt.Errorf("空图片数据")
	}
	decoded, err := base64.StdEncoding.DecodeString(imageB64)
	if err != nil {
		return "", fmt.Errorf("base64 解码失败: %w", err)
	}

	cfg := o.cfg.Snapshot()
	day := time.Now().Format("20060102")
	dir := filepath.Join(cfg.Runtime.SaveDir, day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	name := fmt.Sprintf("%s_%s_%s.png", time.Now().Format("150405"), sanitize(taskID), sanitize(jobID))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, decoded, 0o644); err != nil {
		return "", fmt.Errorf("写入图片失败: %w", err)
	}
	return path, nil
}

func sanitize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")
	v = strings.ReplaceAll(v, " ", "_")
	if v == "" {
		return "unknown"
	}
	return v
}

func mergeArtistPrompt(artist string, positive string) string {
	artist = strings.TrimSpace(artist)
	positive = strings.TrimSpace(positive)
	if artist == "" {
		return positive
	}
	if positive == "" {
		return artist
	}
	return artist + ", " + positive
}
