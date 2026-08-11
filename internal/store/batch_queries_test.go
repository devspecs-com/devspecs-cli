package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
)

func TestCountArtifacts_WithRepositoryFilter_CountsMatchingArtifacts(t *testing.T) {
	db := openTestDB(t)
	rootA, _ := seedArtifactQueryRows(t, db)

	count, err := db.CountArtifacts(FilterParams{RepoRoot: rootA})
	require.NoError(t, err)

	assert.Equal(t, 2, count)
}

func TestListArtifactsByIDs_WithRepositoryFilter_PreservesUniqueInputOrder(t *testing.T) {
	db := openTestDB(t)
	rootA, _ := seedArtifactQueryRows(t, db)

	rows, err := db.ListArtifactsByIDs([]string{"ds_C", "ds_A", "ds_B", "ds_A"}, FilterParams{RepoRoot: rootA})
	require.NoError(t, err)

	require.Len(t, rows, 2)
	assert.Equal(t, "ds_C", rows[0].ID)
	assert.Equal(t, "ds_A", rows[1].ID)
}

func TestGetSourcesForArtifacts_WithTwoSources_GroupsRowsByArtifact(t *testing.T) {
	db := openTestDB(t)
	seedBatchHydrationRows(t, db)

	sources, err := db.GetSourcesForArtifacts([]string{"ds_BATCH"})
	require.NoError(t, err)

	require.Len(t, sources, 1)
	require.Len(t, sources["ds_BATCH"], 2)
	assert.Equal(t, "src_batch_a", sources["ds_BATCH"][0].ID)
	assert.Equal(t, "src_batch_b", sources["ds_BATCH"][1].ID)
}

func TestGetLinksForArtifacts_WithOneLink_GroupsRowsByArtifact(t *testing.T) {
	db := openTestDB(t)
	seedBatchHydrationRows(t, db)

	links, err := db.GetLinksForArtifacts([]string{"ds_BATCH"})
	require.NoError(t, err)

	require.Len(t, links, 1)
	require.Len(t, links["ds_BATCH"], 1)
	assert.Equal(t, "link_batch", links["ds_BATCH"][0].ID)
}

func TestGetTodosForArtifacts_WithOneTodo_GroupsRowsByArtifact(t *testing.T) {
	db := openTestDB(t)
	seedBatchHydrationRows(t, db)

	todos, err := db.GetTodosForArtifacts([]string{"ds_BATCH"})
	require.NoError(t, err)

	require.Len(t, todos, 1)
	require.Len(t, todos["ds_BATCH"], 1)
	assert.Equal(t, "todo_batch", todos["ds_BATCH"][0].ID)
}

func TestGetSectionsForArtifacts_WithMarkdownSections_GroupsRowsByArtifact(t *testing.T) {
	db := openTestDB(t)
	seedBatchHydrationRows(t, db)

	sections, err := db.GetSectionsForArtifacts([]string{"ds_BATCH"})
	require.NoError(t, err)

	require.Len(t, sections, 1)
	assert.NotEmpty(t, sections["ds_BATCH"])
}

func TestGetRevisionsByIDs_WithExtractedJSON_ReturnsPayload(t *testing.T) {
	db := openTestDB(t)
	seedBatchHydrationRows(t, db)

	revisions, err := db.GetRevisionsByIDs([]string{"rev_batch"})
	require.NoError(t, err)

	require.Len(t, revisions, 1)
	revision, ok := revisions["rev_batch"]
	require.True(t, ok)
	assert.Equal(t, `{"mode":"test"}`, revision.ExtractedJSON)
}

func TestFindArtifactIDsFTS_WithMatchingTerm_ReturnsArtifactID(t *testing.T) {
	db := openTestDB(t)
	root := seedArtifactPreselectionRows(t, db)

	ids, err := db.FindArtifactIDsFTS(`"fluxnova"`, FilterParams{RepoRoot: root}, 10)
	require.NoError(t, err)

	require.Len(t, ids, 1)
	assert.Equal(t, "ds_FLUX", ids[0])
}

