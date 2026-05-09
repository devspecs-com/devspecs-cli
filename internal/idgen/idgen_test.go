package idgen

import (
	"strings"
	"testing"
)

func TestNew_Format(t *testing.T) {
	f := NewFactory()
	id := f.New()
	if !strings.HasPrefix(id, "ds_") {
		t.Errorf("expected ds_ prefix, got %q", id)
	}
	// ds_ + 26 char ULID = 29 chars total
	if len(id) != 29 {
		t.Errorf("expected 29 chars, got %d: %q", len(id), id)
	}
}

func TestNew_Unique(t *testing.T) {
	f := NewFactory()
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := f.New()
		if seen[id] {
			t.Fatalf("duplicate ID at iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewWithPrefix(t *testing.T) {
	f := NewFactory()
	id := f.NewWithPrefix("rev_")
	if !strings.HasPrefix(id, "rev_") {
		t.Errorf("expected rev_ prefix, got %q", id)
	}
}
