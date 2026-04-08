package telegram

import (
	"strings"
	"testing"

	taskapp "grimoire/internal/app/task"
	domaindraw "grimoire/internal/domain/draw"
	domainnai "grimoire/internal/domain/nai"
	domainpreferences "grimoire/internal/domain/preferences"
)

func TestBuildImageMenuTextIncludesNoticeAndFallbackArtist(t *testing.T) {
	pref := domainpreferences.DefaultPreference()
	text := buildImageMenuText("已更新", pref)

	for _, expected := range []string{"已更新", "当前尺寸", "当前画师串: 未设置"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text, got %s", expected, text)
		}
	}
}

func TestImageMenuMarkupUsesRequestCallbackProtocol(t *testing.T) {
	markup := imageMenuMarkup()
	if markup == nil || len(markup.InlineKeyboard) != 4 {
		t.Fatalf("unexpected image menu markup: %#v", markup)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "request:shape:small-portrait" {
		t.Fatalf("unexpected small portrait callback: %#v", markup.InlineKeyboard[0][0])
	}
	if markup.InlineKeyboard[3][0].CallbackData != requestArtistsSet {
		t.Fatalf("unexpected set artists callback: %#v", markup.InlineKeyboard[3][0])
	}
	if markup.InlineKeyboard[3][1].CallbackData != requestArtistsClear {
		t.Fatalf("unexpected clear artists callback: %#v", markup.InlineKeyboard[3][1])
	}
}

func TestBuildArtistsPromptText(t *testing.T) {
	text := buildArtistsPromptText()
	for _, expected := range []string{"请发送新的画师串", "/start 取消"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text, got %s", expected, text)
		}
	}
}

func TestBuildStartTextIncludesNewSessionCommand(t *testing.T) {
	text := buildStartText()
	for _, expected := range []string{"/new", "新建一个会话"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text, got %s", expected, text)
		}
	}
}

func TestBuildBalanceText(t *testing.T) {
	text := buildBalanceText(domainnai.AccountBalance{
		PurchasedTrainingSteps: 321,
		FixedTrainingStepsLeft: 45,
		TrialImagesLeft:        6,
		SubscriptionTier:       2,
		SubscriptionActive:     true,
	})

	for _, expected := range []string{
		"购买余额: 321",
		"月度余额: 45",
		"试用剩余图片: 6",
		"订阅: 已激活 (tier=2)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text, got %s", expected, text)
		}
	}
}

func TestBuildTaskStartedText(t *testing.T) {
	if text := buildTaskStartedText(); text != "已开始绘图" {
		t.Fatalf("unexpected task started text: %q", text)
	}
}

func TestBuildNewSessionText(t *testing.T) {
	if text := buildNewSessionText(); text != "已开始新的会话，之前的对话不会影响后续需求。" {
		t.Fatalf("unexpected new session text: %q", text)
	}
}

func TestTaskProgressMarkup(t *testing.T) {
	markup := taskProgressMarkup("task-1")
	if markup == nil {
		t.Fatal("expected progress markup")
	}
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected progress markup: %#v", markup)
	}
	if markup.InlineKeyboard[0][0].Text != "停止任务" {
		t.Fatalf("unexpected progress button: %#v", markup.InlineKeyboard[0][0])
	}
	if markup.InlineKeyboard[0][0].CallbackData != "task:stop:task-1" {
		t.Fatalf("unexpected progress callback: %#v", markup.InlineKeyboard[0][0])
	}
}

func TestResultTaskMarkupAndPromptText(t *testing.T) {
	markup := resultTaskMarkup("task-1")
	if markup == nil {
		t.Fatal("expected result markup")
	}
	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("unexpected result markup rows: %#v", markup)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "task:prompt:task-1" {
		t.Fatalf("unexpected prompt callback: %#v", markup.InlineKeyboard[0][0])
	}
	if markup.InlineKeyboard[1][0].CallbackData != "task:retry:translate:task-1" {
		t.Fatalf("unexpected retry translate callback: %#v", markup.InlineKeyboard[1][0])
	}
	if markup.InlineKeyboard[1][1].CallbackData != "task:retry:draw:task-1" {
		t.Fatalf("unexpected retry draw callback: %#v", markup.InlineKeyboard[1][1])
	}

	text := buildPromptText(taskapp.PromptDetails{
		Prompt:         " masterpiece, moonlit_girl ",
		NegativePrompt: " blurry ",
		Characters: []domaindraw.CharacterPrompt{
			{Prompt: "kinich_(genshin_impact)", NegativePrompt: "extra_arms", Position: "C3"},
		},
	})
	if !strings.Contains(text, "Global Prompt") || !strings.Contains(text, "masterpiece, moonlit_girl") || !strings.Contains(text, "Negative Prompt") || !strings.Contains(text, "Character 1") {
		t.Fatalf("unexpected prompt text: %s", text)
	}
}
