package indexquery

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestLoadCandidatesAttachesPersistedSections(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_sec"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_sec", repoID, "plan", "", "Billing Plan", "active", "rev_sec", now, now)
		require.NoError(t, err)
	}

	body := "# Billing Plan\n\n## Replay Boundary\n\nstripe_event_id idempotency matters."
	{
		err := db.InsertRevisionDirect("rev_sec", "ds_sec", "sha256:test", body, "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_sec", "ds_sec", repoID, "markdown", "docs/plans/billing.md", "docs/plans/billing.md|markdown", "", "", now)
		require.NoError(t, err)
	}

	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown(body), "ds_sec", "rev_sec", "docs/plans/billing.md")
	{
		err := db.ReplaceArtifactSections("ds_sec", "rev_sec", sections, now)
		require.NoError(t, err)
	}

	candidates, err := LoadCandidates(db, store.FilterParams{RepoRoot: tmp})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.NotEmpty(t, candidates[0].Sections)
	assert.Equal(t, "docs/plans/billing.md", candidates[0].Sections[0].SourcePath,
		"unexpected section source path: %#v", candidates[0].Sections[0])

}

func TestLoadCandidatesByArtifactIDsHydratesSelectedCandidate(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_batch"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_a", repoID, "plan", "", "Alpha Plan", "active", "rev_a", now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect("rev_a", "ds_a", "sha256:a", "# Alpha\n\n## Replay\n\nstripe_event_id idempotency.", `{"mode":"test"}`, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_a", "ds_a", repoID, "markdown", "docs/alpha.md", "docs/alpha.md|markdown", "", "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertLink("link_a", "ds_a", "implements", "ds_target", now)
		require.NoError(t, err)
	}
	{

		_, err := db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('todo_a', 'ds_a', 'rev_a', 0, 'Preserve todo', 0, 'docs/alpha.md', 3, ?)", now)
		require.NoError(t, err)
	}

	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Alpha\n\n## Replay\n\nstripe_event_id idempotency."), "ds_a", "rev_a", "docs/alpha.md")
	{
		err := db.ReplaceArtifactSections("ds_a", "rev_a", sections, now)
		require.NoError(t, err)
	}

	selected, err := LoadCandidatesByArtifactIDs(db, store.FilterParams{RepoRoot: tmp}, []string{"ds_a"})

	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, "ds_a", selected[0].ID)
	assert.Equal(t, "docs/alpha.md", selected[0].Path)
	assert.Equal(t, "plan", selected[0].Kind)
	assert.Equal(t, "Alpha Plan", selected[0].Title)
	assert.Equal(t, "active", selected[0].Status)
	assert.Contains(t, selected[0].Body, "stripe_event_id idempotency")
	require.NotEmpty(t, selected[0].Sections)
	assert.Equal(t, "docs/alpha.md", selected[0].Sections[0].SourcePath)
}

func TestPreselectArtifactIDsForQueryUsesFTSSectionAndPathLanes(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_preselect"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_body", repoID, "plan", "", "Runtime Plan", "active", "rev_body", now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect("rev_body", "ds_body", "sha256:body", "FluxNova requirements live here.", "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_body", "ds_body", repoID, "markdown", "docs/runtime.md", "docs/runtime.md|markdown", "", "", now)
		require.NoError(t, err)
	}
	{

		err := db.IndexArtifactFTS("ds_body", "Runtime Plan", "FluxNova requirements live here.", "docs/runtime.md")
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_section", repoID, "plan", "", "Guard Plan", "active", "rev_section", now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect("rev_section", "ds_section", "sha256:section", "# Guard\n\n## CALM Guard\n\nEvidence details.", "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_section", "ds_section", repoID, "markdown", "docs/guard.md", "docs/guard.md|markdown", "", "", now)
		require.NoError(t, err)
	}

	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Guard\n\n## CALM Guard\n\nEvidence details."), "ds_section", "rev_section", "docs/guard.md")
	{
		err := db.ReplaceArtifactSections("ds_section", "rev_section", sections, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_path", repoID, "plan", "", "Path Plan", "active", "rev_path", now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect("rev_path", "ds_path", "sha256:path", "Path-only body.", "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_path", "ds_path", repoID, "markdown", "docs/fluxnova-path.md", "docs/fluxnova-path.md|markdown", "", "", now)
		require.NoError(t, err)
	}

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: tmp}, "FluxNova CALM Guard", PreselectOptions{
		PreselectLimit:              10,
		MaxRepoSizeForFullHydration: 0,
		FallbackFullHydrationBelow:  1,
	})
	require.NoError(t, err)
	require.Equal(t, "", report.FallbackReason,
		"unexpected fallback: %#v", report)

	assert.Contains(t, ids, "ds_body")
	assert.Contains(t, ids, "ds_section")
	assert.Contains(t, ids, "ds_path")
	assert.NotZero(t, report.LaneCounts["artifact_fts"])
	assert.NotZero(t, report.LaneCounts["section_fts"])
	assert.NotZero(t, report.LaneCounts["title_path_like"])
}

func TestParseRuntimeModeDefaultsToPreselectActive(t *testing.T) {
	mode, err := ParseRuntimeMode("")

	require.NoError(t, err)
	assert.Equal(t, RuntimeModePreselectActive, mode)
}

func TestParseRuntimeMode_WithFullMode_ReturnsFull(t *testing.T) {
	mode, err := ParseRuntimeMode("full")

	require.NoError(t, err)
	assert.Equal(t, RuntimeModeFull, mode)
}
