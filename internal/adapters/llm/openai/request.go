package openai

import "strings"

func addReasoningEffort(body map[string]any, reasoningEffort string) {
	reasoningEffort = strings.TrimSpace(reasoningEffort)
	if reasoningEffort == "" {
		return
	}
	body["reasoning_effort"] = reasoningEffort
}
