package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRepo_ReusesStableIdentityAndAliasesQueries(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	identity := RepositoryIdentity{
		RootPath:    rootA,
		RemoteURL:   "git@github.com:acme/example.git",
		RootCommit:  "root-sha",
		GitIdentity: "git_identity",
	}
	repoA, err := db.ResolveRepo(identity, "repo_a", now)
	if err != nil {
		t.Fatal(err)
	}
	identity.RootPath = rootB
	repoB, err := db.ResolveRepo(identity, "repo_b", now)
	if err != nil {
		t.Fatal(err)
	}
	if repoA != repoB {
		t.Fatalf("same Git identity created two repos: %q != %q", repoA, repoB)
	}
	if _, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('artifact', ?, 'plan', '', 'Plan', 'draft', ?, ?, ?, ?)`, repoA, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{rootA, rootB} {
		artifacts, err := db.ListArtifacts(FilterParams{RepoRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if len(artifacts) != 1 || artifacts[0].ID != "artifact" {
			t.Fatalf("root %q did not resolve shared artifact: %#v", root, artifacts)
		}
	}
}

func TestResolveRepo_RebindsExistingDuplicateRootToCanonicalIdentity(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	rootA := filepath.Join(t.TempDir(), "main")
	rootB := filepath.Join(t.TempDir(), "worktree")
	for _, root := range []string{rootA, rootB} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	identity := RepositoryIdentity{RootPath: rootA, GitIdentity: "shared-git", RemoteURL: "remote", RootCommit: "root"}
	canonicalID, err := db.ResolveRepo(identity, "canonical", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('duplicate', ?, ?, ?)`, rootB, now, now); err != nil {
		t.Fatal(err)
	}
	seedPruneArtifact(t, db, "duplicate", "duplicate-artifact", "duplicate-source", now)
	seedPruneArtifact(t, db, canonicalID, "canonical-artifact", "canonical-source", now)
	if _, err := db.Exec(`UPDATE sources SET repo_id = 'duplicate' WHERE id = 'canonical-source'`); err != nil {
		t.Fatal(err)
	}
	identity.RootPath = rootB
	resolvedID, err := db.ResolveRepo(identity, "unused", now)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedID != canonicalID {
		t.Fatalf("duplicate root resolved to %q, want canonical %q", resolvedID, canonicalID)
	}
	var aliasRepo string
	if err := db.QueryRow(`SELECT repo_id FROM repo_roots WHERE root_path = ?`, rootB).Scan(&aliasRepo); err != nil {
		t.Fatal(err)
	}
	if aliasRepo != canonicalID {
		t.Fatalf("root alias still points at duplicate repo %q", aliasRepo)
	}
	var canonicalSourceRepo string
	if err := db.QueryRow(`SELECT repo_id FROM sources WHERE id = 'canonical-source'`).Scan(&canonicalSourceRepo); err != nil {
		t.Fatal(err)
	}
	if canonicalSourceRepo != canonicalID {
		t.Fatalf("canonical artifact source remained on duplicate repo %q", canonicalSourceRepo)
	}
	report, err := db.Prune(PruneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepositoriesPruned != 1 || report.ArtifactsPruned != 1 {
		t.Fatalf("rootless duplicate was not pruned: %#v", report)
	}
}

