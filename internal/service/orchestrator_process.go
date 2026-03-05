package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"grimoire/internal/types"
)

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
	processingSince := time.Time{}
	processingWarned := false

	pollEvery := o.pollIntervalOverride
	if pollEvery <= 0 {
		pollEvery = time.Duration(o.cfg.Snapshot().NAI.PollIntervalSec) * time.Second
	}
	if pollEvery <= 0 {
		pollEvery = 5 * time.Second
	}
	warnAfter := o.processingWarningAfter
	if warnAfter <= 0 {
		warnAfter = 3 * time.Minute
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
			if currentStatus == types.StatusProcessing {
				if processingSince.IsZero() {
					processingSince = time.Now()
				}
				if !processingWarned && time.Since(processingSince) >= warnAfter {
					processingWarned = true
					o.logger.Warn("nai processing duration exceeded warning threshold",
						"task_id", task.TaskID,
						"job_id", jobID,
						"elapsed_ms", time.Since(processingSince).Milliseconds(),
						"threshold_ms", warnAfter.Milliseconds(),
					)
					o.upsertStatus(taskCtx, task.ChatID, &statusMessageID,
						fmt.Sprintf("任务 %s\n状态: processing\nJob ID: %s\n提示: 任务可能失败，系统继续轮询。", task.TaskID, jobID))
				}
			} else {
				processingSince = time.Time{}
			}

		default:
			o.logger.Info("unknown status", "task_id", task.TaskID, "job_id", jobID, "status", result.Status)
		}

		select {
		case <-taskCtx.Done():
			o.logger.Warn("task context cancelled during poll interval wait", "task_id", task.TaskID, "job_id", jobID)
			o.markCancelled(task, &statusMessageID, "已取消（轮询等待）")
			return
		case <-time.After(pollEvery):
		}
	}
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
