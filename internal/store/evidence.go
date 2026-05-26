package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConceptInput is a durable repo-local concept row.
type ConceptInput struct {
	ID                       string
	RepoID                   string
	Canonical                string
	Kind                     string
	Forms                    []string
	DocumentFrequency        int
	InverseDocumentFrequency float64
}

// ConceptMentionInput links a concept to a field or section on an artifact.
type ConceptMentionInput struct {
	ID           string
	ConceptID    string
	ArtifactID   string
	SectionID    string
	Field        string
	Weight       float64
	EvidenceJSON string
}

// ArtifactEdgeInput is an evidence-backed relationship between two artifacts.
type ArtifactEdgeInput struct {
	ID            string
	RepoID        string
	SrcArtifactID string
	DstArtifactID string
	EdgeType      string
	Weight        float64
	Confidence    float64
	EvidenceCount int
	Freshness     string
	SourceSignal  string
	Explanation   string
	MetadataJSON  string
}

// ArtifactEdgeRow is a stored artifact_edges row.
type ArtifactEdgeRow struct {
	ID            string
	RepoID        string
	SrcArtifactID string
	DstArtifactID string
	EdgeType      string
	Weight        float64
	Confidence    float64
	EvidenceCount int
	Freshness     string
	SourceSignal  string
	Explanation   string
	MetadataJSON  string
}

// ArtifactEdgeFilter limits artifact edge queries.
type ArtifactEdgeFilter struct {
	RepoID        string
	SrcArtifactID string
	DstArtifactID string
	EdgeType      string
}

// UpsertConcept creates or updates one concept by its repo/kind/canonical identity.
func (db *DB) UpsertConcept(c ConceptInput, now string) error {
	forms, err := json.Marshal(c.Forms)
	if err != nil {
		return fmt.Errorf("marshal concept forms: %w", err)
	}
	_, err = db.Exec(
		`INSERT INTO concepts
			(id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, kind, canonical) DO UPDATE SET
			id = excluded.id,
			forms_json = excluded.forms_json,
			document_frequency = excluded.document_frequency,
			inverse_document_frequency = excluded.inverse_document_frequency,
			updated_at = excluded.updated_at`,
		c.ID, c.RepoID, c.Canonical, c.Kind, string(forms), c.DocumentFrequency, c.InverseDocumentFrequency, now, now,
	)
	return err
}

