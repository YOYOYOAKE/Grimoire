package llm

import (
	"testing"

	"grimoire/internal/config"
)

func TestBuildLLMChatCompletionsURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider string
		baseURL  string
		want     string
	}{
		{
			name:     "openai custom appends v1 when missing",
			provider: config.ProviderOpenAICustom,
			baseURL:  "https://api.example.com",
			want:     "https://api.example.com/v1/chat/completions",
		},
		{
			name:     "openai custom keeps existing v1",
			provider: config.ProviderOpenAICustom,
			baseURL:  "https://api.example.com/v1",
			want:     "https://api.example.com/v1/chat/completions",
		},
		{
			name:     "openai custom appends v1 under custom path",
			provider: config.ProviderOpenAICustom,
			baseURL:  "https://api.example.com/proxy/",
			want:     "https://api.example.com/proxy/v1/chat/completions",
		},
		{
			name:     "openrouter keeps path",
			provider: config.ProviderOpenRouter,
			baseURL:  "https://openrouter.ai/api/v1/",
			want:     "https://openrouter.ai/api/v1/chat/completions",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Config{
				LLM: config.LLMConfig{
					Provider: tc.provider,
					BaseURL:  tc.baseURL,
				},
			}
			got := buildLLMChatCompletionsURL(cfg)
			if got != tc.want {
				t.Fatalf("unexpected url: got=%q want=%q", got, tc.want)
			}
		})
	}
}
