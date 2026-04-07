package draw

import "testing"

func TestTranslationValidateRejectsEmptyPrompt(t *testing.T) {
	if err := (Translation{}).Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateRequestValidateRejectsInvalidShape(t *testing.T) {
	err := (GenerateRequest{
		Prompt: "masterpiece",
		Shape:  Shape("invalid"),
	}).Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateRequestValidateRejectsInvalidCharacterPrompt(t *testing.T) {
	err := (GenerateRequest{
		Prompt: "masterpiece",
		Shape:  ShapeSquare,
		Characters: []CharacterPrompt{
			{},
		},
	}).Validate()
	if err == nil {
		t.Fatal("expected error")
	}
}
