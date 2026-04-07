package draw

import (
	"fmt"
	"strings"
)

type CharacterPrompt struct {
	Prompt         string
	NegativePrompt string
	Position       string
}

type Translation struct {
	Prompt         string
	NegativePrompt string
	Characters     []CharacterPrompt
}

func (c CharacterPrompt) Validate() error {
	if strings.TrimSpace(c.Prompt) == "" {
		return fmt.Errorf("character prompt is required")
	}
	return nil
}

func (t Translation) Validate() error {
	if strings.TrimSpace(t.Prompt) == "" {
		return fmt.Errorf("translation prompt is required")
	}
	for _, character := range t.Characters {
		if err := character.Validate(); err != nil {
			return err
		}
	}
	return nil
}
