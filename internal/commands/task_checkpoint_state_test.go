package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskCheckpointHeads_WhenLegacyEventsExist_UsesDeterministicLinearOrder(t *testing.T) {
	// Arrange
	events := []taskCheckpointEvent{
		checkpointEventFixture(1, "cp-old", "2026-08-12T10:00:00Z", nil),
		checkpointEventFixture(2, "cp-new", "2026-08-12T11:00:00Z", nil),
	}

	// Act
	resolved, err := resolveTaskCheckpointHeads(events)

	// Assert
	require.NoError(t, err)
	heads := resolved["f01"]
	assert.Len(t, heads, 1)
	assert.Equal(t, "cp-new", heads[0].Record.CheckpointID)
}

func TestResolveTaskCheckpointHeads_WhenV3EventSupersedesLegacyHead_UsesV3Event(t *testing.T) {
	// Arrange
	events := []taskCheckpointEvent{
		checkpointEventFixture(2, "cp-legacy", "2026-08-12T10:00:00Z", nil),
		checkpointEventFixture(3, "cp-current", "2026-08-12T11:00:00Z", []string{"cp-legacy"}),
	}

	// Act
	resolved, err := resolveTaskCheckpointHeads(events)

	// Assert
	require.NoError(t, err)
	heads := resolved["f01"]
	assert.Len(t, heads, 1)
	assert.Equal(t, "cp-current", heads[0].Record.CheckpointID)
}

func TestResolveTaskCheckpointHeads_WhenV3EventsDiverge_PreservesBothHeads(t *testing.T) {
	// Arrange
	events := []taskCheckpointEvent{
		checkpointEventFixture(2, "cp-root", "2026-08-12T10:00:00Z", nil),
		checkpointEventFixture(3, "cp-agent-a", "2026-08-12T11:00:00Z", []string{"cp-root"}),
		checkpointEventFixture(3, "cp-agent-b", "2026-08-12T11:00:01Z", []string{"cp-root"}),
	}

	// Act
	resolved, err := resolveTaskCheckpointHeads(events)

	// Assert
	require.NoError(t, err)
	heads := resolved["f01"]
	assert.Len(t, heads, 2)
	assert.Equal(t, "cp-agent-a", heads[0].Record.CheckpointID)
	assert.Equal(t, "cp-agent-b", heads[1].Record.CheckpointID)
}

func TestCheckpointSupersedesForWrite_WhenHeadsConflict_RequiresExplicitResolution(t *testing.T) {
	// Arrange
	events := []taskCheckpointEvent{
		checkpointEventFixture(3, "cp-agent-a", "2026-08-12T11:00:00Z", nil),
		checkpointEventFixture(3, "cp-agent-b", "2026-08-12T11:00:01Z", nil),
	}

	// Act
	_, err := checkpointSupersedesForWrite(events, "F01", nil)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "conflicting checkpoint heads")
	assert.ErrorContains(t, err, "--supersedes")
}

func TestCheckpointSupersedesForWrite_WhenAllHeadsAreNamed_AcceptsResolution(t *testing.T) {
	// Arrange
	events := []taskCheckpointEvent{
		checkpointEventFixture(3, "cp-agent-a", "2026-08-12T11:00:00Z", nil),
		checkpointEventFixture(3, "cp-agent-b", "2026-08-12T11:00:01Z", nil),
	}

	// Act
	supersedes, err := checkpointSupersedesForWrite(events, "F01", []string{"cp-agent-b", "cp-agent-a"})

	// Assert
	require.NoError(t, err)
	assert.Len(t, supersedes, 2)
	assert.Equal(t, "cp-agent-a", supersedes[0])
	assert.Equal(t, "cp-agent-b", supersedes[1])
}

