package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadThreadOwnerSnapshot_WhenProjectionIsCold_ProducesEquivalentWarmStatus(t *testing.T) {
	// Arrange
	repoRoot, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f01", "F01", "validated", "promote")
	database, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	// Act
	coldSnapshot, coldStats, coldErr := loadThreadOwnerSnapshot(location, nil)
	coldStatus, coldStatusErr := buildThreadStatus(coldSnapshot)
	projection, projectionErr := threadProjectionFromSnapshot(coldSnapshot)
	require.NoError(t, projectionErr)
	require.NoError(t, database.ReplaceThreadProjection(projection))
	stored, found, loadErr := database.GetThreadProjection(location.OwnerID)
	warmSnapshot, warmSnapshotErr := threadSnapshotFromProjection(stored)
	warmStatus, warmStatusErr := buildThreadStatus(warmSnapshot)

	// Assert
	require.NoError(t, coldErr)
	require.NoError(t, coldStatusErr)
	require.NoError(t, loadErr)
	assert.True(t, found)
	require.NoError(t, warmSnapshotErr)
	require.NoError(t, warmStatusErr)
	assert.Equal(t, 1, coldStats.EventsParsed)
	assert.Zero(t, coldStats.EventsReused)
	assert.Equal(t, repoRoot, coldStatus.Owner.RepoRoot)
	assert.Equal(t, coldStatus.Owner, warmStatus.Owner)
	assert.Len(t, coldStatus.Threads, 2)
	assert.Len(t, warmStatus.Threads, 2)
	assert.Equal(t, coldStatus.Threads[0].Key, warmStatus.Threads[0].Key)
	assert.Equal(t, coldStatus.Threads[0].State, warmStatus.Threads[0].State)
	assert.Len(t, coldStatus.Threads[0].Targets, 1)
	assert.Len(t, warmStatus.Threads[0].Targets, 1)
	assert.Equal(t, coldStatus.Threads[0].Targets[0].Target, warmStatus.Threads[0].Targets[0].Target)
	assert.Equal(t, coldStatus.Threads[1].Key, warmStatus.Threads[1].Key)
	assert.Equal(t, coldStatus.Threads[1].State, warmStatus.Threads[1].State)
	require.NotNil(t, coldStatus.Threads[1].CurrentTarget)
	require.NotNil(t, warmStatus.Threads[1].CurrentTarget)
	assert.Equal(t, coldStatus.Threads[1].CurrentTarget.Target, warmStatus.Threads[1].CurrentTarget.Target)
	assert.Len(t, coldStatus.UnassignedTargets, 1)
	assert.Len(t, warmStatus.UnassignedTargets, 1)
	assert.Equal(t, coldStatus.UnassignedTargets[0].Target, warmStatus.UnassignedTargets[0].Target)
}

func TestLoadThreadOwnerSnapshot_WhenOneCheckpointIsAdded_ReusesUnchangedEvents(t *testing.T) {
	// Arrange
	_, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f01", "F01", "validated", "promote")
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f02", "F02", "started", "")
	initial, initialStats, err := loadThreadOwnerSnapshot(location, nil)
	require.NoError(t, err)
	projection, err := threadProjectionFromSnapshot(initial)
	require.NoError(t, err)
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f03", "F03", "planned", "continue")

	// Act
	refreshed, refreshedStats, refreshErr := loadThreadOwnerSnapshot(location, &projection)

	// Assert
	require.NoError(t, refreshErr)
	assert.Equal(t, 2, initialStats.EventsParsed)
	assert.Zero(t, initialStats.EventsReused)
	assert.Equal(t, 1, refreshedStats.EventsParsed)
	assert.Equal(t, 2, refreshedStats.EventsReused)
	require.Len(t, refreshed.Tasks, 1)
	assert.Len(t, refreshed.Tasks[0].Events, 3)
	assert.Equal(t, "cp_f01", refreshed.Tasks[0].Events[0].Record.CheckpointID)
	assert.Equal(t, "cp_f02", refreshed.Tasks[0].Events[1].Record.CheckpointID)
	assert.Equal(t, "cp_f03", refreshed.Tasks[0].Events[2].Record.CheckpointID)
}

