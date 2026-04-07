package clock

import (
	"testing"
	"time"
)

func TestSystemClockNowReturnsTime(t *testing.T) {
	before := time.Now().Add(-time.Second)
	got := NewSystemClock().Now()
	after := time.Now().Add(time.Second)

	if got.Before(before) || got.After(after) {
		t.Fatalf("unexpected system clock time: %s", got)
	}
}

func TestFixedClockNowReturnsConfiguredTime(t *testing.T) {
	expected := time.Date(2026, 4, 7, 16, 45, 0, 0, time.UTC)
	got := NewFixedClock(expected).Now()

	if !got.Equal(expected) {
		t.Fatalf("unexpected fixed clock time: %s", got)
	}
}
