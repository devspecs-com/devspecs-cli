//go:build !windows

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

func TestRemoteAdd_WithConfigDirectorySymlink_LeavesExternalConfigUntouched(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	external := t.TempDir()
	path := filepath.Join(external, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(testRemoteConfig), 0o644))
	require.NoError(t, os.Symlink(external, filepath.Join(root, ".devspecs")))
	cmd := NewRemoteCmd()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"add", "other", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Act
	err := cmd.Execute()

	// Assert
	requireRemoteBoundaryError(t, err)
	assert.Empty(t, stdout.String())
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, testRemoteConfig, string(after))
	entries, readErr := os.ReadDir(external)
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.yaml", entries[0].Name())
}

func TestRemoteList_WithConfigFileSymlink_RejectsExternalConfigBeforeRead(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	external := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(external, []byte(testRemoteConfig), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".devspecs"), 0o755))
	path := config.RepoConfigPath(root)
	require.NoError(t, os.Symlink(external, path))
	cmd := NewRemoteCmd()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"list", "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Act
	err := cmd.Execute()

	// Assert
	requireRemoteBoundaryError(t, err)
	assert.Empty(t, stdout.String())
	after, readErr := os.ReadFile(external)
	require.NoError(t, readErr)
	assert.Equal(t, testRemoteConfig, string(after))
	info, statErr := os.Lstat(path)
	require.NoError(t, statErr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestRemoteWrite_WithDirectoryRedirectedAfterLoad_RechecksBeforeAtomicWrite(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, testRemoteConfig)
	doc, err := loadRemoteDocument()
	require.NoError(t, err)
	external := t.TempDir()
	path := filepath.Join(external, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(testRemoteConfig), 0o644))
	directory := filepath.Join(root, ".devspecs")
	require.NoError(t, os.Rename(directory, filepath.Join(root, "saved-config")))
	require.NoError(t, os.Symlink(external, directory))

	// Act
	err = doc.write()

	// Assert
	requireRemoteBoundaryError(t, err)
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, testRemoteConfig, string(after))
	entries, readErr := os.ReadDir(external)
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.yaml", entries[0].Name())
}

func TestRemoteWrite_WithFileRedirectedAfterLoad_RechecksBeforeAtomicWrite(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, testRemoteConfig)
	doc, err := loadRemoteDocument()
	require.NoError(t, err)
	external := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(external, []byte(testRemoteConfig), 0o644))
	path := config.RepoConfigPath(root)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Symlink(external, path))

	// Act
	err = doc.write()

	// Assert
	requireRemoteBoundaryError(t, err)
	after, readErr := os.ReadFile(external)
	require.NoError(t, readErr)
	assert.Equal(t, testRemoteConfig, string(after))
	info, statErr := os.Lstat(path)
	require.NoError(t, statErr)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}
