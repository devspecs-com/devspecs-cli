package store

import (
	"os"
	"path/filepath"
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
	report, err := db.Prune(PruneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RepositoriesPruned != 1 || report.ArtifactsPruned != 1 {
		t.Fatalf("rootless duplicate was not pruned: %#v", report)
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
	report, err := db.Prune(PruneOptions{Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Vacuumed || report.RepositoriesPruned != 1 {
		t.Fatalf("vacuum prune report = %#v", report)
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
