package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply_WhenOneThreadIsRunnable_SelectsItImplicitly(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "F01", output.Target)
	require.NotNil(t, output.ThreadContext)
	assert.Equal(t, "agent-a", output.ThreadContext.Key)
	assert.Equal(t, "F01", output.ThreadContext.TargetAddress)
	assert.Contains(t, output.Prompt, "Execution thread:")
	assert.Contains(t, output.Prompt, "- Owner: task:thread-apply")
	assert.Contains(t, output.Prompt, "- Thread: agent-a")
}

func TestApply_WhenMultipleThreadsAreRunnable_RequiresThreadSelection(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "second lane")
	definition := validRepoThreadDefinitionForApply("thread-apply")
	definition.Threads[1].After = nil
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "multiple threads are runnable")
	assert.ErrorContains(t, err, "ds thread task:thread-apply --repo")
	assert.ErrorContains(t, err, "ds apply task:thread-apply --thread agent-a --repo")
	assert.ErrorContains(t, err, "ds apply task:thread-apply --thread both-a --repo")
}

func TestTaskNext_WhenMultipleThreadsAreRunnable_RejectsArbitrarySelection(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "second lane")
	definition := validRepoThreadDefinitionForApply("thread-apply")
	definition.Threads[1].After = nil
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewTaskCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"next", "thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "multiple threads are runnable: agent-a, both-a")
	assert.ErrorContains(t, err, "ds apply task:thread-apply --thread agent-a --repo")
	assert.ErrorContains(t, err, "ds apply task:thread-apply --thread both-a --repo")
}

func TestTaskStatus_WhenMultipleThreadsAreRunnable_OmitsSingularNextTarget(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "second lane")
	definition := validRepoThreadDefinitionForApply("thread-apply")
	definition.Threads[1].After = nil
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewTaskCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var output taskStatusOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Empty(t, output.NextTarget)
	assert.Contains(t, output.ThreadStatusCommand, "ds thread task:thread-apply --repo")
	require.Len(t, output.ThreadApplyCommands, 2)
	assert.Contains(t, output.ThreadApplyCommands[0], "ds apply task:thread-apply --thread agent-a --repo")
	assert.Contains(t, output.ThreadApplyCommands[1], "ds apply task:thread-apply --thread both-a --repo")
}

func TestTaskStatus_WhenOneThreadIsRunnable_ReportsExactThreadCommand(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewTaskCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"status", "thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var output taskStatusOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "F01", output.NextTarget)
	assert.Contains(t, output.NextCommand, "ds apply task:thread-apply --thread agent-a --repo")
	require.Len(t, output.ThreadApplyCommands, 1)
	assert.Equal(t, output.NextCommand, output.ThreadApplyCommands[0])
}

func TestTaskShow_WhenThreadSelectsLaterSlice_UsesRunnableLaneTarget(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first manifest slice", "runnable thread slice")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: "thread-apply"},
		Threads: []threadDefinitionLane{{
			Key: "agent-a", Targets: []threadTargetReference{{Target: "F02"}},
		}},
	}
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewTaskCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"show", "thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var output taskTargetOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "F02", output.Target)
	assert.Equal(t, "runnable thread slice", output.Title)
}

func TestTaskCheckpoint_WhenThreadSelectsLaterSlice_RecordsRunnableLaneTarget(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first manifest slice", "runnable thread slice")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: "thread-apply"},
		Threads: []threadDefinitionLane{{
			Key: "agent-a", Targets: []threadTargetReference{{Target: "F02"}},
		}},
	}
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewTaskCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"checkpoint", "thread-apply", "--stage", "validated", "--decision", "promote", "--index=false", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var output taskCheckpointOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	assert.Equal(t, "F02", output.Slice)
	assert.Contains(t, output.ResultPath, "F02-runnable-thread-slice-result.md")
}

func TestApply_WhenSelectedThreadDependencyIsIncomplete_RejectsSelection(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--thread", "both-a", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "thread \"both-a\" is waiting")
	assert.ErrorContains(t, err, "after agent-a (ready)")
}

func TestApply_WhenExplicitTargetIsLaterInLane_RejectsOrderBypass(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "second lane", "third lane")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: "thread-apply"},
		Threads: []threadDefinitionLane{{
			Key: "agent-a", Targets: []threadTargetReference{{Target: "F01"}, {Target: "F02"}},
		}},
	}
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--thread", "agent-a", "--target", "F02", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "is not thread \"agent-a\"'s current target \"F01\"")
	assert.ErrorContains(t, err, "lane order cannot be bypassed")
}

