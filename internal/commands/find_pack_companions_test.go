package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFindPackCompanionCandidatesAddsDirectTestFile(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	all = append(all, retrieval.Candidate{
		ID:    "map-test",
		Path:  "internal/commands/map_test.go",
		Kind:  "source_context",
		Title: "map command tests",
	})

	got := addFindPackCompanionCandidates(context.Background(), "", "map command output", matches, all, findPackCompanionModeGeneric)
	assert.True(t, findPackTestHasPath(got, "internal/commands/map_test.go"),
		"expected direct test companion, got %#v", findPackTestPaths(got))

	companion := findPackTestCandidate(got, "internal/commands/map_test.go")
	assert.Equal(t, "test_companion", companion.Metadata["retrieval_expansion_reason"],
		"expected test companion reason, got %#v", companion.Metadata)
	assert.Equal(t, retrieval.PackTierRelated, companion.Metadata["pack_tier"],
		"expected related pack tier, got %#v", companion.Metadata)

}

func TestAddFindPackCompanionCandidatesAddsFilesystemTestCompanion(t *testing.T) {
	repoRoot := t.TempDir()
	writeFindPackTestFile(t, repoRoot, "internal/commands/map_test.go", "package commands\n\nfunc TestMapOutput(t *testing.T) {}\n")
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}

	got := addFindPackCompanionCandidates(context.Background(), repoRoot, "map command output", matches, matches, findPackCompanionModeGeneric)

	companion := findPackTestCandidate(got, "internal/commands/map_test.go")
	assert.NotEqual(t, "", companion.Path,
		"expected filesystem test companion, got %#v", findPackTestPaths(got))
	assert.Equal(t, "test_case", companion.Subtype,
		"expected filesystem test companion subtype test_case, got %#v", companion)
	assert.Equal(t, "query_time_pack_companion", companion.Metadata["admission_reason"],
		"expected query-time companion admission metadata, got %#v", companion.Metadata)

}

func TestAddFindPackCompanionCandidatesAddsCommandFamilyFiles(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	{
		path := "internal/commands/find.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/find_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/map_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/read_commands_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/refresh.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/freshness_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}

	got := addFindPackCompanionCandidates(context.Background(), "", "auto scan map and find first use output cache", matches, all, findPackCompanionModeAll)

	assert.True(t, findPackTestHasPath(got, "internal/commands/map_test.go"),
		"expected %s in command family companions, got %#v", "internal/commands/map_test.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/find.go"),
		"expected %s in command family companions, got %#v", "internal/commands/find.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/find_test.go"),
		"expected %s in command family companions, got %#v", "internal/commands/find_test.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/read_commands_test.go"),
		"expected %s in command family companions, got %#v", "internal/commands/read_commands_test.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/refresh.go"),
		"expected %s in command family companions, got %#v", "internal/commands/refresh.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/freshness_test.go"),
		"expected %s in command family companions, got %#v", "internal/commands/freshness_test.go", findPackTestPaths(got))

}

func TestScoreFindGitReceiptsReturnsRelatedTouchedPaths(t *testing.T) {
	receipts := scoreFindGitReceipts([]parsedFindGitCommit{
		{
			sha:         "abc123456",
			committedAt: "2026-06-04",
			subject:     "Tighten map command output tests",
			paths: []string{
				"internal/commands/map.go",
				"internal/commands/map_test.go",
				"internal/commands/read_commands_test.go",
				"docs/map-output.md",
			},
		},
	}, []string{"internal/commands/map.go"}, "map command output")
	require.Len(t, receipts, 1,
		"expected one receipt, got %#v", receipts)

	assert.True(t, findPackStringSliceContains(receipts[0].RelatedPaths, "internal/commands/map_test.go"),
		"expected related path %s, got %#v", "internal/commands/map_test.go", receipts[0].RelatedPaths)
	assert.True(t, findPackStringSliceContains(receipts[0].RelatedPaths, "internal/commands/read_commands_test.go"),
		"expected related path %s, got %#v", "internal/commands/read_commands_test.go", receipts[0].RelatedPaths)

	assert.False(t, findPackStringSliceContains(receipts[0].RelatedPaths, "docs/map-output.md"),
		"did not expect doc path in related pack diagnostics: %#v", receipts[0].RelatedPaths)

}

func TestWriteGitTrustTextShowsRelatedTouchedPaths(t *testing.T) {
	buf := &bytes.Buffer{}
	writeGitTrustText(buf, &FindGitTrustContext{
		Receipts: []FindGitReceipt{
			{
				ShortSHA:     "abc1234",
				CommittedAt:  "2026-06-04",
				Subject:      "Tighten map command output tests",
				MatchedPaths: []string{"internal/commands/map.go"},
				RelatedPaths: []string{"internal/commands/map_test.go", "internal/commands/read_commands_test.go"},
			},
		},
	})

	output := buf.String()
	assert.Contains(t, output, "Related files from matching commits, not admitted to pack:",
		"git trust text missing %q:\n%s", "Related files from matching commits, not admitted to pack:", output)
	assert.Contains(t, output, "- internal/commands/map_test.go",
		"git trust text missing %q:\n%s", "- internal/commands/map_test.go", output)
	assert.Contains(t, output, "- internal/commands/read_commands_test.go",
		"git trust text missing %q:\n%s", "- internal/commands/read_commands_test.go", output)

}

