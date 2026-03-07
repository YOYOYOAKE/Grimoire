package openai

import "testing"

func TestParseTranslation(t *testing.T) {
	translation, err := parseTranslation(`{"positivePrompt":"moonlit girl","negativePrompt":"blurry"}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.PositivePrompt != "moonlit girl" {
		t.Fatalf("unexpected positive prompt: %q", translation.PositivePrompt)
	}
}