func TestDeriveThreadStatus_WhenProjectionWasPruned_RebuildsWithoutWeakeningOutput(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	_, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f01", "F01", "validated", "promote")
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	database, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	require.NoError(t, database.DeleteThreadProjection(location.OwnerID))
	require.NoError(t, database.Close())

	// Act
	out, deriveErr := deriveThreadStatus(cmd, location)
	database, reopenErr := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, reopenErr)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	_, found, projectionErr := database.GetThreadProjection(location.OwnerID)

	// Assert
	require.NoError(t, deriveErr)
	assert.Equal(t, "artifact_reconstruction", out.StateSource)
	assert.Equal(t, "rebuilt", out.ProjectionFreshness)
	assert.True(t, out.ProjectionRefreshed)
	assert.Len(t, out.Threads, 2)
	assert.Equal(t, threadStateCompleted, out.Threads[0].State)
	assert.Equal(t, threadStateReady, out.Threads[1].State)
	require.NotNil(t, out.Threads[1].CurrentTarget)
	assert.Equal(t, "F02", out.Threads[1].CurrentTarget.Target)
	require.NoError(t, projectionErr)
	assert.True(t, found)
}

func TestDeriveThreadStatus_WhenProjectionSourcesAreFresh_UsesProjectedState(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	_, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	writeThreadCheckpointFixture(t, taskWorkspace, "thread-task", "cp_f01", "F01", "validated", "promote")
	snapshot, _, err := loadThreadOwnerSnapshot(location, nil)
	require.NoError(t, err)
	projection, err := threadProjectionFromSnapshot(snapshot)
	require.NoError(t, err)
	database, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	require.NoError(t, database.ReplaceThreadProjection(projection))
	require.NoError(t, database.Close())
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Act
	out, deriveErr := deriveThreadStatus(cmd, location)

	// Assert
	require.NoError(t, deriveErr)
	assert.Equal(t, "sqlite_projection", out.StateSource)
	assert.Equal(t, "fresh", out.ProjectionFreshness)
	assert.False(t, out.ProjectionRefreshed)
	assert.Len(t, out.Threads, 2)
	assert.Equal(t, threadStateCompleted, out.Threads[0].State)
	assert.Equal(t, threadStateReady, out.Threads[1].State)
}

func TestResolveThreadOwnerLocation_WhenTaskIsUnlinkedInsideWorkspace_KeepsRepoAuthority(t *testing.T) {
	// Arrange
	workspaceRoot := t.TempDir()
	repoRoot := filepath.Join(workspaceRoot, "api")
	require.NoError(t, os.MkdirAll(repoRoot, 0o755))
	_, _, location := writeRepoThreadFixtureAt(t, repoRoot, "api-task")
	manifestPath := filepath.Join(location.TaskWorkspace, taskManifestFilename)
	manifest, err := readTaskManifest(manifestPath)
	require.NoError(t, err)
	manifest.WorkspaceID = "umbrella"
	manifest.WorkspaceRoot = workspaceRoot
	require.NoError(t, writeTaskManifest(manifestPath, manifest))
	cmd := NewThreadCmd()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--repo", repoRoot}))
	require.Equal(t, repoRoot, commandRepoTarget(cmd))
	resolvedRepo, repoErr := resolveTargetRepoRootContext(cmd.Context(), commandRepoTarget(cmd))
	require.NoError(t, repoErr)
	require.Equal(t, repoRoot, resolvedRepo)

	// Act
	resolved, resolveErr := resolveThreadOwnerLocation(cmd, "api-task", threadStatusOptions{Dir: defaultTaskWorkspaceDir})

	// Assert
	require.NoError(t, resolveErr)
	assert.Equal(t, threadOwnerTask, resolved.Kind)
	assert.Equal(t, "api-task", resolved.TaskID)
	assert.Equal(t, repoRoot, resolved.RepoRoot)
	assert.Empty(t, resolved.WorkspaceRoot)
}

