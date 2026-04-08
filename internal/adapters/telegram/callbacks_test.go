package telegram

import (
	"testing"

	domaindraw "grimoire/internal/domain/draw"
)

func TestParseRequestAction(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   requestAction
		wantOK bool
	}{
		{
			name:   "shape callback",
			input:  requestShapeCallback(domaindraw.ShapeLargeLandscape),
			want:   requestAction{Kind: requestActionUpdateShape, Shape: domaindraw.ShapeLargeLandscape},
			wantOK: true,
		},
		{
			name:   "set artists callback",
			input:  requestArtistsSet,
			want:   requestAction{Kind: requestActionSetArtists},
			wantOK: true,
		},
		{
			name:   "clear artists callback",
			input:  requestArtistsClear,
			want:   requestAction{Kind: requestActionClearArtists},
			wantOK: true,
		},
		{
			name:   "invalid shape callback",
			input:  "request:shape:unknown-shape",
			want:   requestAction{},
			wantOK: false,
		},
		{
			name:   "request decision should not parse here",
			input:  "request:confirm:session-1",
			want:   requestAction{},
			wantOK: false,
		},
		{
			name:   "task action should not parse here",
			input:  "task:stop:1",
			want:   requestAction{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRequestAction(tt.input)
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
