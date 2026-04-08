package telegram

import (
	"strings"
	"testing"

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
