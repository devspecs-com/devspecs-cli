package indexquery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSourceManifestCandidateMode_WithMixedCaseWindow_ReturnsWindow(t *testing.T) {
	mode, err := ParseSourceManifestCandidateMode("  WINDOW ")

	require.NoError(t, err)
	assert.Equal(t, SourceManifestCandidateModeWindow, mode)
}

func TestParseSourceManifestCandidateMode_WithUnknownMode_ReturnsActionableError(t *testing.T) {
	mode, err := ParseSourceManifestCandidateMode("fast")

	assert.ErrorContains(t, err, "valid values: off, metadata, window")
	assert.Empty(t, mode)
}

func TestSourceManifestBodyWindow_WithRankedTerm_ReturnsBoundedExcerptAroundSpecificMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "oauth.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	body := strings.Repeat("generic setup line\n", 12) +
		"func handleSwaggerOAuthRedirect() error { return nil }\n" +
		strings.Repeat("generic cleanup line\n", 12)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	snippet := sourceManifestBodyWindow(root, "src/oauth.go", []string{"generic", "SwaggerOAuth"}, 120)

	assert.Contains(t, snippet, "handleSwaggerOAuthRedirect")
	assert.True(t, strings.HasPrefix(snippet, "...\n"))
	assert.True(t, strings.HasSuffix(snippet, "\n..."))
}

func TestSourceManifestBodyWindow_WithEscapingPath_ReturnsEmpty(t *testing.T) {
	root := t.TempDir()

	snippet := sourceManifestBodyWindow(root, "../outside.go", []string{"outside"}, 100)

	assert.Empty(t, snippet)
}

func TestSourceManifestWindowTermsByPriority_WithSpecificAndGenericTerms_RanksSpecificFirst(t *testing.T) {
	terms := []string{"docs", "oauth", "swagger_redirect", "client"}

	ranked := sourceManifestWindowTermsByPriority(terms)

	require.Len(t, ranked, 4)
	assert.Equal(t, "swagger_redirect", ranked[0])
	assert.Equal(t, "oauth", ranked[1])
	assert.Equal(t, "client", ranked[2])
	assert.Equal(t, "docs", ranked[3])
}

func TestLoadSourceManifestCandidatesForQueryMaterializesTestCandidate(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_src"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.ReplaceRepoSourceManifest(repoID,
			[]store.SourceManifestFileInput{{
				FileID: "src_1", RepoID: repoID, Path: "tests/session_test.go", ContentHash: "abc",
				Language: "go", SourceRoot: "tests", SourceRootKind: "test_root", SourceRole: "test", FirstPartyScore: 0.8,
			}},
			[]store.SourceManifestSymbolInput{{FileID: "src_1", Symbol: "TestRefreshSession", Kind: "test"}},
			[]store.SourceManifestTestInput{{FileID: "src_1", TestName: "TestRefreshSession"}},
			[]store.SourceManifestImportInput{},
			[]store.SourceManifestFTSInput{{
				FileID: "src_1", Path: "tests/session_test.go", PathTerms: "tests session test go",
				SourceRoot: "tests", Language: "go", SourceRole: "test",
				Symbols: "TestRefreshSession", TestNames: "TestRefreshSession",
			}},
			now)
		require.NoError(t, err)
	}

	candidates, report, err := LoadSourceManifestCandidatesForQuery(
		db,
		store.FilterParams{RepoRoot: tmp},
		"what tests cover refresh session",
		SourceManifestCandidateOptions{Mode: SourceManifestCandidateModeMetadata, Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, report.SelectedCount)
	require.Len(t, candidates, 1)

	got := candidates[0]
	assert.Equal(t, "source_context", got.Kind)
	assert.Equal(t, "test_case", got.Subtype)
	assert.Equal(t, "source_manifest", got.Metadata["retrieval_candidate"])
	assert.Equal(t, "TestRefreshSession", got.Metadata["test_name"])
	assert.True(t, strings.Contains(got.Body, "Test names:"))
	assert.True(t, strings.Contains(got.Body, "TestRefreshSession"))
}
