package task

import (
	"encoding/json"
	"fmt"
	"strings"

	domaindraw "grimoire/internal/domain/draw"
)

type PromptBundle struct {
	Prompt         string
	NegativePrompt string
	Characters     []domaindraw.CharacterPrompt
}

type ExecutionContext struct {
	Version      int
	Shape        domaindraw.Shape
	Artists      string
	PromptBundle *PromptBundle
}

type contextPayload struct {
	Version      int               `json:"version,omitempty"`
	Shape        string            `json:"shape,omitempty"`
	Artists      string            `json:"artists,omitempty"`
	PromptBundle *promptBundleJSON `json:"prompt_bundle,omitempty"`
}

type promptBundleJSON struct {
	Prompt         string                `json:"prompt"`
	NegativePrompt string                `json:"negative_prompt"`
	Characters     []characterPromptJSON `json:"characters"`
}

type characterPromptJSON struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt"`
	Position       string `json:"position"`
}

func NewPromptBundle(prompt string, negativePrompt string, characters []domaindraw.CharacterPrompt) (PromptBundle, error) {
	bundle := PromptBundle{
		Prompt:         strings.TrimSpace(prompt),
		NegativePrompt: strings.TrimSpace(negativePrompt),
		Characters:     normalizeCharacterPrompts(characters),
	}
	if err := bundle.Validate(); err != nil {
		return PromptBundle{}, err
	}
	return bundle, nil
}

func (b PromptBundle) Validate() error {
	if strings.TrimSpace(b.Prompt) == "" {
		return fmt.Errorf("prompt bundle prompt is required")
	}
	for _, character := range b.Characters {
		if err := character.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c Context) Execution() (ExecutionContext, error) {
	payload, err := c.parsePayload()
	if err != nil {
		return ExecutionContext{}, err
	}

	shape := domaindraw.Shape(strings.TrimSpace(payload.Shape))
	if !shape.Valid() {
		return ExecutionContext{}, fmt.Errorf("task context shape is required")
	}

	var bundle *PromptBundle
	if payload.PromptBundle != nil {
		parsed, err := payload.PromptBundle.decode()
		if err != nil {
			return ExecutionContext{}, err
		}
		bundle = &parsed
	}

	return ExecutionContext{
		Version:      payload.Version,
		Shape:        shape,
		Artists:      strings.TrimSpace(payload.Artists),
		PromptBundle: bundle,
	}, nil
}

func (c Context) PromptBundle() (PromptBundle, bool, error) {
	payload, err := c.parsePayload()
	if err != nil {
		return PromptBundle{}, false, err
	}
	if payload.PromptBundle == nil {
		return PromptBundle{}, false, nil
	}
	bundle, err := payload.PromptBundle.decode()
	if err != nil {
		return PromptBundle{}, false, err
	}
	return bundle, true, nil
}

func (c Context) WithPromptBundle(bundle PromptBundle) (Context, error) {
	if err := bundle.Validate(); err != nil {
		return Context{}, err
	}
	payload, err := c.parsePayload()
	if err != nil {
		return Context{}, err
	}
	payload.Version = 2
	encoded, err := encodePromptBundle(bundle)
	if err != nil {
		return Context{}, err
	}
	payload.PromptBundle = &encoded
	return newContextFromPayload(payload)
}

func (c Context) WithoutPromptBundle() (Context, error) {
	payload, err := c.parsePayload()
	if err != nil {
		return Context{}, err
	}
	payload.Version = 2
	payload.PromptBundle = nil
	return newContextFromPayload(payload)
}

func (c Context) parsePayload() (contextPayload, error) {
	var payload contextPayload
	if err := json.Unmarshal([]byte(c.raw), &payload); err != nil {
		return contextPayload{}, fmt.Errorf("decode task context: %w", err)
	}
	return payload, nil
}

func newContextFromPayload(payload contextPayload) (Context, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Context{}, fmt.Errorf("marshal task context: %w", err)
	}
	return NewContext(string(raw))
}

func encodePromptBundle(bundle PromptBundle) (promptBundleJSON, error) {
	if err := bundle.Validate(); err != nil {
		return promptBundleJSON{}, err
	}
	characters := make([]characterPromptJSON, 0, len(bundle.Characters))
	for _, character := range bundle.Characters {
		characters = append(characters, characterPromptJSON{
			Prompt:         strings.TrimSpace(character.Prompt),
			NegativePrompt: strings.TrimSpace(character.NegativePrompt),
			Position:       strings.TrimSpace(character.Position),
		})
	}
	return promptBundleJSON{
		Prompt:         strings.TrimSpace(bundle.Prompt),
		NegativePrompt: strings.TrimSpace(bundle.NegativePrompt),
		Characters:     characters,
	}, nil
}

func (p promptBundleJSON) decode() (PromptBundle, error) {
	characters := make([]domaindraw.CharacterPrompt, 0, len(p.Characters))
	for _, character := range p.Characters {
		characters = append(characters, domaindraw.CharacterPrompt{
			Prompt:         strings.TrimSpace(character.Prompt),
			NegativePrompt: strings.TrimSpace(character.NegativePrompt),
			Position:       strings.TrimSpace(character.Position),
		})
	}
	return NewPromptBundle(p.Prompt, p.NegativePrompt, characters)
}

func normalizeCharacterPrompts(characters []domaindraw.CharacterPrompt) []domaindraw.CharacterPrompt {
	if len(characters) == 0 {
		return nil
	}
	normalized := make([]domaindraw.CharacterPrompt, 0, len(characters))
	for _, character := range characters {
		normalized = append(normalized, domaindraw.CharacterPrompt{
			Prompt:         strings.TrimSpace(character.Prompt),
			NegativePrompt: strings.TrimSpace(character.NegativePrompt),
			Position:       strings.TrimSpace(character.Position),
		})
	}
	return normalized
}
