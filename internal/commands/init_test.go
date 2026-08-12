package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_CreatesGlobalDB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	{
		_, err := os.Stat(dbPath)
		require.NoError(t, err,
			"global DB not created: %v", err)
	}

}

func TestInit_CreatesRepoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	{
		_, err := os.Stat(configPath)
		require.NoError(t, err,
			"repo config not created: %v", err)
	}

	output := buf.String()
	assert.Contains(t, output, "Initialized DevSpecs.",
		"expected 'Initialized DevSpecs.' in output, got %q", output)

	assert.Contains(t, output, "Next:",
		"expected init output to contain %q, got %q", "Next:", output)
	assert.Contains(t, output, `ds task "goal"`,
		"expected init output to contain %q, got %q", `ds task "goal"`, output)
	assert.Contains(t, output, "Agent tooling:",
		"expected init output to contain %q, got %q", "Agent tooling:", output)
	assert.Contains(t, output, "No Codex/Cursor/Claude/Windsurf project surfaces detected.",
		"expected init output to contain %q, got %q", "No Codex/Cursor/Claude/Windsurf project surfaces detected.", output)
	assert.Contains(t, output, "Indexing:",
		"expected init output to contain %q, got %q", "Indexing:", output)
	assert.Contains(t, output, "Not started automatically.",
		"expected init output to contain %q, got %q", "Not started automatically.", output)
	assert.Contains(t, output, "ds scan",
		"expected init output to contain %q, got %q", "ds scan", output)

}

func TestInit_DetectsAgentToolingSurfaces(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	{
		dir := filepath.Join(repoDir, ".cursor", "plans")

		{
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)
		}

	}
	{
		dir := filepath.Join(repoDir, ".codex", "skills")

		{
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)
		}

	}

	{
		err := os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte("# Claude\n"), 0o644)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Codex (detected:",
		"expected %q in init output:\n%s", "Codex (detected:", out)
	assert.Contains(t, out, "Cursor (detected:",
		"expected %q in init output:\n%s", "Cursor (detected:", out)
	assert.Contains(t, out, "Claude (detected:",
		"expected %q in init output:\n%s", "Claude (detected:", out)
	assert.Contains(t, out, "prepares:",
		"expected %q in init output:\n%s", "prepares:", out)
	assert.Contains(t, out, "Generated files:",
		"expected %q in init output:\n%s", "Generated files:", out)
	assert.Contains(t, out, `/ds-task "goal"`,
		"expected %q in init output:\n%s", `/ds-task "goal"`, out)

	assert.NotContains(t, out, "Windsurf (",
		"did not expect undetected Windsurf to be selected by default:\n%s", out)

	{
		rel := ".agents/skills/ds-task/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".agents/skills/ds-apply/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".cursor/commands/ds-task.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".cursor/commands/ds-apply.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".claude/skills/ds-task/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".claude/skills/ds-apply/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}

}

func TestInit_ToolFlagSelectsUndetectedTooling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--tool", "codex,windsurf"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Codex (not detected)",
		"expected %q in init output:\n%s", "Codex (not detected)", out)
	assert.Contains(t, out, "Windsurf (not detected)",
		"expected %q in init output:\n%s", "Windsurf (not detected)", out)
	assert.Contains(t, out, "Generated files:",
		"expected %q in init output:\n%s", "Generated files:", out)
	assert.Contains(t, out, `/ds-task "goal"`,
		"expected %q in init output:\n%s", `/ds-task "goal"`, out)

	{
		rel := ".agents/skills/ds-task/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".agents/skills/ds-apply/SKILL.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".windsurf/workflows/ds-task.md"

		assertInitFileExists(t, repoDir, rel)

	}
	{
		rel := ".windsurf/workflows/ds-apply.md"

		assertInitFileExists(t, repoDir, rel)

	}

}

func TestInit_NoToolsSkipsAgentTooling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, ".cursor"), 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--no-tools"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	{

		out := buf.String()
		assert.Contains(t, out, "Skipped (--no-tools).",
			"expected no-tools output, got:\n%s", out)
	}

}

func TestInit_IndexBackgroundUsesStarter(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origStarter := startInitBackgroundScan
	var startedRoot string
	startInitBackgroundScan = func(repoRoot string) (int, error) {
		startedRoot = repoRoot
		return 1234, nil
	}
	defer func() { startInitBackgroundScan = origStarter }()

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--index", "background"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, repoDir, startedRoot,
		"background scan root = %q, want %q", startedRoot, repoDir)

	out := buf.String()
	assert.Contains(t, out, "Started background index refresh.",
		"expected %q in init output:\n%s", "Started background index refresh.", out)
	assert.Contains(t, out, "pid 1234",
		"expected %q in init output:\n%s", "pid 1234", out)

}

func TestInit_WhenAlreadyInitialized_PreservesConfigAndAddsRequestedTooling(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755))
	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	marker := []byte("# marker\nversion: 1\nsources: []\n")
	require.NoError(t, os.WriteFile(configPath, marker, 0o644))

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	require.NoError(t, os.Chdir(repoDir))

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--tool", "cursor"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# marker",
		"config was overwritten without --force")

	output := buf.String()
	assert.Contains(t, output, "already initialized",
		"expected 'already initialized' message, got %q", output)
	assert.Contains(t, output, "Generated files:",
		"expected generated tooling files in output, got %q", output)

	assertInitFileExists(t, repoDir, ".cursor/commands/ds-task.md")
}

func TestInit_DiscoveryMergesDenseDocs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, "docs", "x"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "a.plan.md"), []byte("#\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "b.spec.md"), []byte("#\n"), 0o644)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	cfg, err := config.LoadRepoConfig(repoDir)
	require.NoError(t, err)

	paths := markdownPathsFrom(t, cfg)
	assert.True(t, sliceContains(paths, "docs"),
		"expected bare docs/ merged when dense, got %v", paths)

}

func TestInit_SparseDocsPrintsSuggestion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, "docs"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "docs", "README.md"), []byte("#\n"), 0o644)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Suggestion:",
		"expected Suggestion: in init output, got:\n%s", out)
	assert.Contains(t, out, "docs/",
		"expected docs/ hint in init output, got:\n%s", out)

}

func TestInit_NoDetect_SkipsDenseDocsMerge(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, "docs", "x"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "a.plan.md"), []byte("#\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "docs", "x", "b.spec.md"), []byte("#\n"), 0o644)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetArgs([]string{"--no-detect"})
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	cfg, err := config.LoadRepoConfig(repoDir)
	require.NoError(t, err)

	paths := markdownPathsFrom(t, cfg)
	assert.False(t, sliceContains(paths, "docs"),
		"did not expect bare docs/ with --no-detect, got %v", paths)

}

func TestInit_EmptyStdinNonBlocking(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	defer os.Chdir(origWd)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	cmd := NewInitCmd()
	cmd.SetIn(bytes.NewReader(nil))
	cmd.SetOut(&bytes.Buffer{})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

}

func markdownPathsFrom(t *testing.T, cfg *config.RepoConfig) []string {
	t.Helper()
	require.NotNil(t, cfg, "config")

	var paths []string
	for _, s := range cfg.Sources {
		if s.Type == "markdown" {
			if s.Path != "" {
				paths = append([]string{s.Path}, s.Paths...)
				break
			}
			paths = append([]string(nil), s.Paths...)
			break
		}
	}
	require.NotEmpty(t, paths, "markdown source paths")

	return paths
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
	{
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relPath)))
		require.NoError(t, err,
			"expected %s to exist: %v", relPath, err)
	}

}