func TestPrune_PreservesSourceAttachedToSurvivingArtifact(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-07T00:00:00Z"
	liveRoot := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(liveRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	staleRoot := filepath.Join(t.TempDir(), "gone")
	liveRepo, err := db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now)
	if err != nil {
		t.Fatal(err)
	}
	staleRepo, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	if err != nil {
		t.Fatal(err)
	}
	seedPruneArtifact(t, db, liveRepo, "survivor", "survivor-source", now)
	if _, err := db.Exec(`UPDATE sources SET repo_id = ? WHERE id = 'survivor-source'`, staleRepo); err != nil {
		t.Fatal(err)
	}
	report, err := db.Prune(PruneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepositoriesPruned != 1 || report.ArtifactsPruned != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	var artifactRepo, sourceRepo string
	if err := db.QueryRow(`SELECT a.repo_id, s.repo_id FROM artifacts a JOIN sources s ON s.artifact_id = a.id WHERE a.id = 'survivor'`).Scan(&artifactRepo, &sourceRepo); err != nil {
		t.Fatal(err)
	}
	if artifactRepo != liveRepo || sourceRepo != liveRepo {
		t.Fatalf("surviving ownership was not normalized: artifact=%q source=%q", artifactRepo, sourceRepo)
	}
}

func TestPrune_RemovesStaleRepoButKeepsLogicalRepoWithLiveAlias(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	tmp := t.TempDir()
	liveRoot := filepath.Join(tmp, "live")
	if err := os.MkdirAll(liveRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	staleAlias := filepath.Join(tmp, "gone-worktree")
	staleOnly := filepath.Join(tmp, "gone-repo")

	liveIdentity := RepositoryIdentity{RootPath: liveRoot, GitIdentity: "live-git", RemoteURL: "remote", RootCommit: "root"}
	liveRepo, err := db.ResolveRepo(liveIdentity, "repo_live", now)
	if err != nil {
		t.Fatal(err)
	}
	liveIdentity.RootPath = staleAlias
	if _, err := db.ResolveRepo(liveIdentity, "unused", now); err != nil {
		t.Fatal(err)
	}
	staleRepo, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleOnly}, "repo_stale", now)
	if err != nil {
		t.Fatal(err)
	}

	seedPruneArtifact(t, db, liveRepo, "artifact_live", "source_live", now)
	seedPruneArtifact(t, db, staleRepo, "artifact_stale", "source_stale", now)

	dryRun, err := db.Prune(PruneOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.RepositoriesPruned != 1 || dryRun.ArtifactsPruned != 1 || len(dryRun.StaleRoots) != 2 {
		t.Fatalf("unexpected dry-run report: %#v", dryRun)
	}
	var repoCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&repoCount); err != nil || repoCount != 2 {
		t.Fatalf("dry run changed repositories: count=%d err=%v", repoCount, err)
	}

	report, err := db.Prune(PruneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepositoriesPruned != 1 || report.ArtifactsPruned != 1 {
		t.Fatalf("unexpected prune report: %#v", report)
	}
	if meta := db.GetRepoByRoot(liveRoot); meta == nil || meta.ID != liveRepo {
		t.Fatalf("live alias lost after prune: %#v", meta)
	}
	if meta := db.GetRepoByRoot(staleAlias); meta != nil {
		t.Fatalf("stale alias still resolves: %#v", meta)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = ?`, staleRepo).Scan(&repoCount); err != nil || repoCount != 0 {
		t.Fatalf("stale repo remains: count=%d err=%v", repoCount, err)
	}
	for table := range map[string]struct{}{"artifacts": {}, "artifact_revisions": {}, "sources": {}, "artifacts_fts": {}} {
		var count int
		query := "SELECT COUNT(*) FROM " + table + " WHERE "
		if table == "artifacts" {
			query += "id = 'artifact_stale'"
		} else if table == "artifact_revisions" {
			query += "artifact_id = 'artifact_stale'"
		} else if table == "sources" {
			query += "artifact_id = 'artifact_stale'"
		} else {
			query += "artifact_id = 'artifact_stale'"
		}
		if err := db.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s retained stale rows: count=%d err=%v", table, count, err)
		}
	}
}

func TestPrune_VacuumCompactsAfterDeletion(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	staleRoot := filepath.Join(t.TempDir(), "gone")
	repoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	if err != nil {
		t.Fatal(err)
	}
	seedPruneArtifact(t, db, repoID, "vacuum-artifact", "vacuum-source", now)
	var phases []string
	report, err := db.Prune(PruneOptions{
		Vacuum: true,
		Progress: func(progress PruneProgress) {
			phases = append(phases, progress.Phase)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Vacuumed || report.RepositoriesPruned != 1 {
		t.Fatalf("vacuum prune report = %#v", report)
	}
	if got, want := strings.Join(phases, ","), "inspect,delete,compact,complete"; got != want {
		t.Fatalf("prune progress phases = %q, want %q", got, want)
	}
}

func TestPrune_CompactsOnlyConsecutiveCaptureRevisions(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-06T00:00:00Z"
	liveRoot := t.TempDir()
	repoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("capture-artifact", repoID, "plan", "", "Plan", "draft", "r5", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("capture-source", "capture-artifact", repoID, "capture", "plan.md", "plan.md|capture", "generic", "", now); err != nil {
		t.Fatal(err)
	}
	revisions := []struct {
		id, hash, body, observed string
	}{
		{"r1", "hash-a", "A", "2026-08-06T00:00:01Z"},
		{"r2", "hash-a", "A", "2026-08-06T00:00:02Z"},
		{"r3", "hash-b", "B", "2026-08-06T00:00:03Z"},
		{"r4", "hash-a", "A", "2026-08-06T00:00:04Z"},
		{"r5", "hash-a", "A", "2026-08-06T00:00:05Z"},
	}
	for _, revision := range revisions {
		if err := db.InsertRevisionDirect(revision.id, "capture-artifact", revision.hash, revision.body, "", revision.observed); err != nil {
			t.Fatal(err)
		}
	}
	staleRoot := filepath.Join(t.TempDir(), "gone")
	staleRepoID, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("stale-capture", staleRepoID, "plan", "", "Stale", "draft", "sr2", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("stale-capture-source", "stale-capture", staleRepoID, "capture", "stale.md", "stale.md|capture", "generic", "", now); err != nil {
		t.Fatal(err)
	}
	for _, revisionID := range []string{"sr1", "sr2"} {
		if err := db.InsertRevisionDirect(revisionID, "stale-capture", "stale-hash", "stale", "", now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO artifact_todos
		(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
		VALUES ('todo', 'capture-artifact', 'r1', 0, 'todo', 0, 'plan.md', 1, ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artifact_criteria
		(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at)
		VALUES ('criterion', 'capture-artifact', 'r4', 0, 'criterion', 0, 'plan.md', 2, 'acceptance', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artifact_sections
		(id, artifact_id, revision_id, source_path, heading_path, heading_depth, start_line, end_line, title, body, token_estimate, created_at)
		VALUES ('section', 'capture-artifact', 'r4', 'plan.md', 'Plan', 1, 1, 2, 'Plan', 'A', 1, ?)`, now); err != nil {
		t.Fatal(err)
	}

	preview, err := db.Prune(PruneOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.RevisionsCompacted != 2 {
		t.Fatalf("dry-run compacted revisions = %d, want 2", preview.RevisionsCompacted)
	}
	if preview.RepositoriesPruned != 1 {
		t.Fatalf("dry-run repositories pruned = %d, want 1", preview.RepositoriesPruned)
	}
	assertRevisionCount(t, db, "capture-artifact", 5)

	report, err := db.Prune(PruneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RevisionsCompacted != 2 {
		t.Fatalf("compacted revisions = %d, want 2", report.RevisionsCompacted)
	}
	var staleArtifacts int
	if err := db.QueryRow("SELECT COUNT(*) FROM artifacts WHERE id = 'stale-capture'").Scan(&staleArtifacts); err != nil {
		t.Fatal(err)
	}
	if staleArtifacts != 0 {
		t.Fatalf("stale capture remains after prune: %d", staleArtifacts)
	}
	assertRevisionCount(t, db, "capture-artifact", 3)
	var kept string
	if err := db.QueryRow("SELECT GROUP_CONCAT(id, ',') FROM (SELECT id FROM artifact_revisions WHERE artifact_id = 'capture-artifact' ORDER BY observed_at)").Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != "r2,r3,r5" {
		t.Fatalf("kept revisions = %q, want r2,r3,r5", kept)
	}
	for table, want := range map[string]string{"artifact_todos": "r2", "artifact_criteria": "r5", "artifact_sections": "r5"} {
		var got string
		if err := db.QueryRow("SELECT revision_id FROM " + table + " WHERE artifact_id = 'capture-artifact'").Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s revision = %q, want %q", table, got, want)
		}
	}
	var current string
	if err := db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = 'capture-artifact'").Scan(&current); err != nil {
		t.Fatal(err)
	}
	if current != "r5" {
		t.Fatalf("current revision = %q, want r5", current)
	}
}

func assertRevisionCount(t *testing.T, db *DB, artifactID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", artifactID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("revision count for %s = %d, want %d", artifactID, got, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "idx_sources_artifact_repo") {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("prune ownership lookup did not use idx_sources_artifact_repo")
	}
}

