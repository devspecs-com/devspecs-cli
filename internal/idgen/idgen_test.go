package idgen

import (
	"testing"
)

func TestShortID_Deterministic(t *testing.T) {
	id1 := ShortID("plans/foo.md|markdown")
	id2 := ShortID("plans/foo.md|markdown")
	if id1 != id2 {
		t.Errorf("ShortID not deterministic: %s != %s", id1, id2)
	}
}

func TestShortID_Length(t *testing.T) {
	id := ShortID("plans/foo.md|markdown")
	if len(id) != 8 {
		t.Errorf("expected 8 chars, got %d: %q", len(id), id)
	}
}

func TestShortID_IsHex(t *testing.T) {
	id := ShortID("test|markdown")
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char %c in ShortID %q", c, id)
		}
	}
}

func TestShortID_DifferentInputs(t *testing.T) {
	inputs := []string{
		"plans/foo.md|markdown",
		"plans/bar.md|markdown",
		"specs/api.md|markdown",
		"docs/adr/001.md|adr",
		"openspec/changes/add-foo|openspec",
		"plans/auth/login.md|markdown",
		"plans/auth/signup.md|markdown",
		"requirements.md|markdown",
		"v0.prd.md|markdown",
		"design.md|markdown",
		"contract.md|markdown",
	}

	seen := make(map[string]string)
	for _, input := range inputs {
		id := ShortID(input)
		if prev, ok := seen[id]; ok {
			t.Errorf("collision: %q and %q both produce %s", prev, input, id)
		}
		seen[id] = input
	}
}

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	id := f.New()
	if id == "" {
		t.Error("New() returned empty ID")
	}
	if len(id) < 10 {
		t.Errorf("ID too short: %q", id)
	}
	if id[:3] != "ds_" {
		t.Errorf("expected ds_ prefix, got %q", id)
	}
}

func TestNewWithPrefix(t *testing.T) {
	f := NewFactory()
	id := f.NewWithPrefix("rev_")
	if id[:4] != "rev_" {
		t.Errorf("expected rev_ prefix, got %q", id)
	}
}

func TestNew_Unique(t *testing.T) {
	f := NewFactory()
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := f.New()
		if ids[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		ids[id] = true
	}
}
