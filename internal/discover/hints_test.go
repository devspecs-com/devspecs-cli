package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestScanHintCandidates_ExcludesGitignoredPaths(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(".cursor/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".cursor", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".cursor", "plans", "x.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	c := discover.ScanHintCandidates(tmp, m)
	for _, h := range c {
		if h.RelPath == ".cursor/plans" || strings.HasPrefix(h.RelPath, ".cursor/") {
			t.Fatalf("did not expect gitignored .cursor in hints, got %#v", c)
		}
	}
}
