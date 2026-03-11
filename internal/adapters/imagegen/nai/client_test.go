package nai

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"grimoire/internal/config"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestResolveDimensions(t *testing.T) {
	testCases := []struct {
		shape  domaindraw.Shape
		width  int
		height int
	}{
		{shape: domaindraw.ShapeSquare, width: 1024, height: 1024},
		{shape: domaindraw.ShapeLandscape, width: 1216, height: 832},
		{shape: domaindraw.ShapePortrait, width: 832, height: 1216},
	}

	for _, tc := range testCases {
		width, height, err := resolveDimensions(tc.shape)
		if err != nil {
			t.Fatalf("resolve dimensions for %s: %v", tc.shape, err)
		}
		if width != tc.width || height != tc.height {
			t.Fatalf("unexpected dimensions for %s: %dx%d", tc.shape, width, height)
		}
	}
}

func TestSubmitBuildsV45PayloadAndPollCompletes(t *testing.T) {
	var requestBody map[string]any
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		if req.URL.String() != "https://image.novelai.net/ai/generate-image" {
			t.Fatalf("unexpected url: %s", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(payload, &requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		return newBinaryResponse(http.StatusOK, buildZip(t, map[string]string{"image_0.png": "png"})), nil
	})

	jobID, err := client.Submit(context.Background(), domaindraw.GenerateRequest{
		Prompt:         "street at night",
		NegativePrompt: "blurry",
		Characters: []domaindraw.CharacterPrompt{
			{Prompt: "girl, long hair", NegativePrompt: "bad hands", Position: "A1"},
		},
		Shape:   domaindraw.ShapePortrait,
		Artists: "artist:foo",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if strings.TrimSpace(jobID) == "" {
		t.Fatal("expected job id")
	}

	if requestBody["model"] != supportedModel {
		t.Fatalf("unexpected model: %#v", requestBody["model"])
	}
	if requestBody["action"] != "generate" {
		t.Fatalf("unexpected action: %#v", requestBody["action"])
	}
	if requestBody["input"] != "street at night, location, very aesthetic, masterpiece, no text" {
		t.Fatalf("unexpected input: %#v", requestBody["input"])
	}

	parameters := requestBody["parameters"].(map[string]any)
	if parameters["width"] != float64(832) || parameters["height"] != float64(1216) {
		t.Fatalf("unexpected dimensions: %#v x %#v", parameters["width"], parameters["height"])
	}
	if parameters["params_version"] != float64(3) {
		t.Fatalf("unexpected params_version: %#v", parameters["params_version"])
	}
	if parameters["qualityToggle"] != true {
		t.Fatalf("unexpected qualityToggle: %#v", parameters["qualityToggle"])
	}
	if parameters["sampler"] != "k_euler_ancestral" {
		t.Fatalf("unexpected sampler: %#v", parameters["sampler"])
	}
	if parameters["steps"] != float64(23) {
		t.Fatalf("unexpected steps: %#v", parameters["steps"])
	}
	if parameters["scale"] != float64(5) {
		t.Fatalf("unexpected scale: %#v", parameters["scale"])
	}
	if parameters["negative_prompt"] != baseNegative+", blurry" {
		t.Fatalf("unexpected negative prompt: %#v", parameters["negative_prompt"])
	}
	if parameters["uc"] != baseNegative+", blurry" {
		t.Fatalf("unexpected uc: %#v", parameters["uc"])
	}

	characterPrompts := parameters["characterPrompts"].([]any)
	if len(characterPrompts) != 1 {
		t.Fatalf("unexpected characterPrompts: %#v", characterPrompts)
	}
	firstCharacter := characterPrompts[0].(map[string]any)
	if firstCharacter["prompt"] != "girl, long hair" {
		t.Fatalf("unexpected character prompt: %#v", firstCharacter["prompt"])
	}
	if firstCharacter["uc"] != "bad hands" {
		t.Fatalf("unexpected character negative prompt: %#v", firstCharacter["uc"])
	}
	center := firstCharacter["center"].(map[string]any)
	if center["x"] != 0.1 || center["y"] != 0.1 {
		t.Fatalf("unexpected character center: %#v", center)
	}

	v4Prompt := parameters["v4_prompt"].(map[string]any)
	v4PromptCaption := v4Prompt["caption"].(map[string]any)
	if v4PromptCaption["base_caption"] != "street at night, location, very aesthetic, masterpiece, no text" {
		t.Fatalf("unexpected v4 base caption: %#v", v4PromptCaption["base_caption"])
	}
	v4Chars := v4PromptCaption["char_captions"].([]any)
	if len(v4Chars) != 1 {
		t.Fatalf("unexpected v4 chars: %#v", v4Chars)
	}

	v4NegativePrompt := parameters["v4_negative_prompt"].(map[string]any)
	v4NegativeCaption := v4NegativePrompt["caption"].(map[string]any)
	if v4NegativeCaption["base_caption"] != baseNegative+", blurry" {
		t.Fatalf("unexpected v4 negative base caption: %#v", v4NegativeCaption["base_caption"])
	}

	update, err := client.Poll(context.Background(), jobID)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if update.Status != domaindraw.JobCompleted {
		t.Fatalf("unexpected status: %s", update.Status)
	}
	if update.QueuePosition != 0 {
		t.Fatalf("unexpected queue position: %d", update.QueuePosition)
	}
	if string(update.Image) != "png" {
		t.Fatalf("unexpected image data: %q", update.Image)
	}
	if _, err := client.Poll(context.Background(), jobID); err == nil {
		t.Fatal("expected poll cache miss after consume")
	}
}

func TestSubmitAcceptsCreatedStatus(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newBinaryResponse(http.StatusCreated, buildZip(t, map[string]string{"image_0.png": "png"})), nil
	})

	jobID, err := client.Submit(context.Background(), domaindraw.GenerateRequest{
		Prompt: "street at night",
		Shape:  domaindraw.ShapeSquare,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if strings.TrimSpace(jobID) == "" {
		t.Fatal("expected job id")
	}
}

func TestSubmitLogsRequestMetadata(t *testing.T) {
	logBuffer := &bytes.Buffer{}
	client := newTestClient(t, slog.New(slog.NewTextHandler(logBuffer, nil)), func(req *http.Request) (*http.Response, error) {
		return newBinaryResponse(http.StatusOK, buildZip(t, map[string]string{"image_0.png": "png"})), nil
	})

	_, err := client.Submit(context.Background(), domaindraw.GenerateRequest{
		Prompt:  "street at night",
		Shape:   domaindraw.ShapeLandscape,
		Artists: "artist:foo",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	logOutput := logBuffer.String()
	for _, expected := range []string{
		"nai request started",
		"base_url=https://image.novelai.net",
		"model=nai-diffusion-4-5-full",
		"attempt=1",
		"shape=landscape",
		"artists=artist:foo",
	} {
		if !strings.Contains(logOutput, expected) {
			t.Fatalf("expected %q in log output, got %s", expected, logOutput)
		}
	}
}

func TestSubmitReturnsErrorOnInvalidZip(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		return newBinaryResponse(http.StatusOK, []byte("not-a-zip")), nil
	})

	_, err := client.Submit(context.Background(), domaindraw.GenerateRequest{
		Prompt: "street at night",
		Shape:  domaindraw.ShapeSquare,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "open generated zip") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPollRejectsUnknownJobID(t *testing.T) {
	client := newTestClient(t, nil, nil)
	if _, err := client.Poll(context.Background(), "missing"); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetBalanceReadsUserData(t *testing.T) {
	client := newTestClient(t, nil, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", req.Method)
		}
		if req.URL.String() != balanceURL {
			t.Fatalf("unexpected url: %s", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}

		return newJSONResponse(http.StatusOK, `{
			"subscription":{
				"tier":1,
				"active":true,
				"trainingStepsLeft":{
					"fixedTrainingStepsLeft":23,
					"purchasedTrainingSteps":456
				}
			},
			"information":{
				"trialImagesLeft":12
			}
		}`), nil
	})

	balance, err := client.GetBalance(context.Background())
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}

	expected := domainnai.AccountBalance{
		PurchasedTrainingSteps: 456,
		FixedTrainingStepsLeft: 23,
		TrialImagesLeft:        12,
		SubscriptionTier:       1,
		SubscriptionActive:     true,
	}
	if balance != expected {
		t.Fatalf("unexpected balance: %#v", balance)
	}
}

func newTestClient(t *testing.T, logger *slog.Logger, transport roundTripFunc) *Client {
	t.Helper()

	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if transport == nil {
		transport = func(req *http.Request) (*http.Response, error) {
			return newBinaryResponse(http.StatusOK, buildZip(t, map[string]string{"image_0.png": "png"})), nil
		}
	}

	return &Client{
		cfg: config.Config{
			NAI: config.NAI{
				BaseURL:    "https://image.novelai.net",
				APIKey:     "key",
				Model:      supportedModel,
				TimeoutSec: 10,
			},
		},
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
		now: func() time.Time {
			return time.Unix(100, 0)
		},
		completed: make(map[string][]byte),
	}
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buffer.Bytes()
}

func newBinaryResponse(statusCode int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func newJSONResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
