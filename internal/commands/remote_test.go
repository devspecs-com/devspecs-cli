package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const testRemoteTenant = "33333333-3333-4333-8333-333333333333"
const testRemoteRepo = "44444444-4444-4444-8444-444444444444"
const testRemoteConfig = `version: 1
sources: []
remotes:
  - version: 1
    name: hosted
    url: https://hub.example/base
    tenant_id: 33333333-3333-4333-8333-333333333333
    repo_id: 44444444-4444-4444-8444-444444444444
`

func remoteTestDirectory(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	initTaskGitRepo(t, root)
	t.Chdir(root)
	t.Setenv("DEVSPECS_HOME", filepath.Join(root, "home"))
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	if content != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".devspecs"), 0o755))
		require.NoError(t, os.WriteFile(config.RepoConfigPath(root), []byte(content), 0o644))
	}
	return root
}

func TestRemoteAdd_WithoutConfig_WritesLoadableMetadataWithoutCycle(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	cmd := NewRemoteCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	cfg, err := config.LoadRepoConfig(root)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Remotes, 1)
	assert.Equal(t, "https://hub.example/base", cfg.Remotes[0].URL)
	assert.Equal(t, testRemoteTenant, cfg.Remotes[0].TenantID)
	var result remoteResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, 1, result.SchemaVersion)
	assert.Equal(t, "added", result.Status)
	entries, err := os.ReadDir(filepath.Join(root, ".devspecs"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.yaml", entries[0].Name())
}

func TestRemoteAdd_WithUnknownRootFields_PreservesUserData(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "version: 1\nsources: []\nfuture_section:\n  custom: preserved\nartifacts:\n  test_cases: true\nintegrations:\n  future_provider:\n    enabled: true\n")
	cmd := NewRemoteCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(config.RepoConfigPath(root))
	require.NoError(t, err)
	var value map[string]any
	require.NoError(t, yaml.Unmarshal(data, &value))
	require.Len(t, value, 6)
	future, ok := value["future_section"].(map[string]any)
	require.True(t, ok)
	require.Len(t, future, 1)
	assert.Equal(t, "preserved", future["custom"])
	assert.Contains(t, string(data), "future_provider:")
	assert.Contains(t, string(data), "test_cases: true")
}

func TestRemoteAdd_InFreshGitRepoWithAncestorConfig_WritesOnlyInsideGitRoot(t *testing.T) {
	// Arrange
	ancestor := t.TempDir()
	ancestorContent := "version: 1\nsources: []\nfuture_marker: ancestor-only\n"
	require.NoError(t, os.MkdirAll(filepath.Join(ancestor, ".devspecs"), 0o755))
	require.NoError(t, os.WriteFile(config.RepoConfigPath(ancestor), []byte(ancestorContent), 0o644))
	root := filepath.Join(ancestor, "child")
	require.NoError(t, os.MkdirAll(root, 0o755))
	initTaskGitRepo(t, root)
	t.Chdir(root)
	t.Setenv("DEVSPECS_HOME", filepath.Join(ancestor, "home"))
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	_, err := os.Stat(filepath.Join(root, ".devspecs"))
	require.ErrorIs(t, err, os.ErrNotExist)
	cmd := NewRemoteCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	cfg, err := config.LoadRepoConfig(root)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Remotes, 1)
	assert.Equal(t, "hosted", cfg.Remotes[0].Name)
	assert.Equal(t, "https://hub.example/base", cfg.Remotes[0].URL)
	after, err := os.ReadFile(config.RepoConfigPath(ancestor))
	require.NoError(t, err)
	assert.Equal(t, ancestorContent, string(after))
}

