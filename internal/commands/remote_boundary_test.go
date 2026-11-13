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

func requireRemoteBoundaryError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "config_boundary_invalid", failure.Code)
}

func TestRemoteAdd_WithFileInsteadOfConfigDirectory_RejectsWithoutChangingFile(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	path := filepath.Join(root, ".devspecs")
	content := "owned fixture file, not a directory"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	cmd := NewRemoteCmd()
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Act
	err := cmd.Execute()

	// Assert
	requireRemoteBoundaryError(t, err)
	assert.Empty(t, stdout.String())
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, content, string(after))
}

func TestRemoteList_WithDirectoryInsteadOfConfigFile_RejectsBeforeRead(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	path := config.RepoConfigPath(root)
	require.NoError(t, os.MkdirAll(path, 0o755))
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
	entries, readErr := os.ReadDir(path)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestRemoteWrite_WithDirectoryLeafAfterLoad_RejectsBeforeAtomicWrite(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, testRemoteConfig)
	doc, err := loadRemoteDocument()
	require.NoError(t, err)
	path := config.RepoConfigPath(root)
	require.NoError(t, os.Remove(path))
	require.NoError(t, os.Mkdir(path, 0o755))

	// Act
	err = doc.write()

	// Assert
	requireRemoteBoundaryError(t, err)
	entries, readErr := os.ReadDir(path)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}
