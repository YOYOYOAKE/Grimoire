package draw

import "testing"

func TestKnownShapesRemainValidAndKeepLabels(t *testing.T) {
	tests := map[Shape]string{
		ShapeSmallPortrait:  "Small Portrait (512x768)",
		ShapeSmallLandscape: "Small Landscape (768x512)",
		ShapeSmallSquare:    "Small Square (640x640)",
		ShapePortrait:       "Normal Portrait (832x1216)",
		ShapeLandscape:      "Normal Landscape (1216x832)",
		ShapeSquare:         "Normal Square (1024x1024)",
		ShapeLargePortrait:  "Large Portrait (1024x1536)",
		ShapeLargeLandscape: "Large Landscape (1536x1024)",
		ShapeLargeSquare:    "Large Square (1472x1472)",
	}

	for shape, label := range tests {
		if !shape.Valid() {
			t.Fatalf("expected %s to be valid", shape)
		}
		if got := shape.Label(); got != label {
			t.Fatalf("unexpected label for %s: %q", shape, got)
		}
	}
}

func TestShapeValidRejectsUnknownValue(t *testing.T) {
	if Shape("invalid").Valid() {
		t.Fatal("expected invalid shape to be rejected")
	}
}

func TestShapeLabelFallsBackToRawValue(t *testing.T) {
	if got := Shape("custom-shape").Label(); got != "custom-shape" {
		t.Fatalf("unexpected label: %q", got)
	}
}
