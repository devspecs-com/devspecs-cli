package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepairLegacyArtifactOwnershipRepairsOnlyUnanimousSources(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-07T00:00:00Z"
	seedOwnershipRepo(t, db, "old", now)
	seedOwnershipRepo(t, db, "target", now)
	seedOwnershipRepo(t, db, "other", now)
	seedPruneArtifact(t, db, "old", "repairable", "repairable-source", now)
	_, err := db.Exec(`UPDATE sources SET repo_id = 'target' WHERE id = 'repairable-source'`)
	require.NoError(t, err)

	seedPruneArtifact(t, db, "old", "ambiguous", "ambiguous-source", now)
	_, err = db.Exec(`UPDATE sources SET repo_id = 'target' WHERE id = 'ambiguous-source'`)
	require.NoError(t, err)
	require.NoError(t, db.InsertSourceDirect("ambiguous-other", "ambiguous", "other", "markdown", "shared.md", "shared.md|markdown", "", "", now))

	report, err := db.RepairLegacyArtifactOwnership("target", []string{"repairable|markdown", "ambiguous|markdown"}, now)
	require.NoError(t, err)
	assert.Equal(t, 1, report.RepairedArtifacts)
	assert.Equal(t, 1, report.AmbiguousIdentities)

	var repairableRepo, ambiguousRepo string
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM artifacts WHERE id = 'repairable'`).Scan(&repairableRepo))
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM artifacts WHERE id = 'ambiguous'`).Scan(&ambiguousRepo))
	assert.Equal(t, "target", repairableRepo)
	assert.Equal(t, "old", ambiguousRepo)
}

func seedOwnershipRepo(t *testing.T, db *DB, repoID, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)`, repoID, "/"+repoID, now, now)
	require.NoError(t, err)
}