// ReplaceConceptMentions replaces all mentions for one artifact.
func (db *DB) ReplaceConceptMentions(artifactID string, mentions []ConceptMentionInput, now string) error {
	if _, err := db.Exec("DELETE FROM concept_mentions WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, m := range mentions {
		if m.EvidenceJSON == "" {
			m.EvidenceJSON = "{}"
		}
		if _, err := db.Exec(
			`INSERT INTO concept_mentions
				(id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.ConceptID, m.ArtifactID, m.SectionID, m.Field, m.Weight, m.EvidenceJSON, now,
		); err != nil {
			return err
		}
	}
	return nil
}

// UpsertArtifactEdge creates or updates one edge by its stable edge identity.
func (db *DB) UpsertArtifactEdge(e ArtifactEdgeInput, now string) error {
	if e.EvidenceCount <= 0 {
		e.EvidenceCount = 1
	}
	if e.MetadataJSON == "" {
		e.MetadataJSON = "{}"
	}
	_, err := db.Exec(
		`INSERT INTO artifact_edges
			(id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence, evidence_count, freshness, source_signal, explanation, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, src_artifact_id, dst_artifact_id, edge_type, source_signal) DO UPDATE SET
			id = excluded.id,
			weight = excluded.weight,
			confidence = excluded.confidence,
			evidence_count = excluded.evidence_count,
			freshness = excluded.freshness,
			explanation = excluded.explanation,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at`,
		e.ID, e.RepoID, e.SrcArtifactID, e.DstArtifactID, e.EdgeType, e.Weight, e.Confidence, e.EvidenceCount, e.Freshness, e.SourceSignal, e.Explanation, e.MetadataJSON, now, now,
	)
	return err
}

// DeleteRepoEvidence removes all derived graph evidence for a repo.
func (db *DB) DeleteRepoEvidence(repoID string) error {
	if _, err := db.Exec("DELETE FROM artifact_edges WHERE repo_id = ?", repoID); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM concept_mentions WHERE artifact_id IN (SELECT id FROM artifacts WHERE repo_id = ?)", repoID); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM concepts WHERE repo_id = ?", repoID)
	return err
}

// DeleteArtifactEvidence removes derived graph evidence touching one artifact.
func (db *DB) DeleteArtifactEvidence(artifactID string) error {
	if _, err := db.Exec("DELETE FROM artifact_edges WHERE src_artifact_id = ? OR dst_artifact_id = ?", artifactID, artifactID); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM concept_mentions WHERE artifact_id = ?", artifactID)
	return err
}

// ReplaceRepoEvidence rebuilds derived graph evidence for a repo.
func (db *DB) ReplaceRepoEvidence(repoID string, concepts []ConceptInput, mentions []ConceptMentionInput, edges []ArtifactEdgeInput, now string) error {
	const savepoint = "evidence_graph_replace"
	if _, err := db.Exec("SAVEPOINT " + savepoint); err != nil {
		return err
	}
	rollback := func(err error) error {
		_, _ = db.Exec("ROLLBACK TO SAVEPOINT " + savepoint)
		_, _ = db.Exec("RELEASE SAVEPOINT " + savepoint)
		return err
	}
	if _, err := db.Exec("DELETE FROM artifact_edges WHERE repo_id = ?", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM concept_mentions WHERE artifact_id IN (SELECT id FROM artifacts WHERE repo_id = ?)", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM concepts WHERE repo_id = ?", repoID); err != nil {
		return rollback(err)
	}
	conceptStmt, err := db.Prepare(
		`INSERT INTO concepts
			(id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, kind, canonical) DO UPDATE SET
			id = excluded.id,
			forms_json = excluded.forms_json,
			document_frequency = excluded.document_frequency,
			inverse_document_frequency = excluded.inverse_document_frequency,
			updated_at = excluded.updated_at`,
	)
	if err != nil {
		return rollback(err)
	}
	defer conceptStmt.Close()
	for _, c := range concepts {
		forms, err := json.Marshal(c.Forms)
		if err != nil {
			return rollback(fmt.Errorf("marshal concept forms: %w", err))
		}
		if _, err := conceptStmt.Exec(c.ID, c.RepoID, c.Canonical, c.Kind, string(forms), c.DocumentFrequency, c.InverseDocumentFrequency, now, now); err != nil {
			return rollback(err)
		}
	}
	mentionStmt, err := db.Prepare(
		`INSERT INTO concept_mentions
			(id, concept_id, artifact_id, section_id, field, weight, evidence_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer mentionStmt.Close()
	for _, m := range mentions {
		if m.EvidenceJSON == "" {
			m.EvidenceJSON = "{}"
		}
		if _, err := mentionStmt.Exec(m.ID, m.ConceptID, m.ArtifactID, m.SectionID, m.Field, m.Weight, m.EvidenceJSON, now); err != nil {
			return rollback(err)
		}
	}
	edgeStmt, err := db.Prepare(
		`INSERT INTO artifact_edges
			(id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence, evidence_count, freshness, source_signal, explanation, metadata_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(repo_id, src_artifact_id, dst_artifact_id, edge_type, source_signal) DO UPDATE SET
			id = excluded.id,
			weight = excluded.weight,
			confidence = excluded.confidence,
			evidence_count = excluded.evidence_count,
			freshness = excluded.freshness,
			explanation = excluded.explanation,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at`,
	)
	if err != nil {
		return rollback(err)
	}
	defer edgeStmt.Close()
	for _, e := range edges {
		if e.EvidenceCount <= 0 {
			e.EvidenceCount = 1
		}
		if e.MetadataJSON == "" {
			e.MetadataJSON = "{}"
		}
		if _, err := edgeStmt.Exec(e.ID, e.RepoID, e.SrcArtifactID, e.DstArtifactID, e.EdgeType, e.Weight, e.Confidence, e.EvidenceCount, e.Freshness, e.SourceSignal, e.Explanation, e.MetadataJSON, now, now); err != nil {
			return rollback(err)
		}
	}
	if _, err := db.Exec("RELEASE SAVEPOINT " + savepoint); err != nil {
		return rollback(err)
	}
	return nil
}

// GetArtifactEdges returns artifact edges matching the filter.
func (db *DB) GetArtifactEdges(fp ArtifactEdgeFilter) ([]ArtifactEdgeRow, error) {
	query := `SELECT id, repo_id, src_artifact_id, dst_artifact_id, edge_type, weight, confidence, evidence_count,
		COALESCE(freshness,''), source_signal, COALESCE(explanation,''), COALESCE(metadata_json,'{}')
		FROM artifact_edges`
	var conditions []string
	var args []any
	if fp.RepoID != "" {
		conditions = append(conditions, "repo_id = ?")
		args = append(args, fp.RepoID)
	}
	if fp.SrcArtifactID != "" {
		conditions = append(conditions, "src_artifact_id = ?")
		args = append(args, fp.SrcArtifactID)
	}
	if fp.DstArtifactID != "" {
		conditions = append(conditions, "dst_artifact_id = ?")
		args = append(args, fp.DstArtifactID)
	}
	if fp.EdgeType != "" {
		conditions = append(conditions, "edge_type = ?")
		args = append(args, fp.EdgeType)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY edge_type, src_artifact_id, dst_artifact_id, source_signal"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactEdgeRow
	for rows.Next() {
		var r ArtifactEdgeRow
		if err := rows.Scan(&r.ID, &r.RepoID, &r.SrcArtifactID, &r.DstArtifactID, &r.EdgeType, &r.Weight, &r.Confidence, &r.EvidenceCount, &r.Freshness, &r.SourceSignal, &r.Explanation, &r.MetadataJSON); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
