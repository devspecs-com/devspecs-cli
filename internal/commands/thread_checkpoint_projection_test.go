package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCheckpoint_WhenTaskHasThreadDefinition_RefreshesProjection(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-checkpoint", "F", "thread lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-checkpoint", validRepoThreadDefinitionForApply("thread-checkpoint"))
	cmd := NewTaskCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"checkpoint", "thread-checkpoint", "--target", "F01", "--stage", "validated",
		"--decision", "promote", "--index=false", "--json",
	})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var output taskCheckpointOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "refreshed", output.ThreadProjection)
	assert.Empty(t, output.ThreadWarning)
	database, openErr := openDB()
	require.NoError(t, openErr)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	manifest, readErr := readTaskManifest(filepath.Join(repoRoot, "devspecs", "tasks", "thread-checkpoint", taskManifestFilename))
	require.NoError(t, readErr)
	location := repoThreadOwnerLocation(repoRoot, manifest.Workspace, manifest)
	projection, found, projectionErr := database.GetThreadProjection(location.OwnerID)
	require.NoError(t, projectionErr)
	assert.True(t, found)
	assert.Len(t, projection.Events, 1)
	assert.Equal(t, "F01", projection.Events[0].Target)
}
