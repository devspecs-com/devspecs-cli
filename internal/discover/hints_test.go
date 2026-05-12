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

func TestFormatSuggestCommand(t *testing.T) {
	md := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "plans", SourceType: "markdown"})
	if !strings.Contains(md, "add-source markdown plans") {
		t.Fatalf("markdown: %q", md)
	}
	adr := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "docs/adr", SourceType: "adr"})
	if !strings.Contains(adr, "add-source adr docs/adr") {
		t.Fatalf("adr: %q", adr)
	}
	ospec := discover.FormatSuggestCommand(discover.HintCandidate{RelPath: "openspec", SourceType: "openspec"})
	if !strings.Contains(ospec, "add-source openspec openspec") {
		t.Fatalf("openspec: %q", ospec)
	}
}

func TestHintDisplayPath(t *testing.T) {
	if discover.HintDisplayPath("") != "" {
		t.Fatal()
	}
	if discover.HintDisplayPath("docs") != "docs/" {
		t.Fatalf("%q", discover.HintDisplayPath("docs"))
	}
	if discover.HintDisplayPath("docs/") != "docs/" {
		t.Fatalf("%q", discover.HintDisplayPath("docs/"))
	}
}

func TestScanHintCandidates_plansDirectory(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "plans" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("%#v", c)
	}
}

func TestScanHintCandidates_openspecBranch(t *testing.T) {
	tmp := t.TempDir()
	prop := filepath.Join(tmp, "openspec", "changes", "feat", "proposal.md")
	if err := os.MkdirAll(filepath.Dir(prop), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prop, []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "openspec" && h.SourceType == "openspec" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("%#v", c)
	}
}

func TestScanHintCandidates_speckitFeatureDir(t *testing.T) {
	tmp := t.TempDir()
	spec := filepath.Join(tmp, "specs", "myfeat", "spec.md")
	if err := os.MkdirAll(filepath.Dir(spec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "specs/myfeat" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("%#v", c)
	}
}

func TestScanHintCandidates_docsDense(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"a.plan.md", "b.plan.md"} {
		p := filepath.Join(tmp, "docs", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := discover.ScanHintCandidates(tmp, nil)
	var seen bool
	for _, h := range c {
		if h.RelPath == "docs" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("%#v", c)
	}
}

func TestScanHintCandidates_adrPaths(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{"docs/adr", "adr"} {
		if err := os.MkdirAll(filepath.Join(tmp, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	c := discover.ScanHintCandidates(tmp, nil)
	seen := map[string]bool{}
	for _, h := range c {
		if h.SourceType == "adr" {
			seen[h.RelPath] = true
		}
	}
	if !seen["docs/adr"] || !seen["adr"] {
		t.Fatalf("%#v", c)
	}
}
