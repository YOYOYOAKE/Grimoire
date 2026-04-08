package telegram

import (
	"testing"

	domaindraw "grimoire/internal/domain/draw"
)

func TestParseCallbackAction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   callbackAction
		wantOK bool
	}{
		{
			name:   "shape callback",
			input:  cbShapeLargeLandscape,
			want:   callbackAction{Kind: callbackActionUpdateShape, Shape: domaindraw.ShapeLargeLandscape},
			wantOK: true,
		},
		{
			name:   "set artists callback",
			input:  cbSetArtists,
			want:   callbackAction{Kind: callbackActionSetArtists},
			wantOK: true,
		},
		{
			name:   "clear artists callback",
			input:  cbClearArtists,
			want:   callbackAction{Kind: callbackActionClearArtists},
			wantOK: true,
		},
		{
			name:   "invalid callback",
			input:  "task:stop:1",
			want:   callbackAction{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCallbackAction(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("unexpected ok=%v", ok)
			}
			if got != tt.want {
				t.Fatalf("unexpected action: %#v", got)
			}
		})
	}
}

func TestParseTaskAction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   taskAction
		wantOK bool
	}{
		{
			name:   "stop task",
			input:  "task:stop:task-1",
			want:   taskAction{Kind: taskActionStop, TaskID: "task-1"},
			wantOK: true,
		},
		{
			name:   "task prompt",
			input:  "task:prompt:task-1",
			want:   taskAction{Kind: taskActionPrompt, TaskID: "task-1"},
			wantOK: true,
		},
		{
			name:   "retry translate",
			input:  "task:retry:translate:task-1",
			want:   taskAction{Kind: taskActionRetryTranslate, TaskID: "task-1"},
			wantOK: true,
		},
		{
			name:   "retry draw",
			input:  "task:retry:draw:task-1",
			want:   taskAction{Kind: taskActionRetryDraw, TaskID: "task-1"},
			wantOK: true,
		},
		{
			name:   "invalid blank task id",
			input:  "task:stop:   ",
			want:   taskAction{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseTaskAction(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("unexpected ok=%v", ok)
			}
			if got != tt.want {
				t.Fatalf("unexpected action: %#v", got)
			}
		})
	}
}
