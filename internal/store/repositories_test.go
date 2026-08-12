package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRepo_WithExistingGitIdentity_ReusesCanonicalRepository(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	seedRepositoryIdentity(t, db, "repo_a", rootA, "git@github.com:acme/example.git", "root-sha", "git_identity", now)
	identity := RepositoryIdentity{
		RootPath:    rootB,
		RemoteURL:   "git@github.com:acme/example.git",
		RootCommit:  "root-sha",
		GitIdentity: "git_identity",
	}

	repoID, err := db.ResolveRepo(identity, "repo_b", now)
	require.NoError(t, err)

	assert.Equal(t, "repo_a", repoID)
	var aliasRepoID string
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM repo_roots WHERE root_path = ?`, rootB).Scan(&aliasRepoID))
	assert.Equal(t, "repo_a", aliasRepoID)
}

func TestListArtifacts_WithCanonicalRepositoryRoot_ReturnsSharedArtifact(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	root := filepath.Join(t.TempDir(), "main")
	seedRepositoryIdentity(t, db, "repo_a", root, "remote", "root", "git_identity", now)
	seedRepositoryArtifact(t, db, "repo_a", now)

	artifacts, err := db.ListArtifacts(FilterParams{RepoRoot: root})
	require.NoError(t, err)

	require.Len(t, artifacts, 1)
	assert.Equal(t, "artifact", artifacts[0].ID)
}

func TestListArtifacts_WithRepositoryAliasRoot_ReturnsSharedArtifact(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	seedRepositoryIdentity(t, db, "repo_a", rootA, "remote", "root", "git_identity", now)
	seedRepositoryAlias(t, db, "repo_a", rootB, now)
	seedRepositoryArtifact(t, db, "repo_a", now)

	artifacts, err := db.ListArtifacts(FilterParams{RepoRoot: rootB})
	require.NoError(t, err)

	require.Len(t, artifacts, 1)
	assert.Equal(t, "artifact", artifacts[0].ID)
}

func TestResolveRepo_WithDuplicateRoot_RebindsRootAndSourceToCanonicalIdentity(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	require.NoError(t, os.MkdirAll(rootA, 0o755))
	require.NoError(t, os.MkdirAll(rootB, 0o755))
	seedRepositoryIdentity(t, db, "canonical", rootA, "remote", "root", "shared-git", now)
	seedRepositoryIdentity(t, db, "duplicate", rootB, "", "", "", now)
	seedPruneArtifact(t, db, "duplicate", "duplicate-artifact", "duplicate-source", now)
	seedPruneArtifact(t, db, "canonical", "canonical-artifact", "canonical-source", now)
	mustExecStoreTestSQL(t, db, `UPDATE sources SET repo_id = 'duplicate' WHERE id = 'canonical-source'`)
	identity := RepositoryIdentity{RootPath: rootB, GitIdentity: "shared-git", RemoteURL: "remote", RootCommit: "root"}

	resolvedID, err := db.ResolveRepo(identity, "unused", now)
	require.NoError(t, err)

	assert.Equal(t, "canonical", resolvedID)
	var aliasRepoID string
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM repo_roots WHERE root_path = ?`, rootB).Scan(&aliasRepoID))
	assert.Equal(t, "canonical", aliasRepoID)
	var sourceRepoID string
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM sources WHERE id = 'canonical-source'`).Scan(&sourceRepoID))
	assert.Equal(t, "canonical", sourceRepoID)
}

func TestPrune_WithRootlessDuplicate_RemovesDuplicateRepository(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	require.NoError(t, os.MkdirAll(rootA, 0o755))
	require.NoError(t, os.MkdirAll(rootB, 0o755))
	seedRepositoryIdentity(t, db, "canonical", rootA, "remote", "root", "shared-git", now)
	seedRepositoryIdentity(t, db, "duplicate", rootB, "", "", "", now)
	mustExecStoreTestSQL(t, db, `UPDATE repo_roots SET repo_id = 'canonical' WHERE root_path = ?`, rootB)
	seedPruneArtifact(t, db, "duplicate", "duplicate-artifact", "duplicate-source", now)

	report, err := db.Prune(PruneOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Equal(t, 1, report.ArtifactsPruned)
}

func TestPrune_WithSourceAttachedToSurvivingArtifact_NormalizesOwnership(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-07T00:00:00Z"
	liveRoot := filepath.Join(t.TempDir(), "live")
	require.NoError(t, os.MkdirAll(liveRoot, 0o755))
	staleRoot := filepath.Join(t.TempDir(), "gone")
	liveRepo, err := db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now)
	require.NoError(t, err)
	staleRepo, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	require.NoError(t, err)
	seedPruneArtifact(t, db, liveRepo, "survivor", "survivor-source", now)
	mustExecStoreTestSQL(t, db, `UPDATE sources SET repo_id = ? WHERE id = 'survivor-source'`, staleRepo)

	report, err := db.Prune(PruneOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Zero(t, report.ArtifactsPruned)
	var artifactRepo, sourceRepo string
	require.NoError(t, db.QueryRow(`SELECT a.repo_id, s.repo_id FROM artifacts a JOIN sources s ON s.artifact_id = a.id WHERE a.id = 'survivor'`).Scan(&artifactRepo, &sourceRepo))
	assert.Equal(t, liveRepo, artifactRepo)
	assert.Equal(t, liveRepo, sourceRepo)
}

func TestPrune_DryRunWithLiveAlias_ReportsWithoutDeleting(t *testing.T) {
	db := openTestDB(t)
	scenario := seedPruneLiveAliasScenario(t, db)

	report, err := db.Prune(PruneOptions{DryRun: true})
	require.NoError(t, err)

	assert.True(t, report.DryRun)
	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Equal(t, 1, report.ArtifactsPruned)
	require.Len(t, report.StaleRoots, 2)
	assert.Equal(t, scenario.staleOnly, report.StaleRoots[0])
	assert.Equal(t, scenario.staleAlias, report.StaleRoots[1])
	assertTableCount(t, db, "repos", 2)
}

func TestPrune_WithLiveAlias_RemovesOnlyStaleRepositoryData(t *testing.T) {
	db := openTestDB(t)
	scenario := seedPruneLiveAliasScenario(t, db)

	report, err := db.Prune(PruneOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Equal(t, 1, report.ArtifactsPruned)
	meta := db.GetRepoByRoot(scenario.liveRoot)
	require.NotNil(t, meta)
	assert.Equal(t, scenario.liveRepo, meta.ID)
	assert.Nil(t, db.GetRepoByRoot(scenario.staleAlias))
	assertRowCountForCondition(t, db, "repos", "id = ?", scenario.staleRepo, 0)
	assertRowCountForCondition(t, db, "artifacts", "id = 'artifact_stale'", nil, 0)
	assertRowCountForCondition(t, db, "artifact_revisions", "artifact_id = 'artifact_stale'", nil, 0)
	assertRowCountForCondition(t, db, "sources", "artifact_id = 'artifact_stale'", nil, 0)
	assertRowCountForCondition(t, db, "artifacts_fts", "artifact_id = 'artifact_stale'", nil, 0)
}

func TestPrune_WithVacuum_ReportsCompactionProgress(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	staleRoot := filepath.Join(t.TempDir(), "gone")
	repoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	require.NoError(t, err)
	seedPruneArtifact(t, db, repoID, "vacuum-artifact", "vacuum-source", now)
	var phases []string

	report, err := db.Prune(PruneOptions{
		Vacuum: true,
		Progress: func(progress PruneProgress) {
			phases = append(phases, progress.Phase)
		},
	})
	require.NoError(t, err)

	assert.True(t, report.Vacuumed)
	assert.Equal(t, 1, report.RepositoriesPruned)
	require.Len(t, phases, 4)
	assert.Equal(t, "inspect", phases[0])
	assert.Equal(t, "delete", phases[1])
	assert.Equal(t, "compact", phases[2])
	assert.Equal(t, "complete", phases[3])
}

func TestPrune_DryRunWithConsecutiveCaptureRevisions_ReportsWithoutCompacting(t *testing.T) {
	db := openTestDB(t)
	seedPruneCompactionScenario(t, db)

	report, err := db.Prune(PruneOptions{DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, 2, report.RevisionsCompacted)
	assert.Equal(t, 1, report.RepositoriesPruned)
	assertRevisionCount(t, db, "capture-artifact", 5)
}

func TestPrune_WithConsecutiveCaptureRevisions_CompactsAndRebindsDependents(t *testing.T) {
	db := openTestDB(t)
	seedPruneCompactionScenario(t, db)

	report, err := db.Prune(PruneOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, report.RevisionsCompacted)
	assertRowCountForCondition(t, db, "artifacts", "id = 'stale-capture'", nil, 0)
	assertRevisionCount(t, db, "capture-artifact", 3)
	var kept string
	require.NoError(t, db.QueryRow("SELECT GROUP_CONCAT(id, ',') FROM (SELECT id FROM artifact_revisions WHERE artifact_id = 'capture-artifact' ORDER BY observed_at)").Scan(&kept))
	assert.Equal(t, "r2,r3,r5", kept)
	assertRevisionReference(t, db, "artifact_todos", "r2")
	assertRevisionReference(t, db, "artifact_criteria", "r5")
	assertRevisionReference(t, db, "artifact_sections", "r5")
	var current string
	require.NoError(t, db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = 'capture-artifact'").Scan(&current))
	assert.Equal(t, "r5", current)
}

func TestPruneDryRunUsesSourceArtifactRepoIndex(t *testing.T) {
	db := openTestDB(t)
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT COUNT(*) FROM artifacts a
		WHERE a.repo_id IN (?)
		  AND NOT EXISTS (
			SELECT 1 FROM sources s
			WHERE s.artifact_id = a.id
			  AND s.repo_id NOT IN (?)
		  )`, "stale", "stale")
	require.NoError(t, err)
	defer rows.Close()

	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		if strings.Contains(detail, "idx_sources_artifact_repo") {
			found = true
		}
	}
	require.NoError(t, rows.Err())
	assert.True(t, found)
}