func TestResolveThreadOwnerLocation_WhenTaskLinksWorkspaceChange_EscalatesToCrossRepoOwner(t *testing.T) {
	// Arrange
	workspaceRoot, apiRoot, apiWorkspace, webRoot, webWorkspace := writeWorkspaceThreadFixture(t)
	cmd := NewThreadCmd()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--repo", apiRoot}))
	require.Equal(t, apiRoot, commandRepoTarget(cmd))

	// Act
	resolved, resolveErr := resolveThreadOwnerLocation(cmd, "api-task", threadStatusOptions{Dir: defaultTaskWorkspaceDir})
	snapshot, _, snapshotErr := loadThreadOwnerSnapshot(resolved, nil)
	status, statusErr := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, resolveErr)
	assert.Equal(t, threadOwnerWorkspaceChange, resolved.Kind)
	assert.Equal(t, "COM-C014", resolved.ChangeID)
	assert.Equal(t, workspaceRoot, resolved.WorkspaceRoot)
	require.NoError(t, snapshotErr)
	assert.Len(t, snapshot.Tasks, 2)
	assert.Equal(t, apiWorkspace, snapshot.Tasks[0].Workspace)
	assert.Equal(t, webWorkspace, snapshot.Tasks[1].Workspace)
	assert.Equal(t, apiRoot, snapshot.Tasks[0].RepoRoot)
	assert.Equal(t, webRoot, snapshot.Tasks[1].RepoRoot)
	require.NoError(t, statusErr)
	assert.Len(t, status.Threads, 1)
	assert.Len(t, status.Threads[0].Targets, 2)
	assert.Equal(t, "api:A01", threadStatusTargetAddress(status.Threads[0].Targets[0]))
	assert.Equal(t, "web:B01", threadStatusTargetAddress(status.Threads[0].Targets[1]))
}

func TestResolveThreadOwnerLocation_WhenLinkedTaskDefinitionIsMissing_ReportsOwningChangeError(t *testing.T) {
	// Arrange
	workspaceRoot, apiRoot, _, _, _ := writeWorkspaceThreadFixture(t)
	workspace, err := readWorkspaceManifest(workspaceRoot)
	require.NoError(t, err)
	require.NoError(t, os.Remove(workspaceThreadDefinitionPath(workspaceRoot, workspace, "COM-C014")))
	cmd := NewThreadCmd()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--repo", apiRoot}))

	// Act
	_, resolveErr := resolveThreadOwnerLocation(cmd, "api-task", threadStatusOptions{Dir: defaultTaskWorkspaceDir})

	// Assert
	require.Error(t, resolveErr)
	assert.ErrorContains(t, resolveErr, "workspace change \"COM-C014\" has no thread definition")
}

func TestInferThreadOwnerLocation_WhenCompletedDefinitionAndActiveDefinitionExist_SelectsActiveTask(t *testing.T) {
	// Arrange
	repoRoot := t.TempDir()
	_, completedWorkspace, _ := writeRepoThreadFixtureAt(t, repoRoot, "completed-task")
	completedPath := filepath.Join(completedWorkspace, taskManifestFilename)
	completed, err := readTaskManifest(completedPath)
	require.NoError(t, err)
	completed.Artifacts.Slices[0].Stage = "validated"
	completed.Artifacts.Slices[0].Decision = "promote"
	completed.Artifacts.Slices[1].Stage = "validated"
	completed.Artifacts.Slices[1].Decision = "promote"
	completed.Artifacts.Slices[2].Stage = "validated"
	completed.Artifacts.Slices[2].Decision = "promote"
	require.NoError(t, writeTaskManifest(completedPath, completed))
	definitionPath := repoThreadDefinitionPath(completedWorkspace)
	definition, definitionErr := readThreadDefinition(definitionPath)
	require.NoError(t, definitionErr)
	definition.Threads[1].Targets = append(definition.Threads[1].Targets, threadTargetReference{Target: "F03"})
	require.NoError(t, writeThreadDefinition(definitionPath, definition))
	_, _, activeLocation := writeRepoThreadFixtureAt(t, repoRoot, "active-task")
	cmd := NewThreadCmd()
	cmd.SetContext(t.Context())
	require.NoError(t, cmd.ParseFlags([]string{"--repo", repoRoot}))

	// Act
	resolved, resolveErr := inferThreadOwnerLocation(cmd, threadStatusOptions{Dir: defaultTaskWorkspaceDir})

	// Assert
	require.NoError(t, resolveErr)
	assert.Equal(t, activeLocation.OwnerID, resolved.OwnerID)
	assert.Equal(t, threadOwnerTask, resolved.Kind)
	assert.Equal(t, "active-task", resolved.TaskID)
}

