package scan

import (
	"encoding/json"

	"github.com/devspecs-com/devspecs-cli/internal/openspecmetrics"
)

func (s *Scanner) computeOpenSpecMetrics(repoRoot, repoID string) (*openspecmetrics.Metrics, error) {
	rows, err := s.db.Query(
		`SELECT COALESCE(s.path, ''), s.source_type, s.source_identity, a.subtype, COALESCE(rv.extracted_json, '')
		 FROM artifacts a
		 JOIN sources s ON s.artifact_id = a.id
		 LEFT JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		 WHERE a.repo_id = ?
		 ORDER BY s.path, s.source_identity`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []openspecmetrics.Artifact
	for rows.Next() {
		var artifact openspecmetrics.Artifact
		var extractedJSON string
		if err := rows.Scan(&artifact.Path, &artifact.SourceType, &artifact.SourceIdentity, &artifact.Subtype, &extractedJSON); err != nil {
			return nil, err
		}
		applyOpenSpecMetricExtracted(&artifact, extractedJSON)
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	metrics := openspecmetrics.Analyze(repoRoot, artifacts)
	if !metrics.HasData() {
		return nil, nil
	}
	return &metrics, nil
}

func applyOpenSpecMetricExtracted(artifact *openspecmetrics.Artifact, extractedJSON string) {
	if extractedJSON == "" {
		return
	}
	var payload struct {
		ArtifactScope string `json:"artifact_scope"`
		OpenSpecRole  string `json:"openspec_role"`
	}
	if err := json.Unmarshal([]byte(extractedJSON), &payload); err != nil {
		return
	}
	artifact.ArtifactScope = payload.ArtifactScope
	artifact.OpenSpecRole = payload.OpenSpecRole
}
