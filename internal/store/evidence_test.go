package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceStore_ReplaceRepoEvidenceIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "Auth Plan", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "Auth Tests", "draft", "rev_b", now, now))

	concepts := []ConceptInput{{
		ID:                       "concept_auth",
		RepoID:                   "repo_evidence",
		Canonical:                "auth",
		Kind:                     "path_fragment",
		Forms:                    []string{"auth"},
		DocumentFrequency:        2,
		InverseDocumentFrequency: 1.1,
	}}
	mentions := []ConceptMentionInput{
		{ID: "mention_a", ConceptID: "concept_auth", ArtifactID: "ds_A", Field: "title", Weight: 0.8, EvidenceJSON: `{"form":"auth"}`},
		{ID: "mention_b", ConceptID: "concept_auth", ArtifactID: "ds_B", Field: "title", Weight: 0.8, EvidenceJSON: `{"form":"auth"}`},
	}
	edges := []ArtifactEdgeInput{{
		ID:            "edge_auth",
		RepoID:        "repo_evidence",
		SrcArtifactID: "ds_A",
		DstArtifactID: "ds_B",
		EdgeType:      "mentions_same_concept",
		Weight:        0.7,
		Confidence:    0.8,
		EvidenceCount: 1,
		SourceSignal:  "shared_rare_concept",
		Explanation:   "shares rare concept auth",
		MetadataJSON:  `{"concepts":["auth"]}`,
	}}
	seedEvidenceGraph(t, db, now)

	err = db.ReplaceRepoEvidence("repo_evidence", concepts, mentions, edges, now)
	require.NoError(t, err)

	assertTableCount(t, db, "concepts", 1)
	assertTableCount(t, db, "concept_mentions", 2)
	assertTableCount(t, db, "artifact_edges", 1)

	got, err := db.GetArtifactEdges(ArtifactEdgeFilter{RepoID: "repo_evidence", EdgeType: "mentions_same_concept"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.NotEmpty(t, got[0].Explanation)
	assert.NotEmpty(t, got[0].MetadataJSON)
}

func TestEvidenceStore_ReplaceRepoEvidenceWithPhaseTiming(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "Auth Plan", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "Auth Tests", "draft", "rev_b", now, now))

	phaseMS, err := db.ReplaceRepoEvidenceWithPhaseTiming("repo_evidence",
		[]ConceptInput{{ID: "concept_auth", RepoID: "repo_evidence", Canonical: "auth", Kind: "path_fragment", Forms: []string{"auth"}, DocumentFrequency: 2}},
		[]ConceptMentionInput{{ID: "mention_auth", ConceptID: "concept_auth", ArtifactID: "ds_A", Field: "title", Weight: 0.8}},
		[]ArtifactEdgeInput{{ID: "edge_auth", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "mentions_same_concept", Weight: 0.7, Confidence: 0.8, SourceSignal: "shared_rare_concept"}},
		now,
	)
	require.NoError(t, err)

	require.Len(t, phaseMS, 8)
	assert.Contains(t, phaseMS, "persist_delete_edges")
	assert.Contains(t, phaseMS, "persist_concepts")
	assert.Contains(t, phaseMS, "persist_mentions")
	assert.Contains(t, phaseMS, "persist_edges")
}

func TestEvidenceStore_ReplaceRepoEvidenceBatchesManyMentions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "Auth Plan", "draft", "rev_a", now, now))

	concepts := []ConceptInput{{
		ID:                "concept_auth",
		RepoID:            "repo_evidence",
		Canonical:         "auth",
		Kind:              "path_fragment",
		Forms:             []string{"auth"},
		DocumentFrequency: conceptMentionIndexRebuildThreshold + 3,
	}}
	mentions := make([]ConceptMentionInput, 0, conceptMentionIndexRebuildThreshold+3)
	for i := 0; i < conceptMentionIndexRebuildThreshold+3; i++ {
		evidenceJSON := ""
		if i%2 == 1 {
			evidenceJSON = fmt.Sprintf(`{"ordinal":%d}`, i)
		}
		mentions = append(mentions, ConceptMentionInput{
			ID:           fmt.Sprintf("mention_batch_%03d", i),
			ConceptID:    "concept_auth",
			ArtifactID:   "ds_A",
			SectionID:    fmt.Sprintf("section_%03d", i),
			Field:        "title",
			Weight:       0.8,
			EvidenceJSON: evidenceJSON,
		})
	}
	seedBatchedEvidenceState(t, db, now)

	err = db.ReplaceRepoEvidence("repo_evidence", concepts, mentions, nil, now)
	require.NoError(t, err)

	assertTableCount(t, db, "concepts", 1)
	assertTableCount(t, db, "concept_mentions", len(mentions))
	assertTableCount(t, db, "artifact_edges", 0)

	var defaultEvidence, explicitEvidence string
	require.NoError(t, db.QueryRow("SELECT evidence_json FROM concept_mentions WHERE id = ?", "mention_batch_000").Scan(&defaultEvidence))
	require.NoError(t, db.QueryRow("SELECT evidence_json FROM concept_mentions WHERE id = ?", "mention_batch_001").Scan(&explicitEvidence))
	assert.Equal(t, "{}", defaultEvidence)
	assert.Equal(t, `{"ordinal":1}`, explicitEvidence)
}