func TestApply_WhenSliceShorthandTargetsLaterThreadWork_RejectsOrderBypass(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "second lane")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: "thread-apply"},
		Threads: []threadDefinitionLane{{
			Key: "agent-a", Targets: []threadTargetReference{{Target: "F01"}, {Target: "F02"}},
		}},
	}
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"F02", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "target \"F02\" is not the current target of any thread")
	assert.ErrorContains(t, err, "cannot be bypassed")
}

func TestApply_WhenParentNeedsFollowup_SelectsInheritedFollowup(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	addApplyTestIteration(t, "thread-apply", "repair first lane", "F01", "improve")
	decideApplyTestTarget(t, "thread-apply", "F01", "improve")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--thread", "agent-a", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "F01-1", output.Target)
	require.NotNil(t, output.ThreadContext)
	require.NotNil(t, output.ThreadContext.Target)
	assert.Equal(t, "F01", output.ThreadContext.Target.DefinitionTarget)
	assert.Equal(t, "F01-1", output.ThreadContext.Target.Target)
}

func TestApply_WhenParentSelectorNeedsFollowup_SelectsInheritedFollowup(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	addApplyTestIteration(t, "thread-apply", "repair first lane", "F01", "improve")
	decideApplyTestTarget(t, "thread-apply", "F01", "improve")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--target", "F01", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "F01-1", output.Target)
	require.NotNil(t, output.ThreadContext)
	assert.Equal(t, "agent-a", output.ThreadContext.Key)
}

func TestApply_WhenWorkspaceLaneAdvancesAcrossRepos_ResolvesCurrentRepoPrompt(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	workspaceRoot, _, apiWorkspace, webRoot, webWorkspace := writeWorkspaceThreadFixture(t)
	prepareWorkspaceThreadApplyTarget(t, apiWorkspace, "A01")
	prepareWorkspaceThreadApplyTarget(t, webWorkspace, "B01")
	writeThreadCheckpointFixture(t, apiWorkspace, "api-task", "cp_api_a01", "A01", "validated", "promote")
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"change:COM-C014", "--thread", "both", "--workspace", workspaceRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "web-task", output.TaskID)
	assert.Equal(t, "B01", output.Target)
	assert.True(t, strings.HasPrefix(output.TargetContext.Workspace, filepath.Join(webRoot, "devspecs", "tasks", "web-task")))
	require.NotNil(t, output.ThreadContext)
	assert.Equal(t, "web:B01", output.ThreadContext.TargetAddress)
	assert.Contains(t, output.Prompt, "- Owner: change:COM-C014")
	assert.Contains(t, output.Prompt, "- Current target: web:B01")
}

func TestApply_WhenWorkspaceThreadsCompleteAndChildTaskIsRequested_SelectsRepoDurabilityCloseout(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	workspaceRoot, apiRoot, apiWorkspace, _, webWorkspace := writeWorkspaceThreadFixture(t)
	prepareCompletedWorkspaceThreadApply(t, apiWorkspace, webWorkspace)
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"api-task", "--repo", apiRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "api-task", output.TaskID)
	assert.Equal(t, "A00", output.Target)
	assert.Contains(t, output.Command, "ds apply api-task --repo ")
	require.NotNil(t, output.ThreadContext)
	assert.Equal(t, threadOwnerWorkspaceChange, output.ThreadContext.Owner.Kind)
	assert.Equal(t, "COM-C014", output.ThreadContext.Owner.ChangeID)
	assert.Equal(t, threadStateCompleted, output.ThreadContext.State)
	assert.Contains(t, output.Prompt, "You are closing DevSpecs task api-task")
	assert.Contains(t, output.Prompt, workspaceRoot)
}

func TestApply_WhenWorkspaceThreadsCompleteAndChildCloseoutIsExplicit_AcceptsSeriesTarget(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	_, apiRoot, apiWorkspace, _, webWorkspace := writeWorkspaceThreadFixture(t)
	prepareCompletedWorkspaceThreadApply(t, apiWorkspace, webWorkspace)
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:api-task", "--target", "A00", "--repo", apiRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "api-task", output.TaskID)
	assert.Equal(t, "A00", output.Target)
	assert.Equal(t, "ds apply task:api-task --target A00 --repo "+apiRoot, output.Command)
}

func TestApply_WhenWorkspaceThreadsCompleteAndChangeIsRequested_ReportsTerminalChange(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	workspaceRoot, _, apiWorkspace, _, webWorkspace := writeWorkspaceThreadFixture(t)
	prepareCompletedWorkspaceThreadApply(t, apiWorkspace, webWorkspace)
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"change:COM-C014", "--workspace", workspaceRoot, "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.EqualError(t, err, "workspace change \"COM-C014\" has completed all assigned threads")
}

func TestApply_WhenImplicitSelectionAlsoHasLegacyTask_RejectsAmbiguity(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "thread lane", "joined lane")
	createApplyTask(t, "legacy-apply", "L", "legacy lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "implicit apply is ambiguous between a named thread and legacy targets")
	assert.ErrorContains(t, err, "legacy-apply:L01")
}

