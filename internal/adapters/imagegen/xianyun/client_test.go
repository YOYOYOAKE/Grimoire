package xianyun

import (
	"testing"

	"grimoire/internal/domain/draw"
)

func TestResolveDimensions(t *testing.T) {
	width, height, err := resolveDimensions(draw.ShapeLandscape)
	if err != nil {
		t.Fatalf("resolve dimensions: %v", err)
	}
	if width != 1216 || height != 832 {
		t.Fatalf("unexpected dimensions: %dx%d", width, height)
	}
}