func TestFindArtifactIDsBySectionFTS_WithMatchingTerm_ReturnsArtifactID(t *testing.T) {
	db := openTestDB(t)
	root := seedArtifactPreselectionRows(t, db)

	ids, err := db.FindArtifactIDsBySectionFTS(`"guard"`, FilterParams{RepoRoot: root}, 10)
	require.NoError(t, err)

	require.Len(t, ids, 1)
	assert.Equal(t, "ds_FLUX", ids[0])
}

func TestFindArtifactIDsByTitleOrPathTerms_WithMatchingTerm_ReturnsArtifactID(t *testing.T) {
	db := openTestDB(t)
	root := seedArtifactPreselectionRows(t, db)

	ids, err := db.FindArtifactIDsByTitleOrPathTerms([]string{"fluxnova"}, FilterParams{RepoRoot: root}, 10)
	require.NoError(t, err)

	require.Len(t, ids, 1)
	assert.Equal(t, "ds_FLUX", ids[0])
}

func seedArtifactQueryRows(t *testing.T, db *DB) (string, string) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	rootA := t.TempDir()
	rootB := t.TempDir()
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_a', ?, ?, ?), ('repo_b', ?, ?, ?)", rootA, now, now, rootB, now, now)
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_a", "plan", "", "Alpha", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_b", "plan", "", "Beta", "draft", "rev_b", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_C", "repo_a", "spec", "", "Gamma", "draft", "rev_c", now, now))
	return rootA, rootB
}

func seedBatchHydrationRows(t *testing.T, db *DB) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_batch', '/tmp/repo-batch', ?, ?)", now, now)
	mustNoErr(t, db.InsertArtifactDirect("ds_BATCH", "repo_batch", "plan", "", "Batch Plan", "draft", "rev_batch", now, now))
	mustNoErr(t, db.InsertRevisionDirect("rev_batch", "ds_BATCH", "sha256:batch", "# Batch Plan\n\n## Notes\n\nHydrate me.", `{"mode":"test"}`, now))
	mustNoErr(t, db.InsertSourceDirect("src_batch_a", "ds_BATCH", "repo_batch", "markdown", "docs/batch.md", "docs/batch.md|markdown", "", "", now))
	mustNoErr(t, db.InsertSourceDirect("src_batch_b", "ds_BATCH", "repo_batch", "test_case", "tests/batch_test.go", "tests/batch_test.go|TestBatch|7", "", "", now))
	mustNoErr(t, db.InsertLink("link_batch", "ds_BATCH", "implements", "ds_TARGET", now))
	mustExecStoreTestSQL(t, db, "INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('todo_batch', 'ds_BATCH', 'rev_batch', 0, 'Check batching', 0, 'docs/batch.md', 4, ?)", now)
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Batch Plan\n\n## Notes\n\nHydrate me."), "ds_BATCH", "rev_batch", "docs/batch.md")
	mustNoErr(t, db.ReplaceArtifactSections("ds_BATCH", "rev_batch", sections, now))
}

func seedArtifactPreselectionRows(t *testing.T, db *DB) string {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	root := t.TempDir()
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_find_ids', ?, ?, ?)", root, now, now)
	mustNoErr(t, db.InsertArtifactDirect("ds_FLUX", "repo_find_ids", "requirement", "", "FluxNova AIGF Requirement", "active", "rev_flux", now, now))
	mustNoErr(t, db.InsertRevisionDirect("rev_flux", "ds_FLUX", "sha256:flux", "# Requirement\n\nFluxNova must support AIGF controls.", "", now))
	mustNoErr(t, db.InsertSourceDirect("src_flux", "ds_FLUX", "repo_find_ids", "markdown", "docs/fluxnova.md", "docs/fluxnova.md|markdown", "", "", now))
	mustNoErr(t, db.IndexArtifactFTS("ds_FLUX", "FluxNova AIGF Requirement", "FluxNova must support AIGF controls.", "docs/fluxnova.md"))
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Requirement\n\n## CALM Guard\n\nFluxNova control evidence."), "ds_FLUX", "rev_flux", "docs/fluxnova.md")
	mustNoErr(t, db.ReplaceArtifactSections("ds_FLUX", "rev_flux", sections, now))
	return root
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()

	require.NoError(t, err)
}
