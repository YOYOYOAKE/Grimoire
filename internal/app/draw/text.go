package draw

import (
	"fmt"

	domaindraw "grimoire/internal/domain/draw"
)

func queuedText(task domaindraw.Task, queuePos int) string {
	return fmt.Sprintf("任务 %s\n状态: queued\n队列位置: %d", task.ID, queuePos)
}

func translatingText(task domaindraw.Task) string {
	return fmt.Sprintf("任务 %s\n状态: translating\n阶段: 提示词翻译", task.ID)
}

func submittingText(task domaindraw.Task) string {
	return fmt.Sprintf("任务 %s\n状态: submitting\n阶段: 提交绘图任务", task.ID)
}

func pollingText(task domaindraw.Task, providerStatus string, queuePos int) string {
	text := fmt.Sprintf("任务 %s\n状态: polling\nJob ID: %s\n生成状态: %s", task.ID, task.ProviderJobID, providerStatus)
	if queuePos > 0 {
		text += fmt.Sprintf("\n队列位置: %d", queuePos)
	}
	return text
}

func completedText(task domaindraw.Task) string {
	return fmt.Sprintf("任务 %s\n状态: completed\nJob ID: %s\n结果: 图片已发送", task.ID, task.ProviderJobID)
}

func completedCaption(task domaindraw.Task) string {
	return fmt.Sprintf("任务 %s 完成\nJob ID: %s", task.ID, task.ProviderJobID)
}

func failedText(task domaindraw.Task) string {
	return fmt.Sprintf("任务 %s\n状态: failed\n原因: %s", task.ID, task.ErrorText)
}
