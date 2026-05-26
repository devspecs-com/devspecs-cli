package gitfacts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCollect_NonGitDirectory(t *testing.T) {
	facts, err := Collect(context.Background(), t.TempDir(), Options{MaxCommits: 5})
	if err != nil {
		t.Fatal(err)
	}
	if facts.Diagnostics.HistoryShape != ShapeNonGit && facts.Diagnostics.HistoryShape != ShapeUnavailable {
		t.Fatalf("expected non-git or unavailable shape, got %#v", facts.Diagnostics)
	}
	if len(facts.Commits) != 0 || len(facts.Files) != 0 {
		t.Fatalf("expected no facts, got commits=%d files=%d", len(facts.Commits), len(facts.Files))
	}
}

func TestCollect_LocalGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "checkout", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	mustWriteFile(t, filepath.Join(root, "docs", "auth.md"), "# Auth\n")
	mustWriteFile(t, filepath.Join(root, "docs", "auth-tests.md"), "# Auth Tests\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "add auth docs")

	facts, err := Collect(context.Background(), root, Options{MaxCommits: 10, MaxFilesPerCommit: 8})
	if err != nil {
		t.Fatal(err)
	}
	if facts.Diagnostics.HistoryShape != ShapeSingleCommit {
		t.Fatalf("expected single commit shape, got %#v", facts.Diagnostics)
	}
	if len(facts.Commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(facts.Commits))
	}
	if len(facts.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(facts.Files))
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