func BenchmarkPruneDryRunFatIndex(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(b, err)
	defer db.Close()
	now := "2026-08-07T00:00:00Z"
	liveRoot := filepath.Join(b.TempDir(), "live")
	require.NoError(b, os.MkdirAll(liveRoot, 0o755))
	staleRoot := filepath.Join(b.TempDir(), "gone")
	_, err = db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now)
	require.NoError(b, err)
	_, err = db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	require.NoError(b, err)
	tx, err := db.Begin()
	require.NoError(b, err)
	artifactStmt, err := tx.Prepare(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, ?, 'source_context', '', ?, 'unknown', ?, ?, ?, ?)`)
	require.NoError(b, err)
	sourceStmt, err := tx.Prepare(`INSERT INTO sources
		(id, artifact_id, repo_id, source_type, path, source_identity, format_profile, created_at, updated_at)
		VALUES (?, ?, ?, 'source', ?, ?, 'generic', ?, ?)`)
	require.NoError(b, err)
	for i := 0; i < 30350; i++ {
		repoID := "live"
		if i >= 30000 {
			repoID = "stale"
		}
		artifactID := fmt.Sprintf("artifact_%05d", i)
		path := fmt.Sprintf("src/file_%05d.go", i)
		_, err = artifactStmt.Exec(artifactID, repoID, artifactID, now, now, now, now)
		require.NoError(b, err)
		_, err = sourceStmt.Exec("source_"+artifactID, artifactID, repoID, path, path+"|source", now, now)
		require.NoError(b, err)
	}
	require.NoError(b, artifactStmt.Close())
	require.NoError(b, sourceStmt.Close())
	require.NoError(b, tx.Commit())

	b.ResetTimer()
	var report PruneReport
	for i := 0; i < b.N; i++ {
		report, err = db.Prune(PruneOptions{DryRun: true})
		if err != nil {
			require.NoError(b, err)
		}
	}
	b.StopTimer()
	assert.Equal(b, 1, report.RepositoriesPruned)
	assert.Equal(b, 350, report.ArtifactsPruned)
}

type pruneAliasScenario struct {
	liveRoot   string
	staleAlias string
	staleOnly  string
	liveRepo   string
	staleRepo  string
}

func seedPruneLiveAliasScenario(t *testing.T, db *DB) pruneAliasScenario {
	t.Helper()

	now := "2026-08-06T00:00:00Z"
	tmp := t.TempDir()
	scenario := pruneAliasScenario{
		liveRoot:   filepath.Join(tmp, "live"),
		staleAlias: filepath.Join(tmp, "gone-worktree"),
		staleOnly:  filepath.Join(tmp, "gone-repo"),
	}
	require.NoError(t, os.MkdirAll(scenario.liveRoot, 0o755))
	liveIdentity := RepositoryIdentity{RootPath: scenario.liveRoot, GitIdentity: "live-git", RemoteURL: "remote", RootCommit: "root"}
	var err error
	scenario.liveRepo, err = db.ResolveRepo(liveIdentity, "repo_live", now)
	require.NoError(t, err)
	liveIdentity.RootPath = scenario.staleAlias
	_, err = db.ResolveRepo(liveIdentity, "unused", now)
	require.NoError(t, err)
	scenario.staleRepo, err = db.ResolveRepo(RepositoryIdentity{RootPath: scenario.staleOnly}, "repo_stale", now)
	require.NoError(t, err)
	seedPruneArtifact(t, db, scenario.liveRepo, "artifact_live", "source_live", now)
	seedPruneArtifact(t, db, scenario.staleRepo, "artifact_stale", "source_stale", now)
	return scenario
}

func seedPruneCompactionScenario(t *testing.T, db *DB) {
	t.Helper()

	now := "2026-08-06T00:00:00Z"
	liveRoot := t.TempDir()
	repoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect("capture-artifact", repoID, "plan", "", "Plan", "draft", "r5", now, now))
	require.NoError(t, db.InsertSourceDirect("capture-source", "capture-artifact", repoID, "capture", "plan.md", "plan.md|capture", "generic", "", now))
	require.NoError(t, db.InsertRevisionDirect("r1", "capture-artifact", "hash-a", "A", "", "2026-08-06T00:00:01Z"))
	require.NoError(t, db.InsertRevisionDirect("r2", "capture-artifact", "hash-a", "A", "", "2026-08-06T00:00:02Z"))
	require.NoError(t, db.InsertRevisionDirect("r3", "capture-artifact", "hash-b", "B", "", "2026-08-06T00:00:03Z"))
	require.NoError(t, db.InsertRevisionDirect("r4", "capture-artifact", "hash-a", "A", "", "2026-08-06T00:00:04Z"))
	require.NoError(t, db.InsertRevisionDirect("r5", "capture-artifact", "hash-a", "A", "", "2026-08-06T00:00:05Z"))
	staleRoot := filepath.Join(t.TempDir(), "gone")
	staleRepoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect("stale-capture", staleRepoID, "plan", "", "Stale", "draft", "sr2", now, now))
	require.NoError(t, db.InsertSourceDirect("stale-capture-source", "stale-capture", staleRepoID, "capture", "stale.md", "stale.md|capture", "generic", "", now))
	require.NoError(t, db.InsertRevisionDirect("sr1", "stale-capture", "stale-hash", "stale", "", now))
	require.NoError(t, db.InsertRevisionDirect("sr2", "stale-capture", "stale-hash", "stale", "", now))
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_todos
		(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
		VALUES ('todo', 'capture-artifact', 'r1', 0, 'todo', 0, 'plan.md', 1, ?)`, now)
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_criteria
		(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at)
		VALUES ('criterion', 'capture-artifact', 'r4', 0, 'criterion', 0, 'plan.md', 2, 'acceptance', ?)`, now)
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_sections
		(id, artifact_id, revision_id, source_path, heading_path, heading_depth, start_line, end_line, title, body, token_estimate, created_at)
		VALUES ('section', 'capture-artifact', 'r4', 'plan.md', 'Plan', 1, 1, 2, 'Plan', 'A', 1, ?)`, now)
}

