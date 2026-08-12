package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildThreadStatus_WhenRepoTargetsArePending_ReportsReadyWaitingAndUnassigned(t *testing.T) {
	// Arrange
	snapshot := repoThreadSnapshotFixture()

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, threadOwnerTask, status.Owner.Kind)
	assert.Equal(t, "threaded-task", status.Owner.TaskID)
	assert.Len(t, status.Threads, 2)
	assert.Equal(t, threadStateReady, status.Threads[0].State)
	require.NotNil(t, status.Threads[0].CurrentTarget)
	assert.Equal(t, "F01", status.Threads[0].CurrentTarget.Target)
	assert.Equal(t, threadStateWaiting, status.Threads[1].State)
	assert.Len(t, status.Threads[1].Reasons, 1)
	assert.Equal(t, "after agent-a (ready)", status.Threads[1].Reasons[0])
	assert.Len(t, status.ReadyThreads, 1)
	assert.Equal(t, "agent-a", status.ReadyThreads[0])
	assert.Empty(t, status.ActiveThreads)
	assert.Len(t, status.UnassignedTargets, 1)
	assert.Equal(t, "F03", status.UnassignedTargets[0].Target)
}

func TestBuildThreadStatus_WhenDependencyCheckpointIsPromoted_ReportsJoinReady(t *testing.T) {
	// Arrange
	snapshot := repoThreadSnapshotFixture()
	snapshot.Tasks[0].Events = []taskCheckpointEvent{
		checkpointEventFixture(3, "cp-f01", "2026-08-12T11:00:00Z", nil),
	}

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Len(t, status.Threads, 2)
	assert.Equal(t, threadStateCompleted, status.Threads[0].State)
	assert.Nil(t, status.Threads[0].CurrentTarget)
	assert.Equal(t, threadStateReady, status.Threads[1].State)
	require.NotNil(t, status.Threads[1].CurrentTarget)
	assert.Equal(t, "F02", status.Threads[1].CurrentTarget.Target)
	assert.Len(t, status.ReadyThreads, 1)
	assert.Equal(t, "both-a", status.ReadyThreads[0])
}

func TestBuildThreadStatus_WhenTargetNeedsImprovementWithoutFollowUp_BlocksLane(t *testing.T) {
	// Arrange
	snapshot := repoThreadSnapshotFixture()
	event := checkpointEventFixture(3, "cp-f01", "2026-08-12T11:00:00Z", nil)
	event.Record.Stage = "implemented"
	event.Record.Decision = "improve"
	snapshot.Tasks[0].Events = []taskCheckpointEvent{event}

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, threadStateBlocked, status.Threads[0].State)
	assert.Len(t, status.Threads[0].Reasons, 1)
	assert.Equal(t, "F01 ended with improve", status.Threads[0].Reasons[0])
	assert.Equal(t, threadStateWaiting, status.Threads[1].State)
	assert.Empty(t, status.ReadyThreads)
}

func TestBuildThreadStatus_WhenPromotedFollowUpResolvesImprovement_CompletesLane(t *testing.T) {
	// Arrange
	snapshot := repoThreadSnapshotFixture()
	snapshot.Tasks[0].Manifest.Artifacts.Slices = append(snapshot.Tasks[0].Manifest.Artifacts.Slices, taskSliceArtifact{
		ID:       "F01-1",
		Title:    "Improve first",
		Kind:     "follow_up",
		ParentID: "F01",
	})
	base := checkpointEventFixture(3, "cp-f01", "2026-08-12T11:00:00Z", nil)
	base.Record.Stage = "implemented"
	base.Record.Decision = "improve"
	followUp := checkpointEventFixture(3, "cp-f01-1", "2026-08-12T12:00:00Z", nil)
	followUp.Record.Target = "F01-1"
	followUp.Record.Slice = "F01-1"
	followUp.Record.Stage = "validated"
	followUp.Record.Decision = "promote"
	snapshot.Tasks[0].Events = []taskCheckpointEvent{base, followUp}

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, threadStateCompleted, status.Threads[0].State)
	assert.Equal(t, threadStateReady, status.Threads[1].State)
	assert.Len(t, status.Threads[0].Targets, 1)
	assert.Equal(t, "F01-1", status.Threads[0].Targets[0].Target)
	assert.Equal(t, "F01", status.Threads[0].Targets[0].DefinitionTarget)
}

