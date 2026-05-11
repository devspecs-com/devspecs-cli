package discover_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestRun_gitignore_hides_candidate(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(".cursor/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".cursor", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	res := discover.Run(tmp, m)
	for _, p := range res.MergeMarkdown {
		if p == ".cursor/plans" {
			t.Fatalf("did not expect .cursor/plans when .cursor is ignored, got %#v", res.MergeMarkdown)
		}
	}
}

func TestRun_sparse_docs_not_merged(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "docs", "README.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := discover.Run(tmp, nil)
	for _, p := range res.MergeMarkdown {
		if p == "docs" {
			t.Fatalf("sparse docs/ should not merge bare docs, got %#v", res.MergeMarkdown)
		}
	}
}

func TestRun_docs_dense_merges(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "docs", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docs/one.spec.md", "docs/two.plan.md"} {
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(name)), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := discover.Run(tmp, nil)
	found := false
	for _, p := range res.MergeMarkdown {
		if p == "docs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected docs in merge list, got %#v", res.MergeMarkdown)
	}
}
