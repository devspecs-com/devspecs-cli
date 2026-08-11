package store

import (
	"testing"
	"time"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCriteriaForArtifact_ReturnsCriteriaInOrdinalOrder(t *testing.T) {
	db := openTestDB(t)
	_, artifactID, revisionID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	insertCriterion(t, db, "criterion-2", artifactID, revisionID, 2, "Second", true, "success", now)
	insertCriterion(t, db, "criterion-1", artifactID, revisionID, 1, "First", false, "acceptance", now)

	criteria, err := db.GetCriteriaForArtifact(artifactID)

	require.NoError(t, err)
	require.Len(t, criteria, 2)
	assert.Equal(t, "criterion-1", criteria[0].ID)
	assert.Equal(t, "First", criteria[0].Text)
	assert.Equal(t, "acceptance", criteria[0].CriteriaKind)
	assert.False(t, criteria[0].Done)
	assert.Equal(t, "criterion-2", criteria[1].ID)
	assert.Equal(t, "Second", criteria[1].Text)
	assert.Equal(t, "success", criteria[1].CriteriaKind)
	assert.True(t, criteria[1].Done)
}

func TestListAllCriteria_WithRepositoryAndOpenFilters_ReturnsMatchingCriterion(t *testing.T) {
	db := openTestDB(t)
	_, artifactID, revisionID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "UPDATE repos SET git_current_branch = 'main', scanned_by = 'alice' WHERE id = 'repo_001'")
	require.NoError(t, db.InsertTag(artifactID, "release", "manual", now))
	insertCriterion(t, db, "criterion-open", artifactID, revisionID, 0, "Ship safely", false, "acceptance", now)
	insertCriterion(t, db, "criterion-done", artifactID, revisionID, 1, "Already done", true, "acceptance", now)

	criteria, err := db.ListAllCriteria(FilterParams{
		RepoRoot: "/tmp/repo",
		Tag:      "release",
		Branch:   "main",
		User:     "alice",
	}, true, false, "acceptance")

	require.NoError(t, err)
	require.Len(t, criteria, 1)
	assert.Equal(t, "criterion-open", criteria[0].ID)
	assert.Equal(t, "Test Plan", criteria[0].ArtifactTitle)
	assert.Equal(t, "plan", criteria[0].ArtifactKind)
}

func TestListAllCriteria_WithDoneFilter_ReturnsCompletedCriterion(t *testing.T) {
	db := openTestDB(t)
	_, artifactID, revisionID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	insertCriterion(t, db, "criterion-open", artifactID, revisionID, 0, "Open", false, "success", now)
	insertCriterion(t, db, "criterion-done", artifactID, revisionID, 1, "Done", true, "success", now)

	criteria, err := db.ListAllCriteria(FilterParams{}, false, true, "")

	require.NoError(t, err)
	require.Len(t, criteria, 1)
	assert.Equal(t, "criterion-done", criteria[0].ID)
	assert.True(t, criteria[0].Done)
}

func TestGetSectionsForArtifact_ReturnsPersistedSectionMetadata(t *testing.T) {
	db := openTestDB(t)
	_, artifactID, revisionID := seedArtifact(t, db)
	seedStoredSection(t, db, artifactID, revisionID)

	sections, err := db.GetSectionsForArtifact(artifactID)

	require.NoError(t, err)
	require.Len(t, sections, 1)
	assert.Equal(t, "section-replay", sections[0].ID)
	assert.Equal(t, "Plan > Replay Boundary", sections[0].HeadingPath)
	assert.Equal(t, 2, sections[0].HeadingDepth)
	assert.Equal(t, 3, sections[0].StartLine)
	assert.Equal(t, 5, sections[0].EndLine)
	assert.Contains(t, sections[0].MetadataJSON, `"owner":"platform"`)
}

func TestFindArtifactSections_WithArtifactAndRepositoryFilters_ReturnsMatch(t *testing.T) {
	db := openTestDB(t)
	_, artifactID, revisionID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	seedStoredSection(t, db, artifactID, revisionID)
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET subtype = 'implementation' WHERE id = ?", artifactID)
	mustExecStoreTestSQL(t, db, "UPDATE repos SET git_current_branch = 'main', scanned_by = 'alice' WHERE id = 'repo_001'")
	require.NoError(t, db.InsertTag(artifactID, "release", "manual", now))

	sections, err := db.FindArtifactSections("replay boundary", FilterParams{
		RepoRoot: "/tmp/repo",
		Kind:     "plan",
		Subtype:  "implementation",
		Tag:      "release",
		Branch:   "main",
		User:     "alice",
	}, 0)

	require.NoError(t, err)
	require.Len(t, sections, 1)
	assert.Equal(t, "section-replay", sections[0].ID)
	assert.Equal(t, artifactID, sections[0].ArtifactID)
}

func TestFindArtifactSections_WithOnlyStopWords_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)

	sections, err := db.FindArtifactSections("the and context", FilterParams{}, 10)

	require.NoError(t, err)
	assert.Empty(t, sections)
}