func TestReconcileTaskManifestLifecycle_WhenEventDisagrees_EventBecomesAuthoritative(t *testing.T) {
	// Arrange
	workspace := t.TempDir()
	checkpointDir := filepath.Join(workspace, "checkpoints")
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(checkpointDir, "20260812-110000-validated.json"),
		checkpointRecordFixture(3, "cp-current", "2026-08-12T11:00:00Z", nil),
	))
	manifest := validThreadTaskManifest()
	manifest.Artifacts.Slices[0].Stage = "implemented"
	manifest.Artifacts.Slices[0].Decision = "improve"

	// Act
	reconciled, err := reconcileTaskManifestLifecycle(workspace, manifest)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "validated", reconciled.Artifacts.Slices[0].Stage)
	assert.Equal(t, "promote", reconciled.Artifacts.Slices[0].Decision)
	assert.Equal(t, "cp-current", reconciled.Artifacts.Slices[0].LatestCheckpointID)
	assert.Len(t, reconciled.LifecycleDiagnostics, 2)
	assert.Contains(t, reconciled.LifecycleDiagnostics[0], "is authoritative")
	assert.Contains(t, reconciled.LifecycleDiagnostics[1], "no Markdown projection")
}

func TestReconcileTaskManifestLifecycle_WhenManifestWasRefreshedAfterCheckpoint_PreservesRefreshTimestamp(t *testing.T) {
	// Arrange
	workspace := t.TempDir()
	checkpointDir := filepath.Join(workspace, "checkpoints")
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(checkpointDir, "20260812-110000-validated.json"),
		checkpointRecordFixture(3, "cp-current", "2026-08-12T11:00:00Z", nil),
	))
	manifest := validThreadTaskManifest()
	manifest.UpdatedAt = "2026-08-12T12:00:00Z"

	// Act
	reconciled, err := reconcileTaskManifestLifecycle(workspace, manifest)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "2026-08-12T12:00:00Z", reconciled.UpdatedAt)
}

func TestReconcileTaskManifestLifecycle_WhenHeadsConflict_ReportsConflictWithoutChoosingWinner(t *testing.T) {
	// Arrange
	workspace := t.TempDir()
	checkpointDir := filepath.Join(workspace, "checkpoints")
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(checkpointDir, "20260812-110000-agent-a.json"),
		checkpointRecordFixture(3, "cp-agent-a", "2026-08-12T11:00:00Z", nil),
	))
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(checkpointDir, "20260812-110001-agent-b.json"),
		checkpointRecordFixture(3, "cp-agent-b", "2026-08-12T11:00:01Z", nil),
	))
	manifest := validThreadTaskManifest()

	// Act
	reconciled, err := reconcileTaskManifestLifecycle(workspace, manifest)

	// Assert
	require.NoError(t, err)
	assert.Empty(t, reconciled.Artifacts.Slices[0].Stage)
	assert.Empty(t, reconciled.Artifacts.Slices[0].Decision)
	assert.Len(t, reconciled.LifecycleConflicts, 1)
	assert.Equal(t, "F01", reconciled.LifecycleConflicts[0].Target)
	assert.Len(t, reconciled.LifecycleConflicts[0].HeadCheckpointIDs, 2)
	assert.Equal(t, "cp-agent-a", reconciled.LifecycleConflicts[0].HeadCheckpointIDs[0])
	assert.Equal(t, "cp-agent-b", reconciled.LifecycleConflicts[0].HeadCheckpointIDs[1])
}

func TestTaskCheckpoint_WhenTwoCommandsRunConcurrently_PublishesOneCausalChain(t *testing.T) {
	// Arrange
	repoDir := setupBoundaryTask(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "boundary-test")
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	first := NewTaskCmd()
	first.SetArgs([]string{"checkpoint", "boundary-test", "--target", "A01", "--stage", "implemented", "--decision", "continue", "--description", "agent a", "--index=false", "--repo", repoDir, "--json"})
	first.SetOut(&bytes.Buffer{})
	second := NewTaskCmd()
	second.SetArgs([]string{"checkpoint", "boundary-test", "--target", "A01", "--stage", "validated", "--decision", "promote", "--description", "agent b", "--index=false", "--repo", repoDir, "--json"})
	second.SetOut(&bytes.Buffer{})
	go func() {
		defer workers.Done()
		<-start
		results <- first.Execute()
	}()
	go func() {
		defer workers.Done()
		<-start
		results <- second.Execute()
	}()

	// Act
	close(start)
	workers.Wait()
	firstErr := <-results
	secondErr := <-results
	events, readErr := readTaskCheckpointEvents(workspace, "boundary-test")
	resolved, resolveErr := resolveTaskCheckpointHeads(events)

	// Assert
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NoError(t, readErr)
	require.NoError(t, resolveErr)
	assert.Len(t, events, 2)
	heads := resolved["a01"]
	assert.Len(t, heads, 1)
	assert.Len(t, heads[0].Record.SupersedesCheckpointIDs, 1)
}

