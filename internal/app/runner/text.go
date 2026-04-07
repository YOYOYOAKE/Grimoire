package runner

import "strings"

func queuedText() string {
	return "已入队"
}

func translatingText() string {
	return "正在翻译提示词"
}

func drawingText() string {
	return "正在绘图"
}

func stoppedText() string {
	return "已停止任务"
}

func failedText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "任务失败"
	}
	return "任务失败\n原因: " + reason
}