func TestThreadStatus_WhenRepoProjectionIsMissing_PrintsRunnableAndWaitingThreads(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	repoRoot, _, _ := writeRepoThreadFixture(t, "thread-task")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewThreadCmd()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"thread-task", "--repo", repoRoot})

	// Act
	err := cmd.ExecuteContext(t.Context())

	// Assert
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Task: thread-task")
	assert.Contains(t, stdout.String(), "Ready threads")
	assert.Contains(t, stdout.String(), "agent-a")
	assert.Contains(t, stdout.String(), "F01")
	assert.Contains(t, stdout.String(), "Waiting threads")
	assert.Contains(t, stdout.String(), "both-a")
	assert.Contains(t, stdout.String(), "after agent-a (ready)")
	assert.Contains(t, stdout.String(), "Unassigned targets")
	assert.Contains(t, stdout.String(), "F03")
	assert.Empty(t, stderr.String())
}

func TestWriteThreadStatus_WhenJSONRequested_IsDeterministicAcrossCalls(t *testing.T) {
	// Arrange
	_, _, location := writeRepoThreadFixture(t, "thread-task")
	snapshot, _, err := loadThreadOwnerSnapshot(location, nil)
	require.NoError(t, err)
	out, err := buildThreadStatus(snapshot)
	require.NoError(t, err)
	first := &bytes.Buffer{}
	firstCmd := &cobra.Command{}
	firstCmd.SetOut(first)
	second := &bytes.Buffer{}
	secondCmd := &cobra.Command{}
	secondCmd.SetOut(second)

	// Act
	firstErr := writeThreadStatus(firstCmd, out, true)
	secondErr := writeThreadStatus(secondCmd, out, true)

	// Assert
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, first.String(), second.String())
}

func writeRepoThreadFixture(t testing.TB, taskID string) (string, string, threadOwnerLocation) {
	t.Helper()
	return writeRepoThreadFixtureAt(t, t.TempDir(), taskID)
}

func writeRepoThreadFixtureAt(t testing.TB, repoRoot, taskID string) (string, string, threadOwnerLocation) {
	t.Helper()
	writeThreadGitFixture(t, repoRoot)
	taskWorkspace := taskWorkspacePath(repoRoot, defaultTaskWorkspaceDir, taskID)
	require.NoError(t, os.MkdirAll(taskWorkspace, 0o755))
	manifest := validThreadTaskManifest()
	manifest.TaskID = taskID
	manifest.RepoRoot = repoRoot
	manifest.Workspace = taskWorkspace
	require.NoError(t, writeTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename), manifest))
	definition := validRepoThreadDefinition()
	definition.Owner.TaskID = taskID
	require.NoError(t, writeThreadDefinition(repoThreadDefinitionPath(taskWorkspace), definition))
	return repoRoot, taskWorkspace, repoThreadOwnerLocation(repoRoot, taskWorkspace, manifest)
}

func writeThreadCheckpointFixture(t testing.TB, workspace, taskID, checkpointID, target, stage, decision string) {
	t.Helper()
	record := taskCheckpointRecord{
		SchemaVersion: taskCheckpointSchemaVersion,
		EventKind:     taskCheckpointEventKind,
		CheckpointID:  checkpointID,
		TaskID:        taskID,
		Target:        target,
		Stage:         stage,
		Decision:      decision,
		CreatedAt:     "2026-08-12T14:00:00Z",
	}
	data, err := json.MarshalIndent(record, "", "  ")
	require.NoError(t, err)
	checkpointDir := filepath.Join(workspace, "checkpoints")
	require.NoError(t, os.MkdirAll(checkpointDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(checkpointDir, checkpointID+".json"), append(data, '\n'), 0o644))
}