func TestSectionFTSQuery_WithCompoundTerms_ReturnsDistinctQuotedTerms(t *testing.T) {
	query := sectionFTSQuery("The Payment_Retry.payment-retry and API")

	assert.Equal(t, `"payment_retry.payment-retry" OR "payment" OR "retry" OR "api"`, query)
}

func TestSectionFTSQuery_WithMoreThanTwelveTerms_TruncatesTerms(t *testing.T) {
	query := sectionFTSQuery("one two three four five six seven eight nine ten eleven twelve thirteen fourteen")

	assert.Equal(t, `"one" OR "two" OR "three" OR "four" OR "five" OR "six" OR "seven" OR "eight" OR "nine" OR "ten" OR "eleven" OR "twelve"`, query)
}

func TestFindSourceByIdentityInRepo_ReturnsRepositoryScopedSource(t *testing.T) {
	db := openTestDB(t)
	repoID, _, _ := seedArtifact(t, db)

	artifactID, err := db.FindSourceByIdentityInRepo(repoID, "plans/test.md|markdown")

	require.NoError(t, err)
	assert.Equal(t, "ds_ARTIFACT001", artifactID)
}

func TestRepoHasArtifacts_WithIndexedRepository_ReturnsTrue(t *testing.T) {
	db := openTestDB(t)
	repoID, _, _ := seedArtifact(t, db)

	hasArtifacts, err := db.RepoHasArtifacts(repoID)

	require.NoError(t, err)
	assert.True(t, hasArtifacts)
}

func TestRepoHasArtifacts_WithEmptyRepository_ReturnsFalse(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_empty', '/tmp/empty', ?, ?)", now, now)

	hasArtifacts, err := db.RepoHasArtifacts("repo_empty")

	require.NoError(t, err)
	assert.False(t, hasArtifacts)
}

func TestGetArtifactSourcePaths_ReturnsSourceMappingEvidence(t *testing.T) {
	db := openTestDB(t)
	repoID, _, _ := seedArtifact(t, db)

	paths, err := db.GetArtifactSourcePaths(repoID)

	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, "ds_ARTIFACT001", paths[0].ArtifactID)
	assert.Equal(t, "plan", paths[0].Kind)
	assert.Equal(t, "Test Plan", paths[0].Title)
	assert.Equal(t, "plans/test.md", paths[0].Path)
	assert.Equal(t, "plans/test.md|markdown", paths[0].SourceIdentity)
}

func TestDeleteRepoEvidence_RemovesGraphEvidenceForOnlySelectedRepository(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-11T00:00:00Z"
	require.NoError(t, insertEvidenceRepo(db, "repo_evidence", now))
	require.NoError(t, db.InsertArtifactDirect("artifact-a", "repo_evidence", "plan", "", "A", "draft", "", now, now))
	require.NoError(t, db.InsertArtifactDirect("artifact-b", "repo_evidence", "plan", "", "B", "draft", "", now, now))
	require.NoError(t, db.UpsertConcept(ConceptInput{ID: "concept-a", RepoID: "repo_evidence", Canonical: "auth", Kind: "term"}, now))
	require.NoError(t, db.ReplaceConceptMentions("artifact-a", []ConceptMentionInput{{
		ID: "mention-a", ConceptID: "concept-a", ArtifactID: "artifact-a", Field: "title", Weight: 1,
	}}, now))
	require.NoError(t, db.UpsertArtifactEdge(ArtifactEdgeInput{
		ID: "edge-a", RepoID: "repo_evidence", SrcArtifactID: "artifact-a", DstArtifactID: "artifact-b",
		EdgeType: "explicit_reference", Weight: 1, Confidence: 1, SourceSignal: "path_reference",
	}, now))

	err := db.DeleteRepoEvidence("repo_evidence")

	require.NoError(t, err)
	assertTableCount(t, db, "concepts", 0)
	assertTableCount(t, db, "concept_mentions", 0)
	assertTableCount(t, db, "artifact_edges", 0)
}

func insertCriterion(t *testing.T, db *DB, id, artifactID, revisionID string, ordinal int, text string, done bool, kind, now string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO artifact_criteria
		(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'plans/test.md', 3, ?, ?)`,
		id, artifactID, revisionID, ordinal, text, done, kind, now)
	require.NoError(t, err)
}

func seedStoredSection(t *testing.T, db *DB, artifactID, revisionID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	require.NoError(t, db.ReplaceArtifactSections(artifactID, revisionID, []docsections.Section{{
		ID:            "section-replay",
		SourcePath:    "plans/test.md",
		HeadingPath:   "Plan > Replay Boundary",
		HeadingDepth:  2,
		StartLine:     3,
		EndLine:       5,
		Title:         "Replay Boundary",
		Body:          "Replay boundary uses idempotency keys.",
		TokenEstimate: 9,
		Kind:          "acceptance",
		Metadata:      map[string]string{"owner": "platform"},
	}}, now))
}
