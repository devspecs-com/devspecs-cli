package idgen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortID_WithStableSourceIdentity_ReturnsExpectedHash(t *testing.T) {
	id := ShortID("plans/foo.md|markdown")

	assert.Equal(t, "521973f1", id)
}

func TestShortID_Length(t *testing.T) {
	id := ShortID("plans/foo.md|markdown")
	assert.Len(t, id, 8,
		"expected 8 chars, got %d: %q", len(id), id)

}

func TestShortID_IsHex(t *testing.T) {
	id := ShortID("test|markdown")
	for _, c := range id {
		assert.True(t, ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')),
			"non-hex char %c in ShortID %q", c, id)

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
		prev, ok := seen[id]
		assert.False(t, ok,
			"collision: %q and %q both produce %s", prev, input, id)

		seen[id] = input
	}
}

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	id := f.New()
	assert.NotEqual(t, "", id,
		"New() returned empty ID")
	assert.False(t, len(id) < 10,
		"ID too short: %q", id)
	assert.Equal(t, "ds_", id[:3],
		"expected ds_ prefix, got %q", id)

}

func TestNewWithPrefix(t *testing.T) {
	f := NewFactory()
	id := f.NewWithPrefix("rev_")
	assert.Equal(t, "rev_", id[:4],
		"expected rev_ prefix, got %q", id)

}

func TestNew_Unique(t *testing.T) {
	f := NewFactory()
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := f.New()
		assert.False(t, ids[id],
			"duplicate ID: %s", id)

		ids[id] = true
	}
}