func writeWorkspaceThreadFixture(t testing.TB) (string, string, string, string, string) {
	t.Helper()
	workspaceRoot := t.TempDir()
	apiRoot := filepath.Join(workspaceRoot, "api")
	webRoot := filepath.Join(workspaceRoot, "web")
	require.NoError(t, os.MkdirAll(apiRoot, 0o755))
	require.NoError(t, os.MkdirAll(webRoot, 0o755))
	workspace := workspaceManifest{
		ID:          "umbrella",
		Name:        "Umbrella",
		ArtifactDir: defaultWorkspaceArtifactDir,
		Repos: map[string]workspaceRepo{
			"api": {Path: "./api"},
			"web": {Path: "./web"},
		},
	}
	require.NoError(t, os.MkdirAll(workspaceChangesDir(workspaceRoot, workspace), 0o755))
	require.NoError(t, writeWorkspaceManifest(workspaceManifestPath(workspaceRoot), workspace))
	changeBody := "---\nid: COM-C014\ntype: change\nworkspace: umbrella\nstatus: active\ntitle: Checkout\nrequired_repos: [api, web]\n---\n\n# Checkout\n\n## Repo Slices\n| Repo | Task ID | Target | Name | Status |\n| --- | --- | --- | --- | --- |\n| `api` | `api-task` | `A01` | API | `planned` |\n| `web` | `web-task` | `B01` | Web | `planned` |\n"
	require.NoError(t, os.WriteFile(filepath.Join(workspaceChangesDir(workspaceRoot, workspace), "COM-C014-checkout.md"), []byte(changeBody), 0o644))
	apiWorkspace := writeLinkedThreadTaskFixture(t, apiRoot, workspaceRoot, "api", "api-task", "A", "A01")
	webWorkspace := writeLinkedThreadTaskFixture(t, webRoot, workspaceRoot, "web", "web-task", "B", "B01")
	definition := threadDefinition{
		SchemaVersion: 1,
		Revision:      1,
		Owner: threadDefinitionOwner{
			Kind: threadOwnerWorkspaceChange, WorkspaceID: "umbrella", ChangeID: "COM-C014",
		},
		Threads: []threadDefinitionLane{{
			Key: "both", Targets: []threadTargetReference{
				{Repo: "api", Task: "api-task", Target: "A01"},
				{Repo: "web", Task: "web-task", Target: "B01"},
			},
		}},
	}
	require.NoError(t, writeThreadDefinition(workspaceThreadDefinitionPath(workspaceRoot, workspace, "COM-C014"), definition))
	return workspaceRoot, apiRoot, apiWorkspace, webRoot, webWorkspace
}

func writeLinkedThreadTaskFixture(t testing.TB, repoRoot, workspaceRoot, repoAlias, taskID, series, target string) string {
	t.Helper()
	writeThreadGitFixture(t, repoRoot)
	taskWorkspace := taskWorkspacePath(repoRoot, defaultTaskWorkspaceDir, taskID)
	require.NoError(t, os.MkdirAll(taskWorkspace, 0o755))
	manifest := validThreadTaskManifest()
	manifest.TaskID = taskID
	manifest.Series = series
	manifest.Artifacts.Series = series
	manifest.Artifacts.Slices = []taskSliceArtifact{{ID: target, Title: repoAlias + " work"}}
	manifest.RepoRoot = repoRoot
	manifest.Workspace = taskWorkspace
	manifest.WorkspaceID = "umbrella"
	manifest.WorkspaceRoot = workspaceRoot
	manifest.ParentChange = "COM-C014"
	manifest.RepoAlias = repoAlias
	require.NoError(t, writeTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename), manifest))
	return taskWorkspace
}

func writeThreadGitFixture(t testing.TB, repoRoot string) {
	t.Helper()
	gitDir := filepath.Join(repoRoot, ".git")
	require.NoError(t, os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n\tbare = false\n"), 0o644))
}
