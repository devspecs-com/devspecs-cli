package profiles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAll_containsKnownProfiles(t *testing.T) {
	want := []string{"cursor", "claude", "codex", "openspec", "bmad", "speckit", "adr", "docs"}
	got := make(map[string]bool)
	for _, p := range All() {
		got[p.ID] = true
	}
	for _, id := range want {
		assert.True(t, got[id],
			"missing profile id %q", id)

	}
}

func TestByID_WithKnownProfile_ReturnsProfile(t *testing.T) {
	profile, ok := ByID("openspec")

	assert.True(t, ok)
	assert.Equal(t, "openspec", profile.SourceType)
}

func TestByID_WithUnknownProfile_ReturnsFalse(t *testing.T) {
	_, ok := ByID("nope")

	assert.False(t, ok)
}

func TestDetect_findsOpenspecDir(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "openspec"), 0o755))

	got := Detect(root, nil)
	require.True(t, slices.Contains(got, "openspec"),
		"want openspec in %v", got)

}

func TestCustomProfile(t *testing.T) {
	cp := CustomProfile()
	require.Equal(t, IDCustom, cp.ID,
		"custom id: got %q", cp.ID)

}

func TestBMADProfileHasPRDSubtypeRule(t *testing.T) {
	p, ok := ByID("bmad")
	require.True(t, ok,
		"bmad missing")

	var saw bool
	for _, r := range p.Rules {
		if r.Subtype == config.SubtypePRD && r.Kind == config.KindRequirements {
			saw = true
			break
		}
	}
	require.True(t, saw,
		"expected BMAD rule with requirements/prd subtype")

}
