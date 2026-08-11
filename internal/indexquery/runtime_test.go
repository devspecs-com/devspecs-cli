package indexquery

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRuntimeMode_WithShadowMode_ReturnsShadow(t *testing.T) {
	mode, err := ParseRuntimeMode(" PRESELECT_SHADOW ")

	require.NoError(t, err)
	assert.Equal(t, RuntimeModePreselectShadow, mode)
}

func TestParseRuntimeMode_WithActiveMode_ReturnsActive(t *testing.T) {
	mode, err := ParseRuntimeMode("preselect_active")

	require.NoError(t, err)
	assert.Equal(t, RuntimeModePreselectActive, mode)
}

func TestParseRuntimeMode_WithUnknownMode_ReturnsActionableError(t *testing.T) {
	mode, err := ParseRuntimeMode("turbo")

	require.Error(t, err)
	assert.Empty(t, mode)
	assert.ErrorContains(t, err, `unknown find runtime "turbo"`)
	assert.ErrorContains(t, err, "full, preselect_shadow, preselect_active")
}

func TestDefaultPreselectOptions_ReturnsBoundedProductionDefaults(t *testing.T) {
	options := DefaultPreselectOptions()

	assert.Equal(t, 1500, options.PreselectLimit)
	assert.Equal(t, 500, options.MaxRepoSizeForFullHydration)
	assert.Equal(t, 1, options.FallbackFullHydrationBelow)
}

func TestLoadCandidatesForQueryWithRuntime_WithFullMode_LoadsEntireCorpus(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	result, err := LoadCandidatesForQueryWithRuntime(db, store.FilterParams{RepoRoot: repoRoot}, "runtime needle", RuntimeModeFull)

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	assert.Equal(t, "artifact-000", result.Candidates[0].ID)
	assert.Equal(t, string(RuntimeModeFull), result.Report.RuntimeMode)
	assert.Equal(t, string(RuntimeModeFull), result.Report.EffectiveMode)
	assert.Equal(t, 1, result.Report.FullArtifactCount)
	assert.Equal(t, 1, result.Report.HydratedCount)
}

func TestLoadCandidatesForQueryWithRuntime_WithDefaultMode_FallsBackForSmallCorpus(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	result, err := LoadCandidatesForQueryWithRuntime(db, store.FilterParams{RepoRoot: repoRoot}, "runtime needle", "")

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	assert.Equal(t, string(RuntimeModePreselectActive), result.Report.RuntimeMode)
	assert.Equal(t, string(RuntimeModeFull), result.Report.EffectiveMode)
	assert.Equal(t, "small_corpus", result.Report.FallbackReason)
	assert.Equal(t, 1, result.Report.FullArtifactCount)
}

func TestLoadCandidatesForQueryWithRuntime_WithShadowMode_ReturnsFullSmallCorpusResult(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	result, err := LoadCandidatesForQueryWithRuntime(db, store.FilterParams{RepoRoot: repoRoot}, "runtime needle", RuntimeModePreselectShadow)

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	assert.Equal(t, string(RuntimeModePreselectShadow), result.Report.RuntimeMode)
	assert.Equal(t, string(RuntimeModeFull), result.Report.EffectiveMode)
	assert.Equal(t, "small_corpus", result.Report.FallbackReason)
}

func TestLoadCandidatesForQueryWithRuntime_WithActiveMode_HydratesOnlyPreselectedCandidates(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 501)

	result, err := LoadCandidatesForQueryWithRuntime(db, store.FilterParams{RepoRoot: repoRoot}, "runtime needle", RuntimeModePreselectActive)

	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	assert.Equal(t, "artifact-000", result.Candidates[0].ID)
	assert.Equal(t, string(RuntimeModePreselectActive), result.Report.RuntimeMode)
	assert.Equal(t, string(RuntimeModePreselectActive), result.Report.EffectiveMode)
	assert.Equal(t, 501, result.Report.FullArtifactCount)
	assert.Equal(t, 1, result.Report.PreselectedCount)
	assert.Equal(t, 1, result.Report.HydratedCount)
	assert.Empty(t, result.Report.FallbackReason)
	assert.NotZero(t, result.Report.LaneCounts["artifact_fts"])
}

func TestLoadCandidatesForQueryWithRuntime_WithUnknownMode_ReturnsError(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	result, err := LoadCandidatesForQueryWithRuntime(db, store.FilterParams{RepoRoot: repoRoot}, "runtime", RuntimeMode("unknown"))

	require.Error(t, err)
	assert.Empty(t, result.Candidates)
	assert.ErrorContains(t, err, `unknown find runtime "unknown"`)
}

func TestLoadCandidatesForQueryOptimized_WithClosedDatabase_ReturnsPreselectError(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	result, err := LoadCandidatesForQueryOptimized(db, store.FilterParams{}, "runtime", RuntimeModePreselectActive)

	require.Error(t, err)
	assert.Empty(t, result.Candidates)
	assert.ErrorContains(t, err, "preselect:")
}

func TestPreselectArtifactIDsForQuery_WithEmptyCorpus_ReturnsFallback(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 0)

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: repoRoot}, "runtime", PreselectOptions{})

	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Equal(t, 0, report.FullArtifactCount)
	assert.Equal(t, "empty_corpus", report.FallbackReason)
	assert.Empty(t, report.LaneCounts)
}

