package session

import "strings"

const emptySummaryContent = "{}"

type Summary struct {
	content string
}

func NewSummary(content string) Summary {
	content = strings.TrimSpace(content)
	if content == "" {
		content = emptySummaryContent
	}
	return Summary{content: content}
}

func EmptySummary() Summary {
	return Summary{content: emptySummaryContent}
}

func (s Summary) Content() string {
	if strings.TrimSpace(s.content) == "" {
		return emptySummaryContent
	}
	return s.content
}

func (s Summary) IsEmpty() bool {
	return s.Content() == emptySummaryContent
}
