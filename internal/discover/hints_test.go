package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanHintCandidates_ExcludesGitignoredPaths(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(".cursor/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".cursor", "plans"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".cursor", "plans", "x.md"), []byte("#\n"), 0o644))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	c := discover.ScanHintCandidates(tmp, m)
	for _, h := range c {
		assert.NotEqual(t, ".cursor/plans", h.RelPath)
		assert.False(t, strings.HasPrefix(h.RelPath, ".cursor/"))

	}
}

func TestFormatSuggestCommand(t *testing.T) {
	md := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "plans", SourceType: "markdown"})
	require.Contains(t, md, "add-source markdown plans",
		"markdown: %q", md)

	adr := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "docs/adr", SourceType: "adr"})
	require.Contains(t, adr, "add-source adr docs/adr",
		"adr: %q", adr)

	ospec := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "openspec", SourceType: "openspec"})
	require.Contains(t, ospec, "add-source openspec openspec",
		"openspec: %q", ospec)

}

func TestHintDisplayPath(t *testing.T) {
	require.Equal(t, "", discover.HintDisplayPath(""))
	require.Equal(t, "docs/", discover.HintDisplayPath("docs"),
		"%q", discover.HintDisplayPath("docs"))
	require.Equal(t, "docs/", discover.HintDisplayPath("docs/"),
		"%q", discover.HintDisplayPath("docs/"))

}

func TestScanHintCandidates_plansDirectory(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "plans"), 0o755))

	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "plans" {
			seen = true
			break
		}
	}
	require.True(t, seen,
		"%#v", c)

}

func TestScanHintCandidates_openspecBranch(t *testing.T) {
	tmp := t.TempDir()
	prop := filepath.Join(tmp, "openspec", "changes", "feat", "proposal.md")

	require.NoError(t, os.MkdirAll(filepath.Dir(prop), 0o755))

	require.NoError(t, os.WriteFile(prop, []byte("#\n"), 0o644))

	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "openspec" && h.SourceType == "openspec" {
			seen = true
			break
		}
	}
	require.True(t, seen,
		"%#v", c)

}

func TestScanHintCandidates_speckitFeatureDir(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "specs", "myfeat", "spec.md")

	require.NoError(t, os.MkdirAll(filepath.Dir(spec), 0o755))

	require.NoError(t, os.WriteFile(spec, []byte("#\n"), 0o644))

	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "specs/myfeat" {
			seen = true
			break
		}
	}
	require.True(t, seen,
		"%#v", c)

}

func TestScanHintCandidates_AgentPlanDirs(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{".claude/plans", ".codex/plans"} {
		if err := os.MkdirAll(filepath.Join(tmp, filepath.FromSlash(rel)), 0o755); err != nil {
			require.NoError(t, err)
		}
	}
	c := discover.ScanHintCandidates(tmp, nil)
	for _, rel := range []string{".claude/plans", ".codex/plans"} {
		var seen bool
		for _, h := range c {
			if h.RelPath == rel {
				seen = true
				break
			}
		}
		require.True(t, seen,
			"missing %s in %#v", rel, c)

	}
}

func TestScanHintCandidates_docsDense(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"a.plan.md", "b.plan.md"} {
		p := filepath.Join(tmp, "docs", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			require.NoError(t, err)
		}
		if err := os.WriteFile(p, []byte("#\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}
	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "docs" {
			seen = true
			break
		}
	}
	require.True(t, seen,
		"%#v", c)

}

func TestScanHintCandidates_adrPaths(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{"docs/adr", "adr"} {
		if err := os.MkdirAll(filepath.Join(tmp, filepath.FromSlash(rel)), 0o755); err != nil {
			require.NoError(t, err)
		}
	}
	c := discover.ScanHintCandidates(tmp, nil)
	seen := map[string]bool{}
	for _, h := range c {
		if h.SourceType == "adr" {
			seen[h.RelPath] = true
		}
	}
	assert.True(t, seen["docs/adr"])
	assert.True(t, seen["adr"])

}
