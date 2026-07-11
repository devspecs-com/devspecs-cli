package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestEvidenceStore_ReplaceRepoEvidenceIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
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
	for i := 0; i < 2; i++ {
		if err := db.ReplaceRepoEvidence("repo_evidence", concepts, mentions, edges, now); err != nil {
			t.Fatal(err)
		}
	}
	assertTableCount(t, db, "concepts", 1)
	assertTableCount(t, db, "concept_mentions", 2)
	assertTableCount(t, db, "artifact_edges", 1)

	got, err := db.GetArtifactEdges(ArtifactEdgeFilter{RepoID: "repo_evidence", EdgeType: "mentions_same_concept"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one edge, got %d", len(got))
	}
	if got[0].Explanation == "" || got[0].MetadataJSON == "" {
		t.Fatalf("edge should preserve explanation and metadata: %#v", got[0])
	}
}

func TestEvidenceStore_ReplaceRepoEvidenceWithPhaseTiming(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"persist_delete_edges", "persist_concepts", "persist_mentions", "persist_edges"} {
		if _, ok := phaseMS[phase]; !ok {
			t.Fatalf("missing phase %q in %#v", phase, phaseMS)
		}
	}
}

func TestEvidenceStore_ReplaceRepoEvidenceBatchesManyMentions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
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
	for i := 0; i < 2; i++ {
		if err := db.ReplaceRepoEvidence("repo_evidence", concepts, mentions, nil, now); err != nil {
			t.Fatal(err)
		}
	}
	assertTableCount(t, db, "concepts", 1)
	assertTableCount(t, db, "concept_mentions", len(mentions))
	assertTableCount(t, db, "artifact_edges", 0)

	var defaultEvidence, explicitEvidence string
	if err := db.QueryRow("SELECT evidence_json FROM concept_mentions WHERE id = ?", "mention_batch_000").Scan(&defaultEvidence); err != nil {
		t.Fatal(err)
	}
	if defaultEvidence != "{}" {
		t.Fatalf("default evidence_json = %q, want {}", defaultEvidence)
	}
	if err := db.QueryRow("SELECT evidence_json FROM concept_mentions WHERE id = ?", "mention_batch_001").Scan(&explicitEvidence); err != nil {
		t.Fatal(err)
	}
	if explicitEvidence != `{"ordinal":1}` {
		t.Fatalf("explicit evidence_json = %q, want ordinal payload", explicitEvidence)
	}
}

func TestEvidenceStore_DeleteArtifactEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "A", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "B", "draft", "rev_b", now, now))
	mustNoErr(t, db.UpsertConcept(ConceptInput{ID: "concept_x", RepoID: "repo_evidence", Canonical: "x", Kind: "term"}, now))
	mustNoErr(t, db.ReplaceConceptMentions("ds_A", []ConceptMentionInput{{ID: "mention_x", ConceptID: "concept_x", ArtifactID: "ds_A", Field: "title", Weight: 1}}, now))
	mustNoErr(t, db.UpsertArtifactEdge(ArtifactEdgeInput{ID: "edge_x", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "explicit_reference", Weight: 1, Confidence: 1, SourceSignal: "path_reference"}, now))

	if err := db.DeleteArtifactEvidence("ds_A"); err != nil {
		t.Fatal(err)
	}
	assertTableCount(t, db, "concept_mentions", 0)
	assertTableCount(t, db, "artifact_edges", 0)
	assertTableCount(t, db, "concepts", 1)
}

func TestEvidenceStore_ReplaceRepoEvidenceScopePreservesOtherEvidence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_evidence", now))
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_evidence", "plan", "", "A", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_evidence", "plan", "", "B", "draft", "rev_b", now, now))
	mustNoErr(t, db.ReplaceRepoEvidence("repo_evidence",
		[]ConceptInput{{ID: "concept_auth", RepoID: "repo_evidence", Canonical: "auth", Kind: "term", Forms: []string{"auth"}, DocumentFrequency: 2}},
		[]ConceptMentionInput{{ID: "mention_auth", ConceptID: "concept_auth", ArtifactID: "ds_A", Field: "title", Weight: 0.8}},
		[]ArtifactEdgeInput{{ID: "edge_auth", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "mentions_same_concept", Weight: 0.7, Confidence: 0.8, SourceSignal: "shared_rare_concept"}},
		now,
	))
	scopeConcepts := []ConceptInput{{ID: "concept_ws", RepoID: "repo_evidence", Canonical: "DEV-123", Kind: "workstream_anchor", Forms: []string{"DEV-123"}, DocumentFrequency: 2}}
	scopeMentions := []ConceptMentionInput{{ID: "mention_ws", ConceptID: "concept_ws", ArtifactID: "ds_A", Field: "workstream_anchor", Weight: 0.9}}
	scopeEdges := []ArtifactEdgeInput{{ID: "edge_ws", RepoID: "repo_evidence", SrcArtifactID: "ds_A", DstArtifactID: "ds_B", EdgeType: "same_workstream_anchor", Weight: 0.8, Confidence: 0.9, SourceSignal: "workstream_anchor"}}
	for i := 0; i < 2; i++ {
		mustNoErr(t, db.ReplaceRepoEvidenceScope("repo_evidence", "workstream_anchor", "same_workstream_anchor", scopeConcepts, scopeMentions, scopeEdges, now))
	}

	assertTableCount(t, db, "concepts", 2)
	assertTableCount(t, db, "concept_mentions", 2)
	assertTableCount(t, db, "artifact_edges", 2)
	got, err := db.GetArtifactEdges(ArtifactEdgeFilter{RepoID: "repo_evidence", EdgeType: "mentions_same_concept"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("base evidence edge should be preserved, got %d", len(got))
	}
}

func insertEvidenceRepo(db *DB, repoID, now string) error {
	_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, "/tmp/"+repoID, now, now)
	return err
}

func assertTableCount(t *testing.T, db *DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