func TestEvidenceStore_DeleteArtifactEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "A", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "B", "draft", "rev_b", now, now))
	mustNoErr(t, db.UpsertConcept(ConceptInput{ID: "concept_x", RepoID: "repo_evidence", Canonical: "x", Kind: "term"}, now))
	mustNoErr(t, db.ReplaceConceptMentions("ds_A", []ConceptMentionInput{{ID: "mention_x", ConceptID: "concept_x", ArtifactID: "ds_A", Field: "title", Weight: 1}}, now))
	mustNoErr(t, db.UpsertArtifactEdge(ArtifactEdgeInput{ID: "edge_x", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "explicit_reference", Weight: 1, Confidence: 1, SourceSignal: "path_reference"}, now))
	err = db.DeleteArtifactEvidence("ds_A")
	require.NoError(t, err)

	assertTableCount(t, db, "concept_mentions", 0)
	assertTableCount(t, db, "artifact_edges", 0)
	assertTableCount(t, db, "concepts", 1)
}

func TestEvidenceStore_ReplaceRepoEvidenceScopePreservesOtherEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "A", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "B", "draft", "rev_b", now, now))
	seedBaseScopedEvidence(t, db, now)
	seedScopedEvidenceGraph(t, db, now)
	scopeConcepts := []ConceptInput{{ID: "concept_ws", RepoID: "repo_evidence", Canonical: "DEV-123", Kind: "workstream_anchor", Forms: []string{"DEV-123"}, DocumentFrequency: 2}}
	scopeMentions := []ConceptMentionInput{{ID: "mention_ws", ConceptID: "concept_ws", ArtifactID: "ds_A", Field: "workstream_anchor", Weight: 0.9}}
	scopeEdges := []ArtifactEdgeInput{{ID: "edge_ws", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "same_workstream_anchor", Weight: 0.8, Confidence: 0.9, SourceSignal: "workstream_anchor"}}

	err = db.ReplaceRepoEvidenceScope("repo_evidence", "workstream_anchor", "same_workstream_anchor", scopeConcepts, scopeMentions, scopeEdges, now)
	require.NoError(t, err)

	assertTableCount(t, db, "concepts", 2)
	assertTableCount(t, db, "concept_mentions", 2)
	assertTableCount(t, db, "artifact_edges", 2)
	got, err := db.GetArtifactEdges(ArtifactEdgeFilter{RepoID: "repo_evidence", EdgeType: "mentions_same_concept"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "edge_auth", got[0].ID)
}

func insertEvidenceRepo(db *DB, repoID, now string) error {
	_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, "/tmp/"+repoID, now, now)
	return err
}

func assertTableCount(t *testing.T, db *DB, table string, want int) {
	t.Helper()

	var got int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&got))
	assert.Equal(t, want, got, "%s count", table)
}

func seedEvidenceGraph(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO concepts (
		id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "concept_auth", "repo_evidence", "auth", "path_fragment", `["auth"]`, 2, 1.1, now, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO concept_mentions (
		id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "mention_a", "concept_auth", "ds_A", "", "title", 0.8, `{"form":"auth"}`, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO concept_mentions (
		id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "mention_b", "concept_auth", "ds_B", "", "title", 0.8, `{"form":"auth"}`, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO artifact_edges (
		id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence,
		evidence_count, freshness, source_signal, explanation, metadata_json, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"edge_auth", "repo_evidence", "ds_A", "ds_B", "mentions_same_concept", 0.7, 0.8,
		1, "", "shared_rare_concept", "old explanation", `{}`, now, now)
	require.NoError(t, err)
}

func seedScopedEvidenceGraph(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO concepts (
		id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "concept_ws", "repo_evidence", "DEV-123", "workstream_anchor", `["DEV-123"]`, 2, 0, now, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO concept_mentions (
		id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "mention_ws", "concept_ws", "ds_A", "", "workstream_anchor", 0.9, `{}`, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO artifact_edges (
		id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence,
		evidence_count, freshness, source_signal, explanation, metadata_json, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"edge_ws", "repo_evidence", "ds_A", "ds_B", "same_workstream_anchor", 0.8, 0.9,
		1, "", "workstream_anchor", "", `{}`, now, now)
	require.NoError(t, err)

}

func seedBatchedEvidenceState(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO concepts (
		id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "concept_auth", "repo_evidence", "auth", "path_fragment", `["auth"]`, 1, 0, now, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO concept_mentions (
		id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "mention_batch_000", "concept_auth", "ds_A", "section_000", "title", 0.8, `{}`, now)
	require.NoError(t, err)
}

func seedBaseScopedEvidence(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO concepts (
		id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "concept_auth", "repo_evidence", "auth", "term", `["auth"]`, 2, 0, now, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO concept_mentions (
		id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "mention_auth", "concept_auth", "ds_A", "", "title", 0.8, `{}`, now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO artifact_edges (
		id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence,
		evidence_count, freshness, source_signal, explanation, metadata_json, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"edge_auth", "repo_evidence", "ds_A", "ds_B", "mentions_same_concept", 0.7, 0.8,
		1, "", "shared_rare_concept", "", `{}`, now, now)
	require.NoError(t, err)
}
