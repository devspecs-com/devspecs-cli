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

func TestMergeSelectedProfiles_openspecUpdatesExistingPath(t *testing.T) {
	base := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{Type: "openspec", Path: "wrong"},
		},
	}
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
		t.Fatalf("openspec path: %+v", osrc)
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

func TestMergeSelectedProfiles_nilBaseUsesDefaults(t *testing.T) {
	out, err := MergeSelectedProfiles(nil, []string{"cursor"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Version != 1 {
		t.Fatalf("version %d", out.Version)
	}
	md := markdownSource(out)
	if md == nil || len(md.Paths) == 0 {
		t.Fatalf("markdown %+v", md)
	}
}

// ---------------------------------------------------------------------------
// Agent tooling file generation
// ---------------------------------------------------------------------------

func TestGenerateAgentToolFilesCreatesExpectedFiles(t *testing.T) {
	root := t.TempDir()
	tools, err := SelectAgentTools(root, []string{"all"}, false)
	if err != nil {
		t.Fatal(err)
	}
	files, err := GenerateAgentToolFiles(root, tools, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("generated %d files, want 8: %+v", len(files), files)
	}
	want := map[string]string{
		".agents/skills/ds-task/SKILL.md":  "$ds-task",
		".agents/skills/ds-apply/SKILL.md": "$ds-apply",
		".cursor/commands/ds-task.md":      "/ds-task",
		".cursor/commands/ds-apply.md":     "/ds-apply",
		".claude/skills/ds-task/SKILL.md":  "/ds-task",
		".claude/skills/ds-apply/SKILL.md": "/ds-apply",
		".windsurf/workflows/ds-task.md":   "/ds-task",
		".windsurf/workflows/ds-apply.md":  "/ds-apply",
	}
	for _, file := range files {
		if file.Status != "created" {
			t.Fatalf("%s status = %q, want created", file.Path, file.Status)
		}
		if got, ok := want[file.Path]; !ok {
			t.Fatalf("unexpected generated file: %+v", file)
		} else if file.Invocation != got {
			t.Fatalf("%s invocation = %q, want %q", file.Path, file.Invocation, got)
		}
		assertGeneratedFileContains(t, root, file.Path, "DevSpecs")
	}

	taskSkill := readGeneratedFile(t, root, ".agents/skills/ds-task/SKILL.md")
	for _, wantText := range []string{"name: ds-task", "ds task", "ds apply next", "ds recent", "ds find", "Work exactly one slice", "decision gate"} {
		if !strings.Contains(taskSkill, wantText) {
			t.Fatalf("codex task skill missing %q:\n%s", wantText, taskSkill)
		}
	}
	applyWorkflow := readGeneratedFile(t, root, ".windsurf/workflows/ds-apply.md")
	for _, wantText := range []string{"ds apply", "ds apply next", "ds recent", "ds find", "Stop after the decision gate"} {
		if !strings.Contains(applyWorkflow, wantText) {
			t.Fatalf("windsurf apply workflow missing %q:\n%s", wantText, applyWorkflow)
		}
	}
	if strings.Contains(applyWorkflow, "not available yet") || strings.Contains(applyWorkflow, "ds task next") {
		t.Fatalf("apply workflow should use the real ds apply surface without fallback language:\n%s", applyWorkflow)
	}
	for _, rel := range []string{
		".agents/skills/ds-apply/SKILL.md",
		".cursor/commands/ds-apply.md",
		".claude/skills/ds-apply/SKILL.md",
		".windsurf/workflows/ds-apply.md",
	} {
		body := readGeneratedFile(t, root, rel)
		for _, wantText := range []string{"ds apply", "promote", "improve", "rework", "rollback", "block"} {
			if !strings.Contains(body, wantText) {
				t.Fatalf("%s missing %q:\n%s", rel, wantText, body)
			}
		}
	}
	claudeSkill := readGeneratedFile(t, root, ".claude/skills/ds-task/SKILL.md")
	if strings.Contains(claudeSkill, "allowed-tools") {
		t.Fatalf("claude skill should not include speculative allowed-tools metadata:\n%s", claudeSkill)
	}
}

func TestGenerateAgentToolFilesPreservesExistingWithoutForce(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, ".cursor", "commands", "ds-task.md")
	if err := os.MkdirAll(filepath.Dir(custom), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("# custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tools, err := SelectAgentTools(root, []string{"cursor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	files, err := GenerateAgentToolFiles(root, tools, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := generatedStatus(files, ".cursor/commands/ds-task.md"); got != "skipped-existing" {
		t.Fatalf("status = %q, want skipped-existing; files=%+v", got, files)
	}
	if data := readGeneratedFile(t, root, ".cursor/commands/ds-task.md"); data != "# custom\n" {
		t.Fatalf("existing command was overwritten without force:\n%s", data)
	}
	if got := generatedStatus(files, ".cursor/commands/ds-apply.md"); got != "created" {
		t.Fatalf("apply status = %q, want created; files=%+v", got, files)
	}
}

func TestGenerateAgentToolFilesForceOverwritesExisting(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, ".cursor", "commands", "ds-task.md")
	if err := os.MkdirAll(filepath.Dir(custom), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(custom, []byte("# custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tools, err := SelectAgentTools(root, []string{"cursor"}, false)
	if err != nil {
		t.Fatal(err)
	}
	files, err := GenerateAgentToolFiles(root, tools, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := generatedStatus(files, ".cursor/commands/ds-task.md"); got != "overwritten" {
		t.Fatalf("status = %q, want overwritten; files=%+v", got, files)
	}
	assertGeneratedFileContains(t, root, ".cursor/commands/ds-task.md", "Work exactly one slice")
}

func generatedStatus(files []AgentToolFile, relPath string) string {
	for _, file := range files {
		if file.Path == relPath {
			return file.Status
		}
	}
	return ""
}

func assertGeneratedFileContains(t *testing.T, root, relPath, want string) {
	t.Helper()
	data := readGeneratedFile(t, root, relPath)
	if !strings.Contains(data, want) {
		t.Fatalf("%s missing %q:\n%s", relPath, want, data)
	}
}

func readGeneratedFile(t *testing.T, root, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestMergeSelectedProfiles_adrAddsPaths(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"adr"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ad := adrSource(out)
	if ad == nil {
		t.Fatal("expected adr source")
	}
	found := false
	for _, p := range ad.Paths {
		if p == "docs/adr" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("paths %v", ad.Paths)
	}
}

func TestMergeSelectedProfiles_invalidCustomRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	_, err := MergeSelectedProfiles(base, nil, []string{"plans"}, []config.SourceRule{
		{Match: "*.md", Kind: "invalid_kind"},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMergeSelectedProfiles_unknownProfileIDSkipped(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"cursor", "___unknown___"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := markdownSource(out)
	if md == nil {
		t.Fatal("expected markdown")
	}
	var sawCursor bool
	for _, p := range md.Paths {
		if p == ".cursor/plans" {
			sawCursor = true
			break
		}
	}
	if !sawCursor {
		t.Fatalf("paths %v", md.Paths)
	}
}

func adrSource(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "adr" {
			return &cfg.Sources[i]
		}
	}
	return nil
}

func TestMergeSelectedProfiles_speckitRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"speckit"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := markdownSource(out)
	if md == nil {
		t.Fatal("expected markdown")
	}
	var saw bool
	for _, r := range md.Rules {
		if r.Match == "*/spec.md" && r.Kind == config.KindSpec {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("rules %+v", md.Rules)
	}
}

func TestMergeSelectedProfiles_bmadPRDRule(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"bmad"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	md := markdownSource(out)
	var saw bool
	for _, r := range md.Rules {
		if r.Kind == config.KindRequirements && r.Subtype == config.SubtypePRD {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("rules %+v", md.Rules)
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

func TestDetectPatterns_missingSourceDir(t *testing.T) {
	root := t.TempDir()
	if p := DetectPatterns(root, "no/such/dir"); len(p) != 0 {
		t.Fatalf("got %+v", p)
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
