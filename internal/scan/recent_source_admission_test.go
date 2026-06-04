package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
	if len(got) != 1 {
		t.Fatalf("got %d candidates: %#v", len(got), got)
	}
	if got[0].RelPath != "server/router/api.go" {
		t.Fatalf("candidate path = %q", got[0].RelPath)
	}
	if got[0].Metadata["admission_reason"] != recentSourceAdmissionReason {
		t.Fatalf("metadata = %#v", got[0].Metadata)
	}
}

func writeRecentSourceTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
