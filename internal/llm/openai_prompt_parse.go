package llm

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"grimoire/internal/types"
)

func parsePromptJSON(raw string) (types.TranslationResult, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var parsed struct {
		PositivePrompt   string `json:"positivePrompt"`
		NegativePrompt   string `json:"negativePrompt"`
		CharacterPrompts []struct {
			CharPositivePrompt  string `json:"charPositivePrompt"`
			CharUnconcentPrompt string `json:"charUnconcentPrompt"`
			Centers             struct {
				X any `json:"x"`
				Y any `json:"y"`
			} `json:"centers"`
		} `json:"characterPrompts"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return types.TranslationResult{}, err
	}

	result := types.TranslationResult{
		PositivePrompt: strings.TrimSpace(parsed.PositivePrompt),
		NegativePrompt: strings.TrimSpace(parsed.NegativePrompt),
	}
	if result.PositivePrompt == "" {
		return types.TranslationResult{}, fmt.Errorf("missing positivePrompt")
	}
	if result.NegativePrompt == "" {
		return types.TranslationResult{}, fmt.Errorf("missing negativePrompt")
	}

	for idx, cp := range parsed.CharacterPrompts {
		pos := strings.TrimSpace(cp.CharPositivePrompt)
		neg := strings.TrimSpace(cp.CharUnconcentPrompt)
		if pos == "" {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].charPositivePrompt is required", idx)
		}

		row, err := parseGridRow(cp.Centers.X)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.x invalid: %w", idx, err)
		}
		col, err := parseGridCol(cp.Centers.Y)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.y invalid: %w", idx, err)
		}

		centerX, err := mapGridIndexToCoord(row)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.x invalid: %w", idx, err)
		}
		centerY, err := mapGridIndexToCoord(col)
		if err != nil {
			return types.TranslationResult{}, fmt.Errorf("characterPrompts[%d].centers.y invalid: %w", idx, err)
		}

		result.Characters = append(result.Characters, types.CharacterPrompt{
			PositivePrompt: pos,
			NegativePrompt: neg,
			CenterX:        centerX,
			CenterY:        centerY,
		})
	}

	return result, nil
}

func parseGridRow(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("must be integer")
		}
		return int(n), nil
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, fmt.Errorf("empty value")
		}
		parsed, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("must be integer string")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func parseGridCol(v any) (int, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("must be letter A-E")
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 1 {
		return 0, fmt.Errorf("must be single letter A-E")
	}
	r := s[0]
	if r < 'A' || r > 'E' {
		return 0, fmt.Errorf("must be in A-E")
	}
	return int(r-'A') + 1, nil
}

func mapGridIndexToCoord(idx int) (float64, error) {
	switch idx {
	case 1:
		return 0.1, nil
	case 2:
		return 0.3, nil
	case 3:
		return 0.5, nil
	case 4:
		return 0.7, nil
	case 5:
		return 0.9, nil
	default:
		return 0, fmt.Errorf("index out of range 1..5")
	}
}
