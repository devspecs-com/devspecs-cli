package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThreadSet_WhenRepoOwnerHasNoDefinition_CreatesFirstRevision(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	require.NoError(t, os.Remove(location.DefinitionPath))
	cmd := NewThreadCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"set", "task:thread-task", "agent-a", "F01", "F02", "--repo", repoRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	definition, readErr := readThreadDefinition(repoThreadDefinitionPath(taskWorkspace))
	require.NoError(t, readErr)
	assert.Equal(t, 1, definition.Revision)
	assert.Len(t, definition.Threads, 1)
	assert.Equal(t, "agent-a", definition.Threads[0].Key)
	assert.Len(t, definition.Threads[0].Targets, 2)
	assert.Equal(t, "F01", definition.Threads[0].Targets[0].Target)
	assert.Equal(t, "F02", definition.Threads[0].Targets[1].Target)
	var output threadMutationOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "set", output.Action)
	assert.Equal(t, "refreshed", output.ProjectionStatus)
}

func TestThreadSet_WhenLaneExists_ReplacesLaneAndAdvancesRevision(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot, _, location := writeRepoThreadFixture(t, "thread-task")
	cmd := NewThreadCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"set", "task:thread-task", "both-a", "F02", "F03", "--after", "agent-a", "--name", "Join work", "--repo", repoRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	definition, readErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, readErr)
	assert.Equal(t, 2, definition.Revision)
	assert.Len(t, definition.Threads, 2)
	assert.Equal(t, "both-a", definition.Threads[1].Key)
	assert.Equal(t, "Join work", definition.Threads[1].Name)
	assert.Len(t, definition.Threads[1].Targets, 2)
	assert.Equal(t, "F02", definition.Threads[1].Targets[0].Target)
	assert.Equal(t, "F03", definition.Threads[1].Targets[1].Target)
	assert.Len(t, definition.Threads[1].After, 1)
	assert.Equal(t, "agent-a", definition.Threads[1].After[0])
}

func TestThreadRemove_WhenLaneHasCheckpointHistory_RejectsMutation(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	definition, definitionErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, definitionErr)
	definition.Threads[1].After = nil
	require.NoError(t, writeThreadDefinition(location.DefinitionPath, definition))
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f01", "F01", "validated", "promote")
	before, readErr := os.ReadFile(location.DefinitionPath)
	require.NoError(t, readErr)
	cmd := NewThreadCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"remove", "task:thread-task", "agent-a", "--repo", repoRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "checkpoint history")
	after, afterErr := os.ReadFile(location.DefinitionPath)
	require.NoError(t, afterErr)
	assert.Equal(t, string(before), string(after))
}

func TestThreadRemove_WhenLaneHasNoHistory_RemovesLaneAndAdvancesRevision(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot, _, location := writeRepoThreadFixture(t, "thread-task")
	cmd := NewThreadCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"remove", "task:thread-task", "both-a", "--repo", repoRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	definition, readErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, readErr)
	assert.Equal(t, 2, definition.Revision)
	assert.Len(t, definition.Threads, 1)
	assert.Equal(t, "agent-a", definition.Threads[0].Key)
	var output threadMutationOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "remove", output.Action)
	assert.Equal(t, "refreshed", output.ProjectionStatus)
}

func TestThreadSet_WhenWorkspaceUsesRepoTarget_ResolvesLinkedTask(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	workspaceRoot, _, _, _, _ := writeWorkspaceThreadFixture(t)
	manifest, err := readWorkspaceManifest(workspaceRoot)
	require.NoError(t, err)
	definitionPath := workspaceThreadDefinitionPath(workspaceRoot, manifest, "COM-C014")
	require.NoError(t, os.Remove(definitionPath))
	cmd := NewThreadCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"set", "change:COM-C014", "cross-repo", "api:A01", "web:B01", "--workspace", workspaceRoot, "--json"})

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	definition, readErr := readThreadDefinition(definitionPath)
	require.NoError(t, readErr)
	assert.Equal(t, 1, definition.Revision)
	assert.Len(t, definition.Threads, 1)
	assert.Len(t, definition.Threads[0].Targets, 2)
	assert.Equal(t, "api", definition.Threads[0].Targets[0].Repo)
	assert.Equal(t, "api-task", definition.Threads[0].Targets[0].Task)
	assert.Equal(t, "A01", definition.Threads[0].Targets[0].Target)
	assert.Equal(t, "web", definition.Threads[0].Targets[1].Repo)
	assert.Equal(t, "web-task", definition.Threads[0].Targets[1].Task)
	assert.Equal(t, "B01", definition.Threads[0].Targets[1].Target)
}