func TestPreselectArtifactIDsForQuery_WithSmallCorpus_ReturnsFallback(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: repoRoot}, "runtime", DefaultPreselectOptions())

	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Equal(t, 1, report.FullArtifactCount)
	assert.Equal(t, "small_corpus", report.FallbackReason)
}

func TestPreselectArtifactIDsForQuery_WithOnlyStopWords_ReturnsNoTermsFallback(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: repoRoot}, "the and context", PreselectOptions{
		PreselectLimit:              10,
		MaxRepoSizeForFullHydration: 0,
		FallbackFullHydrationBelow:  1,
	})

	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Equal(t, "no_query_terms", report.FallbackReason)
}

func TestPreselectArtifactIDsForQuery_WithInsufficientMatches_ReturnsCandidatePoolFallback(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: repoRoot}, "missing phrase", PreselectOptions{
		PreselectLimit:              10,
		MaxRepoSizeForFullHydration: 0,
		FallbackFullHydrationBelow:  2,
	})

	require.NoError(t, err)
	assert.Empty(t, ids)
	assert.Equal(t, "candidate_pool_too_small", report.FallbackReason)
	assert.Zero(t, report.SelectedCount)
}

func TestLoadCandidatesForQuery_EnrichesMatchingSections(t *testing.T) {
	db, repoRoot := setupRuntimeCandidateDB(t, 1)

	candidates, err := LoadCandidatesForQuery(db, store.FilterParams{RepoRoot: repoRoot}, "runtime needle")

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "artifact-000", candidates[0].ID)
	assert.Contains(t, candidates[0].Body, "Runtime Needle")
}

func TestArtifactCandidate_WithSourceAndTodo_RendersCandidate(t *testing.T) {
	artifact := store.ArtifactRow{ID: "artifact-1", RepoID: "repo-1", Kind: "plan", Title: "Plan", Status: "active"}
	sources := []store.SourceRow{{SourceType: "markdown", Path: `docs\plan.md`}}
	todos := []store.TodoRow{{Text: "Ship it", Done: true}}

	candidate := ArtifactCandidate(artifact, sources, todos, "Body", "")

	assert.Equal(t, "artifact-1", candidate.ID)
	assert.Equal(t, "docs/plan.md", candidate.Path)
	assert.Equal(t, "docs/plan.md", candidate.Source)
	assert.Contains(t, candidate.Body, "- [x] Ship it")
	assert.Contains(t, candidate.Body, "Body")
}

func TestApproximateTokenCount_WithPartialToken_RoundsUp(t *testing.T) {
	count := ApproximateTokenCount("12345")

	assert.Equal(t, 2, count)
}

func TestApproximateTokenCount_WithEmptyText_ReturnsZero(t *testing.T) {
	count := ApproximateTokenCount("")

	assert.Zero(t, count)
}

func TestRetrievalRuntimeTerms_WithCompoundQuery_ReturnsDistinctUsefulTerms(t *testing.T) {
	terms := retrievalRuntimeTerms("The Payment_Retry.payment-retry and API")

	require.Len(t, terms, 4)
	assert.Equal(t, "payment_retry.payment-retry", terms[0])
	assert.Equal(t, "payment", terms[1])
	assert.Equal(t, "retry", terms[2])
	assert.Equal(t, "api", terms[3])
}

func TestRetrievalRuntimeTerms_WithMoreThanTwelveTerms_TruncatesResult(t *testing.T) {
	terms := retrievalRuntimeTerms("one two three four five six seven eight nine ten eleven twelve thirteen fourteen")

	require.Len(t, terms, 12)
	assert.Equal(t, "one", terms[0])
	assert.Equal(t, "twelve", terms[11])
}

func TestRetrievalRuntimeFTSQuery_WithQuote_EscapesTerm(t *testing.T) {
	query := retrievalRuntimeFTSQuery([]string{"alpha", `a"b`})

	assert.Equal(t, `"alpha" OR "a""b"`, query)
}

func setupRuntimeCandidateDB(t *testing.T, artifactCount int) (*store.DB, string) {
	t.Helper()
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, db.Close())
	})
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo-runtime", repoRoot, now, now)
	require.NoError(t, err)
	for i := 0; i < artifactCount; i++ {
		artifactID := fmt.Sprintf("artifact-%03d", i)
		title := fmt.Sprintf("Filler Artifact %03d", i)
		revisionID := ""
		if i == 0 {
			title = "Runtime Needle"
			revisionID = "revision-000"
		}
		require.NoError(t, db.InsertArtifactDirect(artifactID, "repo-runtime", "plan", "", title, "active", revisionID, now, now))
	}
	if artifactCount > 0 {
		require.NoError(t, db.InsertRevisionDirect("revision-000", "artifact-000", "sha256:runtime", "Runtime needle body.", "", now))
		require.NoError(t, db.InsertSourceDirect("source-000", "artifact-000", "repo-runtime", "markdown", "docs/runtime.md", "docs/runtime.md|markdown", "", "", now))
		require.NoError(t, db.IndexArtifactFTS("artifact-000", "Runtime Needle", "Runtime needle body.", "docs/runtime.md"))
	}
	return db, repoRoot
}
