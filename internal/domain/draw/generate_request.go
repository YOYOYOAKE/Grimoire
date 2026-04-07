package draw

import (
	"fmt"
	"strings"
)

type GenerateRequest struct {
	Prompt         string
	NegativePrompt string
	Characters     []CharacterPrompt
	Shape          Shape
}

func (r GenerateRequest) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("generate request prompt is required")
	}
	if !r.Shape.Valid() {
		return fmt.Errorf("invalid generate request shape %q", r.Shape)
	}
	for _, character := range r.Characters {
		if err := character.Validate(); err != nil {
			return err
		}
	}
	return nil
}
