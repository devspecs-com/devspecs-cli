package store

import (
	"fmt"
	"sort"
	"strings"
)

// LegacyOwnershipRepairReport summarizes conservative source ownership repair.
type LegacyOwnershipRepairReport struct {
	RepairedArtifacts   int
	AmbiguousIdentities int
}

type legacyOwnershipMatch struct {
	artifactID      string
	artifactRepoID  string
	sourceRepoCount int
	soleSourceRepo  string
}

// RepairLegacyArtifactOwnership moves artifacts whose current sources
// unanimously belong to repoID. Ambiguous ownership is left untouched.
func (db *DB) RepairLegacyArtifactOwnership(repoID string, sourceIdentities []string, now string) (LegacyOwnershipRepairReport, error) {
	report := LegacyOwnershipRepairReport{}
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return report, fmt.Errorf("repository id is required")
	}
	identities := uniqueOwnershipIdentities(sourceIdentities)
	if len(identities) == 0 {
		return report, nil
	}

	tentative := map[string]string{}
	blocked := map[string]bool{}
	const chunkSize = 400
	for start := 0; start < len(identities); start += chunkSize {
		end := start + chunkSize
		if end > len(identities) {
			end = len(identities)
		}
		chunk := identities[start:end]
		args := []any{repoID}
		for _, identity := range chunk {
			args = append(args, identity)
		}
		rows, err := db.Query(`
			SELECT matched.source_identity, a.id, a.repo_id,
			       COUNT(DISTINCT owned.repo_id), COALESCE(MIN(owned.repo_id), '')
			FROM sources matched
			JOIN artifacts a ON a.id = matched.artifact_id
			JOIN sources owned ON owned.artifact_id = a.id
			WHERE matched.repo_id = ?
			  AND matched.source_identity IN (`+ownershipQuestionMarks(len(chunk))+`)
			GROUP BY matched.source_identity, a.id, a.repo_id
			ORDER BY matched.source_identity, a.id`, args...)
		if err != nil {
			return report, err
		}
		byIdentity := map[string][]legacyOwnershipMatch{}
		for rows.Next() {
			var identity string
			var match legacyOwnershipMatch
			if err := rows.Scan(&identity, &match.artifactID, &match.artifactRepoID, &match.sourceRepoCount, &match.soleSourceRepo); err != nil {
				rows.Close()
				return report, err
			}
			byIdentity[identity] = append(byIdentity[identity], match)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return report, err
		}
		rows.Close()

		for _, identity := range chunk {
			matches := byIdentity[identity]
			targetExists := false
			var candidates []legacyOwnershipMatch
			var mismatches []legacyOwnershipMatch
			for _, match := range matches {
				if match.artifactRepoID == repoID {
					targetExists = true
					continue
				}
				mismatches = append(mismatches, match)
				if match.sourceRepoCount == 1 && match.soleSourceRepo == repoID {
					candidates = append(candidates, match)
				}
			}
			if len(mismatches) == 0 {
				continue
			}
			if !targetExists && len(candidates) == 1 && len(mismatches) == 1 {
				tentative[candidates[0].artifactID] = candidates[0].artifactRepoID
				continue
			}
			report.AmbiguousIdentities++
			for _, match := range mismatches {
				blocked[match.artifactID] = true
			}
		}
	}
	for artifactID := range blocked {
		delete(tentative, artifactID)
	}
	if len(tentative) == 0 {
		return report, nil
	}

	artifactIDs := make([]string, 0, len(tentative))
	for artifactID := range tentative {
		artifactIDs = append(artifactIDs, artifactID)
	}
	sort.Strings(artifactIDs)
	if _, err := db.Exec("SAVEPOINT legacy_ownership_repair"); err != nil {
		return report, err
	}
	rollback := func(err error) (LegacyOwnershipRepairReport, error) {
		_, _ = db.Exec("ROLLBACK TO SAVEPOINT legacy_ownership_repair")
		_, _ = db.Exec("RELEASE SAVEPOINT legacy_ownership_repair")
		return LegacyOwnershipRepairReport{}, err
	}
	oldRepos := map[string]bool{}
	for _, artifactID := range artifactIDs {
		oldRepoID := tentative[artifactID]
		if _, err := db.Exec("DELETE FROM artifact_edges WHERE src_artifact_id = ? OR dst_artifact_id = ?", artifactID, artifactID); err != nil {
			return rollback(err)
		}
		if _, err := db.Exec("DELETE FROM concept_mentions WHERE artifact_id = ?", artifactID); err != nil {
			return rollback(err)
		}
		result, err := db.Exec("UPDATE artifacts SET repo_id = ?, updated_at = ? WHERE id = ? AND repo_id = ?", repoID, now, artifactID, oldRepoID)
		if err != nil {
			return rollback(err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return rollback(err)
		}
		if changed != 1 {
			return rollback(fmt.Errorf("legacy ownership changed %d rows for artifact %s", changed, artifactID))
		}
		oldRepos[oldRepoID] = true
	}
	for oldRepoID := range oldRepos {
		if _, err := db.Exec(`DELETE FROM concepts
			WHERE repo_id = ?
			  AND NOT EXISTS (SELECT 1 FROM concept_mentions cm WHERE cm.concept_id = concepts.id)`, oldRepoID); err != nil {
			return rollback(err)
		}
	}
	if _, err := db.Exec("RELEASE SAVEPOINT legacy_ownership_repair"); err != nil {
		return report, err
	}
	report.RepairedArtifacts = len(artifactIDs)
	return report, nil
}

func uniqueOwnershipIdentities(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func ownershipQuestionMarks(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}
