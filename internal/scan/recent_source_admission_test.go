package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRecentGitSourceContextCandidates(t *testing.T) {
	root := t.TempDir()
	runGitCommand(t, root, "init", "-b", "main")
	runGitCommand(t, root, "config", "user.email", "devspecs@example.test")
	runGitCommand(t, root, "config", "user.name", "Devspecs")

	writeRecentSourceTestFile(t, root, "server/router/api.go", "package router\n\nfunc Handle() {}\n")
	writeRecentSourceTestFile(t, root, "server/router/api_test.go", "package router\n\nfunc TestHandle() {}\n")
	runGitCommand(t, root, "add", ".")
	runGitCommand(t, root, "commit", "-m", "add router behavior")

	got := buildRecentGitSourceContextCandidates(context.Background(), root, nil, RunOptions{GitMaxCommits: 20, GitMaxFilesPerCommit: 20})
	require.Len(t, got, 1,
		"got %d candidates: %#v", len(got), got)
	require.Equal(t, "server/router/api.go", got[0].RelPath,
		"candidate path = %q", got[0].RelPath)
	require.Equal(t, recentSourceAdmissionReason, got[0].Metadata["admission_reason"],
		"metadata = %#v", got[0].Metadata)

}

func writeRecentSourceTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
