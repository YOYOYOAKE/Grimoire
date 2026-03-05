package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func extractAssistantContent(respBody []byte) (string, error) {
	respBody = bytes.TrimSpace(respBody)
	if len(respBody) == 0 {
		return "", fmt.Errorf("空响应")
	}

	if content, ok := parseOpenAICompletionPayload(respBody); ok {
		return content, nil
	}

	if _, err := parsePromptJSON(string(respBody)); err == nil {
		return string(respBody), nil
	}

	if bytes.Contains(respBody, []byte("data:")) {
		content, err := parseSSEContent(respBody)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(content) != "" {
			return content, nil
		}
	}

	return "", fmt.Errorf("不支持的响应格式")
}

func parseSSEContent(respBody []byte) (string, error) {
	lines := bytes.Split(respBody, []byte("\n"))
	var builder strings.Builder
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		if content, ok := parseOpenAICompletionPayload(payload); ok {
			builder.WriteString(content)
			continue
		}

		if _, err := parsePromptJSON(string(payload)); err == nil {
			builder.Write(payload)
			continue
		}
	}

	if builder.Len() == 0 {
		return "", fmt.Errorf("SSE 数据中未找到可解析内容")
	}
	return builder.String(), nil
}

func parseOpenAICompletionPayload(payload []byte) (string, bool) {
	var out struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Delta struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
			Text json.RawMessage `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		return "", false
	}
	if len(out.Choices) == 0 {
		return "", false
	}

	if content, ok := decodeContentField(out.Choices[0].Message.Content); ok {
		return content, true
	}
	if content, ok := decodeContentField(out.Choices[0].Delta.Content); ok {
		return content, true
	}
	if content, ok := decodeContentField(out.Choices[0].Text); ok {
		return content, true
	}
	return "", false
}

func decodeContentField(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), true
	}

	var asArray []map[string]any
	if err := json.Unmarshal(raw, &asArray); err == nil {
		var parts []string
		for _, item := range asArray {
			if t, ok := item["text"].(string); ok && strings.TrimSpace(t) != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ""), true
		}
	}

	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if t, ok := asObject["text"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t), true
		}
	}

	return "", false
}