func TestThreadRemove_WhenLinkedCheckpointPublishesConcurrently_ReloadsHistoryBeforeMutation(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	workspaceRoot, _, apiWorkspace, _, _ := writeWorkspaceThreadFixture(t)
	workspaceManifest, manifestErr := readWorkspaceManifest(workspaceRoot)
	require.NoError(t, manifestErr)
	definitionPath := workspaceThreadDefinitionPath(workspaceRoot, workspaceManifest, "COM-C014")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner: threadDefinitionOwner{
			Kind: threadOwnerWorkspaceChange, WorkspaceID: "umbrella", ChangeID: "COM-C014",
		},
		Threads: []threadDefinitionLane{
			{Key: "api", Targets: []threadTargetReference{{Repo: "api", Task: "api-task", Target: "A01"}}},
			{Key: "web", Targets: []threadTargetReference{{Repo: "web", Task: "web-task", Target: "B01"}}},
		},
	}
	require.NoError(t, writeThreadDefinition(definitionPath, definition))
	checkpointLease, leaseErr := acquireTaskMutation(context.Background(), apiWorkspace)
	require.NoError(t, leaseErr)
	cmd := NewThreadCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"remove", "change:COM-C014", "api", "--workspace", workspaceRoot, "--json"})
	result := make(chan error, 1)

	// Act
	go func() { result <- cmd.Execute() }()
	select {
	case earlyErr := <-result:
		require.Failf(t, "thread mutation completed before linked checkpoint lock was released", "error: %v", earlyErr)
	case <-time.After(250 * time.Millisecond):
	}
	writeThreadCheckpointFixture(t, apiWorkspace, "api-task", "cp_api_a01", "A01", "validated", "promote")
	require.NoError(t, checkpointLease.Release())
	err := <-result

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "checkpoint history")
	definition, definitionErr := readThreadDefinition(definitionPath)
	require.NoError(t, definitionErr)
	assert.Equal(t, 1, definition.Revision)
	assert.Len(t, definition.Threads, 2)
	assert.Equal(t, "api", definition.Threads[0].Key)
	assert.Equal(t, "web", definition.Threads[1].Key)
}

func TestThreadSet_WhenDefinitionChangesWhileWaitingForLock_PreservesBothUpdates(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	definitionLease, leaseErr := acquireTaskMutation(context.Background(), taskWorkspace)
	require.NoError(t, leaseErr)
	cmd := NewThreadCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"set", "task:thread-task", "agent-b", "F03", "--repo", repoRoot, "--json"})
	result := make(chan error, 1)

	// Act
	go func() { result <- cmd.Execute() }()
	select {
	case earlyErr := <-result:
		require.Failf(t, "thread mutation completed before definition lock was released", "error: %v", earlyErr)
	case <-time.After(250 * time.Millisecond):
	}
	definition, readErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, readErr)
	definition.Revision++
	definition.Threads[0].Name = "Human updated"
	require.NoError(t, writeThreadDefinition(location.DefinitionPath, definition))
	require.NoError(t, definitionLease.Release())
	err := <-result

	// Assert
	require.NoError(t, err)
	updated, updatedErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, updatedErr)
	assert.Equal(t, 3, updated.Revision)
	assert.Len(t, updated.Threads, 3)
	assert.Equal(t, "Human updated", updated.Threads[0].Name)
	assert.Equal(t, "agent-b", updated.Threads[2].Key)
}