func TestTaskStatus_WhenCheckpointEventDisagreesWithManifest_ReportsEventAuthority(t *testing.T) {
	// Arrange
	repoDir := setupBoundaryTask(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "boundary-test")
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(workspace, "checkpoints", "20260812-110000-validated.json"),
		taskCheckpointRecord{
			SchemaVersion: taskCheckpointSchemaVersion,
			EventKind:     taskCheckpointEventKind,
			CheckpointID:  "cp-current",
			TaskID:        "boundary-test",
			Target:        "A01",
			Slice:         "A01",
			Stage:         "validated",
			Decision:      "promote",
			CreatedAt:     "2026-08-12T11:00:00Z",
		},
	))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "boundary-test", "--repo", repoDir, "--json"})
	output := &bytes.Buffer{}
	cmd.SetOut(output)

	// Act
	err := cmd.Execute()
	var status taskStatusOutput
	decodeErr := json.Unmarshal(output.Bytes(), &status)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Len(t, status.LifecycleDiagnostics, 2)
	assert.Contains(t, status.LifecycleDiagnostics[0], "is authoritative")
	assert.Contains(t, status.LifecycleDiagnostics[1], "no Markdown projection")
	assert.Len(t, status.Slices, 2)
	assert.Equal(t, "validated", status.Slices[0].Stage)
	assert.Equal(t, "promote", status.Slices[0].Decision)
}

func TestReadTaskCheckpointEvents_WhenV3EventHasNoID_ReturnsError(t *testing.T) {
	// Arrange
	workspace := t.TempDir()
	require.NoError(t, writeTaskCheckpointRecord(
		filepath.Join(workspace, "checkpoints", "missing-id.json"),
		taskCheckpointRecord{
			SchemaVersion: taskCheckpointSchemaVersion,
			EventKind:     taskCheckpointEventKind,
			TaskID:        "threaded-task",
			Target:        "F01",
			Stage:         "validated",
			Decision:      "promote",
			CreatedAt:     "2026-08-12T11:00:00Z",
		},
	))

	// Act
	_, err := readTaskCheckpointEvents(workspace, "threaded-task")

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "has no checkpoint_id")
}

func TestReadTaskCheckpointRecord_WhenLegacyEventHasNoID_UsesFilenameStem(t *testing.T) {
	// Arrange
	workspace := t.TempDir()
	path := filepath.Join(workspace, "checkpoints", "20260812-110000-validated.json")
	require.NoError(t, writeTaskCheckpointRecord(path, taskCheckpointRecord{
		SchemaVersion: 2,
		TaskID:        "threaded-task",
		Target:        "F01",
		Stage:         "validated",
		Decision:      "promote",
		CreatedAt:     "2026-08-12T11:00:00Z",
	}))

	// Act
	record, err := readTaskCheckpointRecord(path)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "20260812-110000-validated", record.CheckpointID)
}

func checkpointEventFixture(schema int, checkpointID, createdAt string, supersedes []string) taskCheckpointEvent {
	return taskCheckpointEvent{
		Record: checkpointRecordFixture(schema, checkpointID, createdAt, supersedes),
		JSONPath: filepath.Join(
			"checkpoints",
			checkpointID+".json",
		),
	}
}

func checkpointRecordFixture(schema int, checkpointID, createdAt string, supersedes []string) taskCheckpointRecord {
	record := taskCheckpointRecord{
		SchemaVersion:           schema,
		CheckpointID:            checkpointID,
		TaskID:                  "threaded-task",
		Target:                  "F01",
		Slice:                   "F01",
		Stage:                   "validated",
		Decision:                "promote",
		CreatedAt:               createdAt,
		SupersedesCheckpointIDs: supersedes,
	}
	if schema == taskCheckpointSchemaVersion {
		record.EventKind = taskCheckpointEventKind
	}
	return record
}
