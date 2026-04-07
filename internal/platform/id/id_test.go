package id

import "testing"

func TestUUIDGeneratorNewString(t *testing.T) {
	generator := NewUUIDGenerator()
	first := generator.NewString()
	second := generator.NewString()

	if first == "" {
		t.Fatal("expected first id to be non-empty")
	}
	if second == "" {
		t.Fatal("expected second id to be non-empty")
	}
	if first == second {
		t.Fatalf("expected distinct ids, got %q", first)
	}
}

func TestStaticGeneratorNewString(t *testing.T) {
	generator := NewStaticGenerator("task-1")

	if got := generator.NewString(); got != "task-1" {
		t.Fatalf("unexpected id: %q", got)
	}
}
