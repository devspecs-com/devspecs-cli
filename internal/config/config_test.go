package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHomeDir_Default(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", "")
	dir, err := HomeDir()
	require.NoError(t, err)

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".devspecs")
	assert.Equal(t, expected, dir,
		"expected %q, got %q", expected, dir)

}

func TestHomeDir_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	dir, err := HomeDir()
	require.NoError(t, err)

	assert.Equal(t, tmp, dir,
		"expected %q, got %q", tmp, dir)

}

func TestDBPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	p, err := DBPath()
	require.NoError(t, err)

	expected := filepath.Join(tmp, "devspecs.db")
	assert.Equal(t, expected, p,
		"expected %q, got %q", expected, p)

}

func TestLoadRepoConfig_Missing(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := LoadRepoConfig(tmp)
	require.NoError(t, err)

	assert.Nil(t, cfg,
		"expected nil config for missing file")

}

func TestLoadRepoConfig_Valid(t *testing.T) {
	tmp := t.TempDir()
	writeRepoConfigFixture(t, tmp, `version: 1
sources:
  - type: openspec
    path: openspec
  - type: adr
    paths: [docs/adr, adr]
  - type: markdown
    paths: [docs/plans]
  - type: source_context
`)

	loaded, err := LoadRepoConfig(tmp)

	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, 1, loaded.Version)
	require.Len(t, loaded.Sources, 4)
}

func TestWriteRepoConfig_WithArtifactFlags_WritesFlags(t *testing.T) {
	tmp := t.TempDir()
	cfg := WithCodeCommentArtifacts(WithTestCaseArtifacts(DefaultRepoConfig(), true), true)

	err := WriteRepoConfig(tmp, cfg)

	require.NoError(t, err)
	data, err := os.ReadFile(RepoConfigPath(tmp))
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_cases: true")
	assert.Contains(t, string(data), "code_comments: true")
}

func TestLoadRepoConfig_WithArtifactFlags_LoadsFlags(t *testing.T) {
	tmp := t.TempDir()
	writeRepoConfigFixture(t, tmp, `version: 1
sources: []
artifacts:
  test_cases: true
  code_comments: true
`)

	loaded, err := LoadRepoConfig(tmp)

	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.True(t, loaded.TestCaseArtifactsEnabled(false))
	assert.True(t, loaded.CodeCommentArtifactsEnabled(false))
}

func TestTestCaseArtifactsEnabled_WithLegacyExperiment_ReturnsTrue(t *testing.T) {
	legacy := &RepoConfig{Experiments: ExperimentConfig{TestCaseArtifacts: boolPtr(true)}}

	actual := legacy.TestCaseArtifactsEnabled(false)

	assert.True(t, actual)
}

func TestLoadRepoConfig_Invalid(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(":::invalid"), 0o644))

	_, err := LoadRepoConfig(tmp)
	assert.Error(t, err,
		"expected error for invalid YAML")

}

func TestWriteRepoConfig_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultRepoConfig()

	err := WriteRepoConfig(tmp, cfg)

	require.NoError(t, err)
	path := RepoConfigPath(tmp)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestRepoConfigPath(t *testing.T) {
	got := RepoConfigPath("/my/repo")
	want := filepath.Join("/my/repo", ".devspecs", "config.yaml")
	assert.Equal(t, want, got,
		"want %q, got %q", want, got)

}

func TestDefaultRepoConfig_Structure(t *testing.T) {
	cfg := DefaultRepoConfig()

	assert.Equal(t, 1, cfg.Version)
	require.Len(t, cfg.Sources, 4)
	assert.Equal(t, "openspec", cfg.Sources[0].Type)
	assert.Equal(t, "adr", cfg.Sources[1].Type)
	assert.Equal(t, "markdown", cfg.Sources[2].Type)
	assert.Equal(t, "source_context", cfg.Sources[3].Type)
	assert.Contains(t, cfg.Sources[2].Paths, ".claude/notes")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/specs")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/plans")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/prd")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/rfcs")
	assert.Contains(t, cfg.Sources[2].Paths, "rfcs")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/design")
	assert.Contains(t, cfg.Sources[2].Paths, "docs/technical")
}

func TestDBPath_WithEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	p, err := DBPath()
	require.NoError(t, err)

	assert.Equal(t, "devspecs.db", filepath.Base(p),
		"expected devspecs.db, got %q", filepath.Base(p))
	assert.Equal(t, tmp, filepath.Dir(p),
		"expected dir %q, got %q", tmp, filepath.Dir(p))

}

func TestWriteRepoConfig_OverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	writeRepoConfigFixture(t, tmp, "version: 1\nsources:\n  - type: a\n    path: x\n")
	cfg := &RepoConfig{Version: 1, Sources: []SourceConfig{{Type: "b", Path: "y"}}}

	err := WriteRepoConfig(tmp, cfg)

	require.NoError(t, err)
	data, err := os.ReadFile(RepoConfigPath(tmp))
	require.NoError(t, err)
	assert.Contains(t, string(data), "type: b")
	assert.Contains(t, string(data), "path: \"y\"")
	assert.NotContains(t, string(data), "type: a")
}

func TestLoadRepoConfig_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(""), 0o644)

	cfg, err := LoadRepoConfig(tmp)

	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Zero(t, cfg.Version)
	assert.Nil(t, cfg.Sources)
}

func TestWriteRepoConfig_InvalidPath(t *testing.T) {
	// NUL is invalid in paths on every OS
	err := WriteRepoConfig(string([]byte{0}), DefaultRepoConfig())
	assert.Error(t, err,
		"expected error for invalid path")

}

func TestWriteRepoConfig_WithCustomSources_WritesSources(t *testing.T) {
	tmp := t.TempDir()
	cfg := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "custom", Path: "my/path"},
			{Type: "multi", Paths: []string{"a", "b"}},
		},
	}

	err := WriteRepoConfig(tmp, cfg)

	require.NoError(t, err)
	data, err := os.ReadFile(RepoConfigPath(tmp))
	require.NoError(t, err)
	assert.Contains(t, string(data), "type: custom")
	assert.Contains(t, string(data), "path: my/path")
	assert.Contains(t, string(data), "type: multi")
	assert.Contains(t, string(data), "- a")
	assert.Contains(t, string(data), "- b")
}

func TestLoadRepoConfig_WithCustomSources_LoadsSources(t *testing.T) {
	tmp := t.TempDir()
	writeRepoConfigFixture(t, tmp, `version: 1
sources:
  - type: custom
    path: my/path
  - type: multi
    paths: [a, b]
`)

	loaded, err := LoadRepoConfig(tmp)

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Len(t, loaded.Sources, 2)
	assert.Equal(t, "custom", loaded.Sources[0].Type)
	assert.Equal(t, "my/path", loaded.Sources[0].Path)
	assert.Equal(t, "multi", loaded.Sources[1].Type)
	require.Len(t, loaded.Sources[1].Paths, 2)
	assert.Equal(t, "a", loaded.Sources[1].Paths[0])
	assert.Equal(t, "b", loaded.Sources[1].Paths[1])
}

func TestLoadRepoConfig_InvalidMarkdownRuleKind(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	yaml := `version: 1
sources:
  - type: markdown
    rules:
      - match: "*.md"
        kind: not_a_kind
`

	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644))

	_, err := LoadRepoConfig(tmp)

	assert.Error(t, err)
}

func writeRepoConfigFixture(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".devspecs")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644))
}
