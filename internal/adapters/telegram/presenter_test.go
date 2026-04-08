package telegram

import (
	"strings"
	"testing"

	requestapp "grimoire/internal/app/request"
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

func TestBuildPendingRequestTextAndMarkup(t *testing.T) {
	pending := requestapp.PendingRequest{
		Request: "draw a moonlit girl",
		ConfirmAction: requestapp.Action{
			CallbackData: "request:confirm:session-1",
		},
		ReviseAction: requestapp.Action{
			CallbackData: "request:revise:session-1",
		},
	}

	text := buildPendingRequestText(pending.Request)
	for _, expected := range []string{"待确认 request", "draw a moonlit girl", "请确认执行"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in text, got %s", expected, text)
		}
	}

	markup := requestDecisionMarkup(pending)
	if len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 2 {
		t.Fatalf("unexpected request markup: %#v", markup)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "request:confirm:session-1" {
		t.Fatalf("unexpected confirm callback: %#v", markup.InlineKeyboard[0][0])
	}
	if markup.InlineKeyboard[0][1].CallbackData != "request:revise:session-1" {
		t.Fatalf("unexpected revise callback: %#v", markup.InlineKeyboard[0][1])
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

	text := buildPromptText(" masterpiece, moonlit_girl ")
	if !strings.Contains(text, "Prompt") || !strings.Contains(text, "masterpiece, moonlit_girl") {
		t.Fatalf("unexpected prompt text: %s", text)
	}
}
