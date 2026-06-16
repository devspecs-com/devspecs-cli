package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

func TestInit_CreatesGlobalDB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("global DB not created: %v", err)
	}
}

func TestInit_CreatesRepoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("repo config not created: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Initialized DevSpecs.") {
		t.Errorf("expected 'Initialized DevSpecs.' in output, got %q", output)
	}
	for _, want := range []string{
		"Next:",
		`ds task "goal"`,
		"Agent tooling:",
		"No Codex/Cursor/Claude/Windsurf project surfaces detected.",
		"Indexing:",
		"Not started automatically.",
		"ds scan",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected init output to contain %q, got %q", want, output)
		}
	}
}

func TestInit_DetectsAgentToolingSurfaces(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	for _, dir := range []string{filepath.Join(repoDir, ".cursor", "plans"), filepath.Join(repoDir, ".codex", "skills")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte("# Claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Codex (detected:",
		"Cursor (detected:",
		"Claude (detected:",
		"prepares:",
		"Generated files:",
		`/ds-task "goal"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in init output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Windsurf (") {
		t.Fatalf("did not expect undetected Windsurf to be selected by default:\n%s", out)
	}
	for _, rel := range []string{
		".agents/skills/ds-task/SKILL.md",
		".agents/skills/ds-apply/SKILL.md",
		".cursor/commands/ds-task.md",
		".cursor/commands/ds-apply.md",
		".claude/skills/ds-task/SKILL.md",
		".claude/skills/ds-apply/SKILL.md",
	} {
		assertInitFileExists(t, repoDir, rel)
	}
}

func TestInit_ToolFlagSelectsUndetectedTooling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--tool", "codex,windsurf"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Codex (not detected)", "Windsurf (not detected)", "Generated files:", `/ds-task "goal"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in init output:\n%s", want, out)
		}
	}
	for _, rel := range []string{
		".agents/skills/ds-task/SKILL.md",
		".agents/skills/ds-apply/SKILL.md",
		".windsurf/workflows/ds-task.md",
		".windsurf/workflows/ds-apply.md",
	} {
		assertInitFileExists(t, repoDir, rel)
	}
}

func TestInit_NoToolsSkipsAgentTooling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--no-tools"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "Skipped (--no-tools).") {
		t.Fatalf("expected no-tools output, got:\n%s", out)
	}
}

func TestInit_IndexBackgroundUsesStarter(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origStarter := startInitBackgroundScan
	var startedRoot string
	startInitBackgroundScan = func(repoRoot string) (int, error) {
		startedRoot = repoRoot
		return 1234, nil
	}
	defer func() { startInitBackgroundScan = origStarter }()

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--index", "background"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if startedRoot != repoDir {
		t.Fatalf("background scan root = %q, want %q", startedRoot, repoDir)
	}
	out := buf.String()
	for _, want := range []string{"Started background index refresh.", "pid 1234"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in init output:\n%s", want, out)
		}
	}
}

func TestInit_NoDestructiveRerun(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	// First init
	cmd1 := NewInitCmd()
	cmd1.SetOut(&bytes.Buffer{})
	if err := cmd1.Execute(); err != nil {
		t.Fatal(err)
	}

	// Write a marker into config to detect overwrite
	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	marker := []byte("# marker\nversion: 1\nsources: []\n")
	if err := os.WriteFile(configPath, marker, 0o644); err != nil {
		t.Fatal(err)
	}

	// Second init without --force may still add explicitly requested tooling.
	cmd2 := NewInitCmd()
	cmd2.SetArgs([]string{"--tool", "cursor"})
	buf := &bytes.Buffer{}
	cmd2.SetOut(buf)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}

	// Config should NOT be overwritten
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# marker") {
		t.Error("config was overwritten without --force")
	}

	output := buf.String()
	if !strings.Contains(output, "already initialized") {
		t.Errorf("expected 'already initialized' message, got %q", output)
	}
	if !strings.Contains(output, "Generated files:") {
		t.Errorf("expected generated tooling files in output, got %q", output)
	}
	assertInitFileExists(t, repoDir, ".cursor/commands/ds-task.md")
}

func TestInit_DiscoveryMergesDenseDocs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, "docs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "a.plan.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "b.spec.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadRepoConfig(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	paths := markdownPathsFrom(t, cfg)
	if !sliceContains(paths, "docs") {
		t.Fatalf("expected bare docs/ merged when dense, got %v", paths)
	}
}

func TestInit_SparseDocsPrintsSuggestion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "README.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Suggestion:") {
		t.Fatalf("expected Suggestion: in init output, got:\n%s", out)
	}
	if !strings.Contains(out, "docs/") {
		t.Fatalf("expected docs/ hint in init output, got:\n%s", out)
	}
}

func TestInit_NoDetect_SkipsDenseDocsMerge(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, "docs", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "a.plan.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "b.spec.md"), []byte("#\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--no-detect"})
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadRepoConfig(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	paths := markdownPathsFrom(t, cfg)
	if sliceContains(paths, "docs") {
		t.Fatalf("did not expect bare docs/ with --no-detect, got %v", paths)
	}
}

func TestInit_EmptyStdinNonBlocking(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func markdownPathsFrom(t *testing.T, cfg *config.RepoConfig) []string {
	t.Helper()
	if cfg == nil {
		t.Fatal("nil config")
	}
	for _, s := range cfg.Sources {
		if s.Type == "markdown" {
			if s.Path != "" {
				return append([]string{s.Path}, s.Paths...)
			}
			return append([]string(nil), s.Paths...)
		}
	}
	t.Fatal("no markdown source")
	return nil
}

func sliceContains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func assertInitFileExists(t *testing.T, root, relPath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath))); err != nil {
		t.Fatalf("expected %s to exist: %v", relPath, err)
	}
}