func BenchmarkPruneDryRunFatIndex(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	now := "2026-08-07T00:00:00Z"
	liveRoot := filepath.Join(b.TempDir(), "live")
	if err := os.MkdirAll(liveRoot, 0o755); err != nil {
		b.Fatal(err)
	}
	staleRoot := filepath.Join(b.TempDir(), "gone")
	if _, err := db.ResolveRepo(RepositoryIdentity{RootPath: liveRoot}, "live", now); err != nil {
		b.Fatal(err)
	}
	if _, err := db.ResolveRepo(RepositoryIdentity{RootPath: staleRoot}, "stale", now); err != nil {
		b.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	artifactStmt, err := tx.Prepare(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, ?, 'source_context', '', ?, 'unknown', ?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	sourceStmt, err := tx.Prepare(`INSERT INTO sources
		(id, artifact_id, repo_id, source_type, path, source_identity, format_profile, created_at, updated_at)
		VALUES (?, ?, ?, 'source', ?, ?, 'generic', ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 30350; i++ {
		repoID := "live"
		if i >= 30000 {
			repoID = "stale"
		}
		artifactID := fmt.Sprintf("artifact_%05d", i)
		path := fmt.Sprintf("src/file_%05d.go", i)
		if _, err := artifactStmt.Exec(artifactID, repoID, artifactID, now, now, now, now); err != nil {
			b.Fatal(err)
		}
		if _, err := sourceStmt.Exec("source_"+artifactID, artifactID, repoID, path, path+"|source", now, now); err != nil {
			b.Fatal(err)
		}
	}
	if err := artifactStmt.Close(); err != nil {
		b.Fatal(err)
	}
	if err := sourceStmt.Close(); err != nil {
		b.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		report, err := db.Prune(PruneOptions{DryRun: true})
		if err != nil {
			b.Fatal(err)
		}
		if report.RepositoriesPruned != 1 || report.ArtifactsPruned != 350 {
			b.Fatalf("unexpected report: %#v", report)
		}
	}
}

func seedPruneArtifact(t *testing.T, db *DB, repoID, artifactID, sourceID, now string) {
	t.Helper()
	revisionID := "rev_" + artifactID
	if err := db.InsertArtifactDirect(artifactID, repoID, "plan", "", artifactID, "draft", revisionID, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect(revisionID, artifactID, "hash", "body", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect(sourceID, artifactID, repoID, "markdown", artifactID+".md", artifactID+"|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}
	db.IndexArtifactFTS(artifactID, artifactID, "body", artifactID+".md")
}
