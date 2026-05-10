package freshness

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupDB(t *testing.T) *store.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestCheck_NilIfNoRepo(t *testing.T) {
	db := setupDB(t)
	result := Check(db, t.TempDir())
	if result != nil {
		t.Errorf("expected nil for uninitialized repo, got %+v", result)
	}
}

func TestCheck_GitFresh(t *testing.T) {
	dir := initGitRepo(t)
	db := setupDB(t)

	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', ?, ?, ?)", dir, now, now)

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, _ := cmd.Output()
	head := string(out[:len(out)-1])
	db.UpdateScanMeta("r1", head, "", now)

	status := Check(db, dir)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Stale {
		t.Errorf("expected fresh, got stale: %s", status.Reason)
	}
}

func TestCheck_GitStale(t *testing.T) {
	dir := initGitRepo(t)
	db := setupDB(t)

	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', ?, ?, ?)", dir, now, now)
	db.UpdateScanMeta("r1", "oldcommitsha", "", now)

	status := Check(db, dir)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if !status.Stale {
		t.Error("expected stale")
	}
	if status.Reason != "git HEAD changed" {
		t.Errorf("unexpected reason: %s", status.Reason)
	}
}

func TestCheck_MtimeFresh(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "test.md"), []byte("# test"), 0o644)

	db := setupDB(t)
	now := time.Now().Add(1 * time.Second).UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', ?, ?, ?)", dir, now, now)
	db.UpdateScanMeta("r1", "", "", now)

	status := Check(db, dir)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Stale {
		t.Errorf("expected fresh, got stale: %s", status.Reason)
	}
}

func TestCheck_MtimeStale(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	os.MkdirAll(specsDir, 0o755)

	db := setupDB(t)
	past := time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', ?, ?, ?)", dir, past, past)
	db.UpdateScanMeta("r1", "", "", past)

	// Write a file after the scan timestamp
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(specsDir, "new.md"), []byte("# new"), 0o644)

	status := Check(db, dir)
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if !status.Stale {
		t.Error("expected stale due to mtime")
	}
}
