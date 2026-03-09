package draw

import (
	"fmt"
	"strings"
)

func queuedText() string {
	return "已入队"
}

func translatingText() string {
	return "正在翻译提示词"
}

func drawingText(queuePos int) string {
	text := "正在绘图"
	if queuePos > 0 {
		text += fmt.Sprintf("\n当前队列位置: %d", queuePos)
	}
	return text
}

func failedText(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "任务失败"
	}
	return "任务失败\n原因: " + reason
}