func TestAddFindPackCompanionCandidatesAddsParentCommandForHelperFile(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "find-pack", Path: "internal/commands/find_pack.go", Kind: "source_context", Title: "find pack"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	{
		path := "internal/commands/find.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/find_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/find_pack_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}

	got := addFindPackCompanionCandidates(context.Background(), "", "boundary primary packs", matches, all, findPackCompanionModeAll)

	assert.True(t, findPackTestHasPath(got, "internal/commands/find.go"),
		"expected helper command family path %s, got %#v", "internal/commands/find.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/find_test.go"),
		"expected helper command family path %s, got %#v", "internal/commands/find_test.go", findPackTestPaths(got))
	assert.True(t, findPackTestHasPath(got, "internal/commands/find_pack_test.go"),
		"expected helper command family path %s, got %#v", "internal/commands/find_pack_test.go", findPackTestPaths(got))

}

func TestAddFindPackCompanionCandidatesModeOffAddsNothing(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	all = append(all, retrieval.Candidate{
		ID:    "map-test",
		Path:  "internal/commands/map_test.go",
		Kind:  "source_context",
		Title: "map command tests",
	})

	got := addFindPackCompanionCandidates(context.Background(), "", "map command output", matches, all, findPackCompanionModeOff)
	require.Len(t, got, len(matches),
		"expected no companions in off mode, got %#v", findPackTestPaths(got))

}

func TestAddFindPackCompanionCandidatesGenericModeSkipsCommandFamily(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	{
		path := "internal/commands/find.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/map_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}
	{
		path := "internal/commands/read_commands_test.go"

		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})

	}

	got := addFindPackCompanionCandidates(context.Background(), "", "map output cache", matches, all, findPackCompanionModeGeneric)
	assert.True(t, findPackTestHasPath(got, "internal/commands/map_test.go"),
		"expected generic direct test companion, got %#v", findPackTestPaths(got))

	assert.False(t, findPackTestHasPath(got, "internal/commands/find.go"),
		"generic mode should not include command-family path %s: %#v", "internal/commands/find.go", findPackTestPaths(got))
	assert.False(t, findPackTestHasPath(got, "internal/commands/read_commands_test.go"),
		"generic mode should not include command-family path %s: %#v", "internal/commands/read_commands_test.go", findPackTestPaths(got))

}

func TestNormalizeFindPackCompanionMode_WithEmptyInput_ReturnsAll(t *testing.T) {
	got := normalizeFindPackCompanionMode("")

	assert.Equal(t, findPackCompanionModeAll, got)
}

func TestNormalizeFindPackCompanionMode_WithAll_ReturnsAll(t *testing.T) {
	got := normalizeFindPackCompanionMode("all")

	assert.Equal(t, findPackCompanionModeAll, got)
}

func TestNormalizeFindPackCompanionMode_WithOff_ReturnsOff(t *testing.T) {
	got := normalizeFindPackCompanionMode("off")

	assert.Equal(t, findPackCompanionModeOff, got)
}

func TestNormalizeFindPackCompanionMode_WithNone_ReturnsOff(t *testing.T) {
	got := normalizeFindPackCompanionMode("none")

	assert.Equal(t, findPackCompanionModeOff, got)
}

func TestNormalizeFindPackCompanionMode_WithGeneric_ReturnsGeneric(t *testing.T) {
	got := normalizeFindPackCompanionMode("generic")

	assert.Equal(t, findPackCompanionModeGeneric, got)
}

func TestNormalizeFindPackCompanionMode_WithHyphenatedGenericGit_ReturnsGenericGit(t *testing.T) {
	got := normalizeFindPackCompanionMode("generic-git")

	assert.Equal(t, findPackCompanionModeGenericGit, got)
}

func TestNormalizeFindPackCompanionMode_WithUnderscoreGenericGit_ReturnsGenericGit(t *testing.T) {
	got := normalizeFindPackCompanionMode("generic_git")

	assert.Equal(t, findPackCompanionModeGenericGit, got)
}

func TestNormalizeFindPackCompanionMode_WithPlusGenericGit_ReturnsGenericGit(t *testing.T) {
	got := normalizeFindPackCompanionMode("generic+git")

	assert.Equal(t, findPackCompanionModeGenericGit, got)
}

func TestNormalizeFindPackCompanionMode_WithUnknownInput_ReturnsEmpty(t *testing.T) {
	got := normalizeFindPackCompanionMode("nonsense")

	assert.Empty(t, got)
}

func writeFindPackTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}

func findPackTestCandidate(candidates []retrieval.Candidate, path string) retrieval.Candidate {
	for _, candidate := range candidates {
		if candidate.Path == path {
			return candidate
		}
	}
	return retrieval.Candidate{}
}

func findPackTestHasPath(candidates []retrieval.Candidate, path string) bool {
	return findPackTestCandidate(candidates, path).Path != ""
}

func findPackTestPaths(candidates []retrieval.Candidate) []string {
	var out []string
	for _, candidate := range candidates {
		out = append(out, candidate.Path)
	}
	return out
}

func findPackStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
