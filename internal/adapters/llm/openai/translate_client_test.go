package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"grimoire/internal/config"
	"grimoire/internal/domain/draw"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

const translatePromptBaseNegativeSequence = "bad_anatomy, bad_hands, malformed_hands, extra_fingers, missing_fingers, fused_fingers, malformed_limbs, extra_limbs, missing_limbs, deformed, distorted, disfigured, bad_eyes, poorly_drawn_eyes, asymmetrical_eyes, bad_proportions, cropped, out_of_frame, blurry, worst_quality, bad_quality, very_displeasing, jpeg_artifacts, watermark, signature, text, logo"

func TestParseTranslation(t *testing.T) {
	translation, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry, lowres" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 0 {
		t.Fatalf("expected no characters, got %#v", translation.Characters)
	}
}

func TestTranslateSystemPromptRequiresJSONOutput(t *testing.T) {
	for _, expected := range []string{
		"Your output must always be a valid `json` object.",
		`"prompt": "shared scene-level English tags"`,
		`"negative_prompt": "shared/global English negative tags"`,
		`"characters": [`,
		"Always infer the subject count from the request and express it explicitly in the global `prompt`",
		"Even for a single clearly identified subject, you must still include an explicit count tag",
		"1.2::masterpiece::, best_quality, highres, extremely_detailed_CG, perfect_lighting, 8k_wallpaper",
		translatePromptBaseNegativeSequence,
		"The fixed base negative tag sequence is a mandatory generation-level quality baseline",
		"Keep the base negative tag sequence in this order, with underscores replacing spaces.",
		"The shared `negative_prompt` applies globally to the whole image and every character.",
		"Negative prompts are not better just because they contain more tags.",
		"Do not add long generic negative tag lists beyond the fixed base sequence.",
		"Each character `negative_prompt` applies only to that character.",
		"if a character `prompt` includes `boy`, that character's `negative_prompt` should include `girl`",
		"Avoid repeating tags from the base negative tag sequence in character `negative_prompt`.",
		"Keep each character `negative_prompt` as short as possible while still useful.",
		"single concise character-level drift negative such as `off_model`",
		"boy, male, wrong_outfit",
		"Ensure the base negative tag sequence appears in the shared `negative_prompt`.",
		"Ensure character `negative_prompt` values avoid unnecessary duplication of tags from the base negative tag sequence.",
		"Ensure negative prompts stay minimal and targeted; do not add extra negative tags just to make them longer.",
		"NovelAI prompt weights represent influence strength or model focus",
		"heavy_rain, 1.3::rain::",
		"-1::monochrome::",
		"Order tags as: subject count/subject type, character/series, appearance, clothing, pose/action, composition, scene, lighting/atmosphere, style, quality tags, other restrictions.",
		"the global `prompt` order is: subject count/subject type, composition, scene, lighting/atmosphere, style, quality tags, other restrictions.",
		"each character `prompt` order is: character/series, appearance, clothing, pose/action.",
		"Put the most important tags as early as possible within the correct order group.",
		"Ensure the required quality tag sequence appears exactly once in the global `prompt`.",
		"The `characters` array length must match the actual number of distinct characters you inferred from the request.",
	} {
		if !strings.Contains(translateSystemPrompt, expected) {
			t.Fatalf("expected system prompt to contain %q", expected)
		}
	}
	if strings.Contains(translateSystemPrompt, "tool calling") || strings.Contains(translateSystemPrompt, "translate_prompt") {
		t.Fatalf("did not expect tool-calling instructions in system prompt")
	}
}

func TestParseTranslationRequiresCharacters(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing characters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationSupportsLegacyFieldNames(t *testing.T) {
	translation, err := parseTranslation(`{"positivePrompt":"moonlit girl","negativePrompt":"blurry"}`)
	if err != nil {
		t.Fatalf("parse translation: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 0 {
		t.Fatalf("expected empty legacy characters, got %#v", translation.Characters)
	}
}

func TestParseTranslationRejectsInvalidCharacterPosition(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl","negative_prompt":"bad hands","position":"Z9"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid position") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationRejectsEmptyNegativePrompt(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"","characters":[]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing negative_prompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTranslationRejectsEmptyCharacterNegativePrompt(t *testing.T) {
	_, err := parseTranslation(`{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl","negative_prompt":"","position":"C3"}]}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "characters[0] missing negative_prompt") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTranslateSendsJSONOutputRequest(t *testing.T) {
	var requestBody map[string]any
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages payload: %#v", requestBody["messages"])
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected user message payload: %#v", messages[1])
	}
	content, ok := userMessage["content"].(string)
	if !ok {
		t.Fatalf("unexpected user message content: %#v", userMessage["content"])
	}
	if !strings.Contains(content, "request=画一个月下的少女") {
		t.Fatalf("expected request payload, got %q", content)
	}
	if strings.Contains(content, "shape=") {
		t.Fatalf("did not expect shape payload, got %q", content)
	}

	if _, ok := requestBody["tools"]; ok {
		t.Fatalf("did not expect tools in request body: %#v", requestBody["tools"])
	}
	if _, ok := requestBody["tool_choice"]; ok {
		t.Fatalf("did not expect tool_choice in request body: %#v", requestBody["tool_choice"])
	}
	if _, ok := requestBody["reasoning_effort"]; ok {
		t.Fatalf("did not expect reasoning_effort without explicit config: %#v", requestBody["reasoning_effort"])
	}

	responseFormat, ok := requestBody["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected response_format: %#v", requestBody["response_format"])
	}
	if responseFormat["type"] != "json_object" {
		t.Fatalf("unexpected response_format.type: %#v", responseFormat["type"])
	}
}

func TestTranslateSendsReasoningEffortWhenConfigured(t *testing.T) {
	var requestBody map[string]any
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[]}`)), nil
	})
	client.cfg.ReasoningEffort = " custom-effort "

	if _, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare); err != nil {
		t.Fatalf("translate: %v", err)
	}
	if requestBody["reasoning_effort"] != "custom-effort" {
		t.Fatalf("unexpected reasoning_effort: %#v", requestBody["reasoning_effort"])
	}
}

