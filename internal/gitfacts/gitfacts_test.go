package gitfacts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGitErrorMeansNonGit(t *testing.T) {
	if !gitErrorMeansNonGit(assertErr("exit status 128: fatal: not a git repository")) {
		t.Fatal("expected not-a-git error to be classified as non-git")
	}
	if gitErrorMeansNonGit(assertErr("exit status 128: fatal: detected dubious ownership in repository")) {
		t.Fatal("expected dubious ownership to be unavailable, not non-git")
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
	runGit(t, root, "commit", "-m", "add auth docs", "-m", "Fixes #42\n\nAuth docs PR title.")

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
	if facts.Commits[0].BodyPreview == "" || !strings.Contains(facts.Commits[0].BodyPreview, "Fixes #42") {
		t.Fatalf("expected commit body preview, got %#v", facts.Commits[0])
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

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
