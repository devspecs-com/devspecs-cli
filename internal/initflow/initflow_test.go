package initflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/profiles"
)

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
	base.Sources = nil // minimal
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
			break
		}
	}
	if !sawReadme {
		t.Fatalf("patterns: %+v", pats)
	}
}

func markdownSource(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "markdown" {
			return &cfg.Sources[i]
		}
	}
	return nil
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