func TestTranslateParsesAssistantJSONResponse(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[{"prompt":"girl, long hair","negative_prompt":"bad hands","position":"C3"}]}`)), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
	if len(translation.Characters) != 1 || translation.Characters[0].Position != "C3" {
		t.Fatalf("unexpected characters: %#v", translation.Characters)
	}
}

func TestTranslateParsesSSEAssistantJSONResponse(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			sseChunk(t, map[string]any{"content": "{\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "prompt":"moonlit girl",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "negative_prompt":"blurry",` + "\n"}),
			"",
			sseChunk(t, map[string]any{"content": `  "characters":[]` + "\n}"}),
			"",
			"data: [DONE]",
		}, "\n")

		return newHTTPResponse(http.StatusOK, body), nil
	})

	translation, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if translation.Prompt != "moonlit girl" {
		t.Fatalf("unexpected prompt: %q", translation.Prompt)
	}
	if translation.NegativePrompt != "blurry" {
		t.Fatalf("unexpected negative prompt: %q", translation.NegativePrompt)
	}
}

func TestTranslateRejectsInvalidAssistantJSON(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry","characters":[]}}`)), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse llm json") {
		t.Fatalf("expected parse llm json error, got %v", err)
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "parse llm json content failed") {
		t.Fatalf("expected parse failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `assistant_content="{\"prompt\":\"moonlit girl\"`) {
		t.Fatalf("expected assistant_content log, got %s", logOutput)
	}
}

func TestTranslateLogsRequestAndTranslatedPrompts(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, completionWithContent(t, `{"prompt":"moonlit girl","negative_prompt":"blurry, lowres","characters":[{"prompt":"girl, long hair","negative_prompt":"bad hands","position":"C3"}]}`)), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"llm request started",
		"base_url=https://api.openai.com/v1",
		"model=gpt-4o-mini",
		"attempt=1",
		"llm translated",
		`prompt="moonlit girl"`,
		`negative_prompt="blurry, lowres"`,
		"characters=",
		"girl, long hair",
		"bad hands",
		"C3",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in logs, got %s", expected, logOutput)
		}
	}
	if strings.Contains(logOutput, "response_mode=") {
		t.Fatalf("did not expect response_mode in success log, got %s", logOutput)
	}
}

func TestTranslateLogsRawResponseOnUnsupportedFormat(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, `{"unexpected":"payload"}`), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "extract llm json content failed") {
		t.Fatalf("expected failure log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, `raw_response="{\"unexpected\":\"payload\"}"`) {
		t.Fatalf("expected raw response log, got %s", logOutput)
	}
}

func TestTranslateLogsFullRawResponseWithoutTruncation(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	rawContent := `{"prompt":"moonlit girl"}`
	fullResponse := `{"choices":[{"message":{"content":"` + rawContent + `}"}}]}__RAW_RESPONSE_TAIL__`
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newHTTPResponse(http.StatusOK, fullResponse), nil
	})

	_, err := client.Translate(context.Background(), "画一个月下的少女", draw.ShapeSquare)
	if err == nil {
		t.Fatal("expected error")
	}
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "__RAW_RESPONSE_TAIL__") {
		t.Fatalf("expected full raw response in logs, got %s", logOutput)
	}
}

func newTestClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *TranslateClient {
	t.Helper()

	return &TranslateClient{
		cfg: config.LLM{
			BaseURL:    "https://api.openai.com/v1",
			APIKey:     "key",
			Model:      "gpt-4o-mini",
			TimeoutSec: 10,
		},
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
	}
}

func completionWithContent(t *testing.T, content string) string {
	t.Helper()

	return mustJSON(t, map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": content,
				},
			},
		},
	})
}

func sseChunk(t *testing.T, delta map[string]any) string {
	t.Helper()

	return "data: " + mustJSON(t, map[string]any{
		"id": "1",
		"choices": []any{
			map[string]any{
				"delta": delta,
			},
		},
	})
}

func newHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(payload)
}
