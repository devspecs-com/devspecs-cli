package initflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/profiles"
)

// ---------------------------------------------------------------------------
// MergeSelectedProfiles
// ---------------------------------------------------------------------------

func TestMergeSelectedProfiles_cursorAddsPathsAndRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"cursor"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := markdownSource(out)
	if md == nil {
		t.Fatal("expected markdown source")
	}
	foundPlans := false
	for _, p := range md.Paths {
		if p == ".cursor/plans" {
			foundPlans = true
			break
		}
	}
	if !foundPlans {
		t.Fatalf("paths: %v", md.Paths)
	}
	var sawPlanRule bool
	for _, r := range md.Rules {
		if r.Match == "*.plan.md" && r.Kind == config.KindPlan {
			sawPlanRule = true
			break
		}
	}
	if !sawPlanRule {
		t.Fatalf("rules: %+v", md.Rules)
	}
}

func TestMergeSelectedProfiles_openspecSetsPath(t *testing.T) {
	base := config.DefaultRepoConfig()
	base.Sources = nil
	out, err := MergeSelectedProfiles(base, []string{"openspec"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var osrc *config.SourceConfig
	for i := range out.Sources {
		if out.Sources[i].Type == "openspec" {
			osrc = &out.Sources[i]
			break
		}
	}
	if osrc == nil || osrc.Path != "openspec" {
		t.Fatalf("openspec source: %+v", osrc)
	}
}

func TestMergeSelectedProfiles_customPathsAndRules(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "plans", "01_step")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	base := config.DefaultRepoConfig()
	rules := []config.SourceRule{{Match: "README.md", Kind: config.KindPlan}}
	out, err := MergeSelectedProfiles(base, nil, []string{"plans"}, rules)
	if err != nil {
		t.Fatal(err)
	}
	md := markdownSource(out)
	if md == nil {
		t.Fatal("expected markdown")
	}
	found := false
	for _, p := range md.Paths {
		if p == "plans" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("paths %v", md.Paths)
	}
	if len(md.Rules) == 0 {
		t.Fatal("expected merged rules")
	}
}

func TestMergeSelectedProfiles_emptySelectionPreservesBase(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sources) != len(base.Sources) {
		t.Fatalf("sources len %d vs %d", len(out.Sources), len(base.Sources))
	}
}

func TestProfilesOpenspecIDMergeIntegration(t *testing.T) {
	if _, ok := profiles.ByID("openspec"); !ok {
		t.Fatal("registry missing openspec")
	}
}

// ---------------------------------------------------------------------------
// DetectPatterns
// ---------------------------------------------------------------------------

func TestDetectPatterns_readme(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "v2", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pats := DetectPatterns(root, "v2/plans")
	if len(pats) == 0 {
		t.Fatal("expected patterns")
	}
	var sawReadme bool
	for _, p := range pats {
		if strings.Contains(p.Match, "README") {
			sawReadme = true
			if p.FileCount != 1 {
				t.Errorf("README.md FileCount: want 1, got %d", p.FileCount)
			}
			if p.DefaultKind != config.KindPlan {
				t.Errorf("README.md DefaultKind: want plan, got %s", p.DefaultKind)
			}
			break
		}
	}
	if !sawReadme {
		t.Fatalf("patterns: %+v", pats)
	}
}

func TestDetectPatterns_numberedAndNested(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"02_FOO.md", "03_BAR.md", "04_BAZ.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# "+n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "01-setup")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"README.md", "01-first.md", "02-second.md"} {
		if err := os.WriteFile(filepath.Join(sub, n), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# roadmap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pats := DetectPatterns(root, "plans")
	counts := make(map[string]int)
	kinds := make(map[string]string)
	for _, p := range pats {
		counts[p.Match] = p.FileCount
		kinds[p.Match] = p.DefaultKind
	}

	if counts["[0-9][0-9]_*.md"] != 3 {
		t.Errorf("[0-9][0-9]_*.md: want 3, got %d", counts["[0-9][0-9]_*.md"])
	}
	if counts["*/README.md"] != 1 {
		t.Errorf("*/README.md: want 1, got %d", counts["*/README.md"])
	}
	if counts["*/[0-9][0-9]-*.md"] != 2 {
		t.Errorf("*/[0-9][0-9]-*.md: want 2, got %d", counts["*/[0-9][0-9]-*.md"])
	}
	if counts["ROADMAP.md"] != 1 {
		t.Errorf("ROADMAP.md individual: want 1, got %d", counts["ROADMAP.md"])
	}
	if kinds["ROADMAP.md"] != config.KindPlan {
		t.Errorf("ROADMAP.md kind: want plan, got %s", kinds["ROADMAP.md"])
	}
}

func TestDetectPatterns_threeDigitDecisions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"001-auth.md", "002-storage.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pats := DetectPatterns(root, "decisions")
	var found *DetectedPattern
	for i := range pats {
		if pats[i].Match == "[0-9][0-9][0-9]-*.md" {
			found = &pats[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected [0-9][0-9][0-9]-*.md, got %+v", pats)
	}
	if found.FileCount != 2 {
		t.Errorf("FileCount: want 2, got %d", found.FileCount)
	}
	if found.DefaultKind != config.KindDecision {
		t.Errorf("DefaultKind: want decision, got %s", found.DefaultKind)
	}
}

func TestDetectPatterns_individualFileKindInference(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"ROADMAP.md", "spec-auth.md", "design-system.md", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pats := DetectPatterns(root, "docs")
	kinds := make(map[string]string)
	for _, p := range pats {
		kinds[p.Match] = p.DefaultKind
	}

	if kinds["ROADMAP.md"] != config.KindPlan {
		t.Errorf("ROADMAP.md: want plan, got %s", kinds["ROADMAP.md"])
	}
	if kinds["spec-auth.md"] != config.KindSpec {
		t.Errorf("spec-auth.md: want spec, got %s", kinds["spec-auth.md"])
	}
	if kinds["design-system.md"] != config.KindDesign {
		t.Errorf("design-system.md: want design, got %s", kinds["design-system.md"])
	}
	if kinds["notes.md"] != config.KindMarkdownArtifact {
		t.Errorf("notes.md: want markdown_artifact, got %s", kinds["notes.md"])
	}
}

func TestDetectPatterns_emptyDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pats := DetectPatterns(root, "empty")
	if len(pats) != 0 {
		t.Errorf("expected nil, got %+v", pats)
	}
}

func TestInferDirKind(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"decisions", config.KindDecision},
		{"docs/adr", config.KindDecision},
		{"specs", config.KindSpec},
		{"v2/design", config.KindDesign},
		{"requirements", config.KindRequirements},
		{"v2/plans", config.KindPlan},
		{".", config.KindPlan},
	}
	for _, tc := range cases {
		got := inferDirKind(tc.path)
		if got != tc.want {
			t.Errorf("inferDirKind(%q) = %s, want %s", tc.path, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func markdownSource(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "markdown" {
			return &cfg.Sources[i]
		}
	}
	return nil
}
