package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestPlainDocsWorthIndexing_RespectsMaxDirs(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "docs", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.spec.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.plan.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	out := &Result{}
	if plainDocsWorthIndexing(tmp, m, out, 1, 400) {
		t.Fatal("expected false when maxDirs stops walk before nested hits")
	}
	out = &Result{}
	if !plainDocsWorthIndexing(tmp, m, out, 10, 400) {
		t.Fatal("expected true when walk can reach two spec-like files")
	}
}
