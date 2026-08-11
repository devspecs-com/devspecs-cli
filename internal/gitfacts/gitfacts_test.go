package gitfacts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect_NonGitDirectory(t *testing.T) {
	facts, err := Collect(context.Background(), t.TempDir(), Options{MaxCommits: 5})
	require.NoError(t, err)

	assert.Contains(t, []string{ShapeNonGit, ShapeUnavailable}, facts.Diagnostics.HistoryShape)
	assert.Empty(t, facts.Commits)
	assert.Empty(t, facts.Files)

}

func TestGitErrorMeansNonGit(t *testing.T) {
	require.True(t, gitErrorMeansNonGit(assertErr("exit status 128: fatal: not a git repository")),
		"expected not-a-git error to be classified as non-git")
	require.False(t, gitErrorMeansNonGit(assertErr("exit status 128: fatal: detected dubious ownership in repository")),
		"expected dubious ownership to be unavailable, not non-git")

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
	require.NoError(t, err)

	require.Equal(t, ShapeSingleCommit, facts.Diagnostics.HistoryShape,
		"expected single commit shape, got %#v", facts.Diagnostics)
	require.Len(t, facts.Commits, 1,
		"expected 1 commit, got %d", len(facts.Commits))
	assert.NotEmpty(t, facts.Commits[0].BodyPreview)
	assert.Contains(t, facts.Commits[0].BodyPreview, "Fixes #42")
	require.Len(t, facts.Files, 2,
		"expected 2 files, got %d", len(facts.Files))

}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"git %v failed: %v\n%s", args, err, out)

}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