func TestRemoteAdd_OutsideGitWithAncestorDevspecs_RejectsWithoutChangingAncestor(t *testing.T) {
	// Arrange
	ancestor := t.TempDir()
	ancestorContent := "version: 1\nsources: []\nfuture_marker: global-home-style\n"
	require.NoError(t, os.MkdirAll(filepath.Join(ancestor, ".devspecs"), 0o755))
	require.NoError(t, os.WriteFile(config.RepoConfigPath(ancestor), []byte(ancestorContent), 0o644))
	root := filepath.Join(ancestor, "nongit")
	require.NoError(t, os.MkdirAll(root, 0o755))
	t.Chdir(root)
	t.Setenv("DEVSPECS_HOME", filepath.Join(ancestor, ".devspecs"))
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	cmd := NewRemoteCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "repository_required", failure.Code)
	assert.Empty(t, stdout.String())
	after, readErr := os.ReadFile(config.RepoConfigPath(ancestor))
	require.NoError(t, readErr)
	assert.Equal(t, ancestorContent, string(after))
	_, statErr := os.Stat(filepath.Join(root, ".devspecs"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestRemoteAdd_WithFutureRemoteVersion_DoesNotMutateFile(t *testing.T) {
	// Arrange
	content := bytes.Replace([]byte(testRemoteConfig), []byte("  - version: 1"), []byte("  - version: 2"), 1)
	root := remoteTestDirectory(t, string(content))
	cmd := NewRemoteCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"add", "other", "https://other.example", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo})
	// Act
	err := cmd.Execute()
	// Assert
	require.Error(t, err)
	after, readErr := os.ReadFile(config.RepoConfigPath(root))
	require.NoError(t, readErr)
	assert.Equal(t, string(content), string(after))
}

func TestRemoteRemove_WithUnknownRootFields_PreservesOtherSections(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, testRemoteConfig+"future_section:\n  keep: true\n")
	cmd := NewRemoteCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"remove", "hosted", "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	cfg, err := config.LoadRepoConfig(root)
	require.NoError(t, err)
	assert.Empty(t, cfg.Remotes)
	data, err := os.ReadFile(config.RepoConfigPath(root))
	require.NoError(t, err)
	assert.Contains(t, string(data), "future_section:")
	assert.Contains(t, string(data), "keep: true")
}

func TestRemoteList_WithAmbientToken_RemainsLocalMetadataOnly(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, testRemoteConfig)
	t.Setenv("DEVSPECS_REMOTE_TOKEN", "private-ambient-token")
	t.Setenv("DEVSPECS_REMOTE_URL", "https://attacker.invalid")
	cmd := NewRemoteCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"list", "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	var result remoteResult
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.NotNil(t, result.Remotes)
	require.Len(t, *result.Remotes, 1)
	assert.Equal(t, "hosted", (*result.Remotes)[0].Name)
	assert.NotContains(t, output.String(), "private-ambient-token")
	_, err = os.Stat(filepath.Join(root, "home"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRemoteList_WithoutConfig_ReturnsVersionedEmptyArray(t *testing.T) {
	// Arrange
	remoteTestDirectory(t, "")
	cmd := NewRemoteCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"list", "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	assert.Contains(t, output.String(), `"schema_version":1`)
	assert.Contains(t, output.String(), `"remotes":[]`)
}

func TestPrintCommandError_WithJSONBeforeRemoteCommand_RedactsRawCobraInput(t *testing.T) {
	// Arrange
	var stderr bytes.Buffer
	err := errors.New("private-secret-in-parser-diagnostic")
	// Act
	PrintCommandError(&stderr, err, []string{"--json", "login", "hosted"})
	// Assert
	var result remoteResult
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &result))
	assert.Equal(t, 1, result.SchemaVersion)
	assert.Equal(t, "error", result.Status)
	require.NotNil(t, result.Error)
	assert.Equal(t, "invalid_arguments", result.Error.Code)
	assert.NotContains(t, stderr.String(), "private-secret")
}

func TestPrintCommandError_WithLocalRemoteSearch_PreservesLegacyErrorHandling(t *testing.T) {
	// Arrange
	var stderr bytes.Buffer
	err := errors.New("local diagnostic")
	// Act
	PrintCommandError(&stderr, err, []string{"find", "remote", "--json"})
	// Assert
	assert.Equal(t, "local diagnostic\n", stderr.String())
}