func TestBuildThreadStatus_WhenCheckpointHeadsConflict_BlocksLaneWithoutChoosingWinner(t *testing.T) {
	// Arrange
	snapshot := repoThreadSnapshotFixture()
	snapshot.Tasks[0].Events = []taskCheckpointEvent{
		checkpointEventFixture(3, "cp-agent-a", "2026-08-12T11:00:00Z", nil),
		checkpointEventFixture(3, "cp-agent-b", "2026-08-12T11:00:01Z", nil),
	}

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, threadStateBlocked, status.Threads[0].State)
	require.NotNil(t, status.Threads[0].CurrentTarget)
	assert.True(t, status.Threads[0].CurrentTarget.Conflicted)
	assert.Len(t, status.Threads[0].CurrentTarget.ConflictHeads, 2)
	assert.Equal(t, "cp-agent-a", status.Threads[0].CurrentTarget.ConflictHeads[0])
	assert.Equal(t, "cp-agent-b", status.Threads[0].CurrentTarget.ConflictHeads[1])
	assert.Equal(t, threadStateWaiting, status.Threads[1].State)
}

func TestBuildThreadStatus_WhenWorkspaceTargetsSpanRepos_UsesQualifiedAddresses(t *testing.T) {
	// Arrange
	snapshot := workspaceThreadSnapshotFixture()

	// Act
	status, err := buildThreadStatus(snapshot)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, threadOwnerWorkspaceChange, status.Owner.Kind)
	assert.Equal(t, "change-1", status.Owner.ChangeID)
	assert.Len(t, status.Threads, 1)
	assert.Equal(t, threadStateReady, status.Threads[0].State)
	assert.Len(t, status.Threads[0].Targets, 2)
	assert.Equal(t, "api", status.Threads[0].Targets[0].RepoAlias)
	assert.Equal(t, "A01", status.Threads[0].Targets[0].Target)
	assert.Equal(t, "web", status.Threads[0].Targets[1].RepoAlias)
	assert.Equal(t, "W01", status.Threads[0].Targets[1].Target)
}

func repoThreadSnapshotFixture() threadOwnerSnapshot {
	return threadOwnerSnapshot{
		OwnerID:        "owner-repo",
		Kind:           threadOwnerTask,
		TaskID:         "threaded-task",
		RepoRoot:       "/repo",
		DefinitionPath: "/repo/devspecs/tasks/threaded-task/threads.yaml",
		Definition:     validRepoThreadDefinition(),
		Tasks: []threadTaskSnapshot{{
			RepoRoot:  "/repo",
			Workspace: "/repo/devspecs/tasks/threaded-task",
			Manifest:  validThreadTaskManifest(),
		}},
	}
}

func workspaceThreadSnapshotFixture() threadOwnerSnapshot {
	context := validWorkspaceThreadValidationContext()
	return threadOwnerSnapshot{
		OwnerID:        "owner-workspace",
		Kind:           threadOwnerWorkspaceChange,
		WorkspaceID:    "umbrella",
		ChangeID:       "change-1",
		WorkspaceRoot:  "/workspace",
		DefinitionPath: "/workspace/devspecs/changes/change-1.threads.yaml",
		Definition:     validWorkspaceThreadDefinition(),
		Tasks: []threadTaskSnapshot{
			{
				RepoAlias: "api",
				RepoRoot:  "/workspace/repos/api",
				Workspace: "/workspace/repos/api/devspecs/tasks/api-task",
				Manifest:  context.TaskManifests[workspaceThreadTaskKey("api", "api-task")],
			},
			{
				RepoAlias: "web",
				RepoRoot:  "/workspace/repos/web",
				Workspace: "/workspace/repos/web/devspecs/tasks/web-task",
				Manifest:  context.TaskManifests[workspaceThreadTaskKey("web", "web-task")],
			},
		},
	}
}