func seedRepositoryIdentity(t *testing.T, db *DB, repoID, root, remote, rootCommit, gitIdentity, now string) {
	t.Helper()

	mustExecStoreTestSQL(t, db, `INSERT INTO repos (
		id, root_path, git_remote_url, git_root_commit, git_identity, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`, repoID, root, remote, rootCommit, gitIdentity, now, now)
}

func seedRepositoryAlias(t *testing.T, db *DB, repoID, root, now string) {
	t.Helper()

	mustExecStoreTestSQL(t, db, `INSERT INTO repo_roots (root_path, repo_id, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?)`, root, repoID, now, now)
}

func seedRepositoryArtifact(t *testing.T, db *DB, repoID, now string) {
	t.Helper()

	mustExecStoreTestSQL(t, db, `INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('artifact', ?, 'plan', '', 'Plan', 'draft', ?, ?, ?, ?)`, repoID, now, now, now, now)
}

func seedPruneArtifact(t *testing.T, db *DB, repoID, artifactID, sourceID, now string) {
	t.Helper()

	revisionID := "rev_" + artifactID
	require.NoError(t, db.InsertArtifactDirect(artifactID, repoID, "plan", "", artifactID, "draft", revisionID, now, now))
	require.NoError(t, db.InsertRevisionDirect(revisionID, artifactID, "hash", "body", "", now))
	require.NoError(t, db.InsertSourceDirect(sourceID, artifactID, repoID, "markdown", artifactID+".md", artifactID+"|markdown", "", "", now))
	db.IndexArtifactFTS(artifactID, artifactID, "body", artifactID+".md")
}

func assertRowCountForCondition(t *testing.T, db *DB, table, condition string, arg any, want int) {
	t.Helper()

	query := "SELECT COUNT(*) FROM " + table + " WHERE " + condition
	var count int
	if arg == nil {
		require.NoError(t, db.QueryRow(query).Scan(&count))
	} else {
		require.NoError(t, db.QueryRow(query, arg).Scan(&count))
	}
	assert.Equal(t, want, count, "%s row count", table)
}

func assertRevisionCount(t *testing.T, db *DB, artifactID string, want int) {
	t.Helper()

	var got int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", artifactID).Scan(&got))
	assert.Equal(t, want, got)
}

func assertRevisionReference(t *testing.T, db *DB, table, want string) {
	t.Helper()

	var got string
	require.NoError(t, db.QueryRow("SELECT revision_id FROM "+table+" WHERE artifact_id = 'capture-artifact'").Scan(&got))
	assert.Equal(t, want, got, "%s revision", table)
}