func TestApply_WhenCompletedThreadsLeaveUnassignedTarget_BlocksCloseout(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "assigned lane", "unassigned lane")
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: "thread-apply"},
		Threads: []threadDefinitionLane{{
			Key: "agent-a", Targets: []threadTargetReference{{Target: "F01"}},
		}},
	}
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", definition)
	decideApplyTestTarget(t, "thread-apply", "F01", "promote")
	decideApplyTestTarget(t, "thread-apply", "F02", "promote")
	cmd := NewApplyCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "no thread is runnable")
	assert.ErrorContains(t, err, "unassigned work")
}

func TestApply_WhenAllThreadTargetsAreComplete_SelectsDurabilityCloseout(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	decideApplyTestTarget(t, "thread-apply", "F01", "promote")
	decideApplyTestTarget(t, "thread-apply", "F02", "promote")
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Equal(t, "F00", output.Target)
	require.NotNil(t, output.ThreadContext)
	assert.Equal(t, threadStateCompleted, output.ThreadContext.State)
	assert.Nil(t, output.ThreadContext.Target)
}

func TestApply_WhenThreadUsesRepoFlag_PreservesRepoArgumentInPrompt(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := setupApplyTask(t, "thread-apply", "F", "first lane", "joined lane")
	writeApplyThreadDefinition(t, repoRoot, "thread-apply", validRepoThreadDefinitionForApply("thread-apply"))
	cmd := NewApplyCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"thread-apply", "--thread", "agent-a", "--repo", ".", "--json"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	output := decodeApplyPromptOutput(t, stdout)
	assert.Contains(t, output.Prompt, "ds task checkpoint thread-apply --target F01 --repo .")
}

func validRepoThreadDefinitionForApply(taskID string) threadDefinition {
	return threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner:         threadDefinitionOwner{Kind: threadOwnerTask, TaskID: taskID},
		Threads: []threadDefinitionLane{
			{Key: "agent-a", Targets: []threadTargetReference{{Target: "F01"}}},
			{Key: "both-a", Targets: []threadTargetReference{{Target: "F02"}}, After: []string{"agent-a"}},
		},
	}
}

func writeApplyThreadDefinition(t *testing.T, repoRoot, taskID string, definition threadDefinition) {
	t.Helper()
	path := repoThreadDefinitionPath(taskWorkspacePath(repoRoot, defaultTaskWorkspaceDir, taskID))
	require.NoError(t, writeThreadDefinition(path, definition))
}

func prepareWorkspaceThreadApplyTarget(t *testing.T, taskWorkspace, target string) {
	t.Helper()
	manifestPath := filepath.Join(taskWorkspace, taskManifestFilename)
	manifest, err := readTaskManifest(manifestPath)
	require.NoError(t, err)
	require.Len(t, manifest.Artifacts.Slices, 1)
	manifest.Artifacts.Slices[0].Plan = target + "-plan.md"
	manifest.Artifacts.Slices[0].Result = target + "-result.md"
	require.NoError(t, writeTaskManifest(manifestPath, manifest))
	require.NoError(t, os.WriteFile(filepath.Join(taskWorkspace, manifest.Artifacts.Slices[0].Plan), []byte("# Plan\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(taskWorkspace, manifest.Artifacts.Slices[0].Result), []byte("# Result\n"), 0o644))
}

func prepareCompletedWorkspaceThreadApply(t *testing.T, apiWorkspace, webWorkspace string) {
	t.Helper()
	prepareWorkspaceThreadApplyTarget(t, apiWorkspace, "A01")
	prepareWorkspaceThreadApplyTarget(t, webWorkspace, "B01")
	prepareWorkspaceThreadCloseout(t, apiWorkspace, "A00-index.md")
	prepareWorkspaceThreadCloseout(t, webWorkspace, "B00-index.md")
	writeThreadCheckpointFixture(t, apiWorkspace, "api-task", "cp_api_a01", "A01", "validated", "promote")
	writeThreadCheckpointFixture(t, webWorkspace, "web-task", "cp_web_b01", "B01", "validated", "promote")
}

func prepareWorkspaceThreadCloseout(t *testing.T, taskWorkspace, indexName string) {
	t.Helper()
	manifestPath := filepath.Join(taskWorkspace, taskManifestFilename)
	manifest, err := readTaskManifest(manifestPath)
	require.NoError(t, err)
	manifest.Artifacts.Index = indexName
	manifest.Durability.Required = true
	require.NoError(t, writeTaskManifest(manifestPath, manifest))
	require.NoError(t, os.WriteFile(filepath.Join(taskWorkspace, indexName), []byte("# Task closeout\n"), 0o644))
}
