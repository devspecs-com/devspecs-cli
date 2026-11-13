package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jsonCheckpointTask = "b-series-test"

func TestTaskCheckpointJSON_WithReceiptEvidence_RecordsCallersDecisionWithoutExecutingTests(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	payload := jsonCheckpointPayload()
	payload["decision"] = "improve"
	payload["description"] = "-"
	payload["tests_run"] = []string{"echo unexpected > json-checkpoint-executed.txt"}
	payload["files_edited"] = []string{"src/example.go"}
	digest := "sha256:" + strings.Repeat("a", 64)
	payload["receipt_evidence"] = map[string]string{
		"dispatch_id":    "dispatch_01KZZW1BBN55VK2GVYQNCQBZB9",
		"receipt_id":     "receipt_01KZZW1KXP4AZ7TSB0NN68HJ86",
		"request_sha256": digest,
		"wire_sha256":    digest,
	}
	cmd, stdout := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	out, event := readJSONCheckpointResult(t, stdout)
	assert.Equal(t, "B01", out.Slice)
	assert.Equal(t, "improve", event.Decision)
	assert.Equal(t, "-", event.Description)
	require.Len(t, event.FilesEdited, 1)
	assert.Equal(t, "src/example.go", event.FilesEdited[0])
	require.Len(t, event.TestsRun, 1)
	assert.Equal(t, "echo unexpected > json-checkpoint-executed.txt", event.TestsRun[0])
	assert.Empty(t, event.Evidence.TestCommands)
	assert.Contains(t, strings.Join(event.Resources, "\n"), "receipt_01KZZW1KXP4AZ7TSB0NN68HJ86")
	assert.Contains(t, strings.Join(event.Resources, "\n"), digest)
	assert.NoFileExists(t, filepath.Join(repo, "json-checkpoint-executed.txt"))
}

func TestTaskCheckpointJSON_WithMatchingHead_AppendsThroughExistingLifecycle(t *testing.T) {
	// Arrange
	setupBSeriesTask(t)
	first := recordJSONCheckpointLegacyHead(t)
	payload := jsonCheckpointPayload()
	payload["expected_heads"] = []string{first.CheckpointID}
	cmd, stdout := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	out, event := readJSONCheckpointResult(t, stdout)
	assert.NotEqual(t, first.CheckpointID, out.CheckpointID)
	require.Len(t, event.SupersedesCheckpointIDs, 1)
	assert.Equal(t, first.CheckpointID, event.SupersedesCheckpointIDs[0])
}

func TestTaskCheckpointJSON_WithStaleEmptyHeads_DoesNotChangeTaskBytes(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	first := recordJSONCheckpointLegacyHead(t)
	before := snapshotJSONCheckpointTask(t, repo)
	existing := mustReadFile(t, first.CheckpointJSONPath)
	cmd, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload())

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "expected_heads is stale")
	before.assertUnchanged(t)
	assert.Equal(t, existing, mustReadFile(t, first.CheckpointJSONPath))
}

func TestTaskCheckpointJSON_WithConcurrentFirstRequests_PublishesOnlyOne(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	first, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload())
	second, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload())
	start := make(chan struct{})
	done := make(chan error, 2)
	submit := func(cmd *cobra.Command) {
		<-start
		done <- cmd.Execute()
	}

	// Act
	go submit(first)
	go submit(second)
	close(start)
	a, b := <-done, <-done

	// Assert
	if a != nil {
		a, b = b, a
	}
	require.NoError(t, a)
	require.ErrorContains(t, b, "expected_heads is stale")
	assert.Equal(t, 1, snapshotJSONCheckpointTask(t, repo).events)
}

func TestTaskCheckpointJSON_WithConflictingHeads_RequiresExplicitSupersedes(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	root := filepath.Join(repo, defaultTaskWorkspaceDir, jsonCheckpointTask, "checkpoints")
	first := writeJSONCheckpointHead(t, root, "cp_20261110T120000Z_b01_validated")
	second := writeJSONCheckpointHead(t, root, "cp_20261110T120001Z_b01_validated")
	before := snapshotJSONCheckpointTask(t, repo)
	payload := jsonCheckpointPayload()
	payload["expected_heads"] = []string{second, first}
	cmd, _ := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "conflicting checkpoint heads")
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithExplicitSupersedes_ResolvesCurrentConflict(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	root := filepath.Join(repo, defaultTaskWorkspaceDir, jsonCheckpointTask, "checkpoints")
	first := writeJSONCheckpointHead(t, root, "cp_20261110T120000Z_b01_validated")
	second := writeJSONCheckpointHead(t, root, "cp_20261110T120001Z_b01_validated")
	payload := jsonCheckpointPayload()
	payload["expected_heads"] = []string{second, first}
	payload["supersedes"] = []string{first, second}
	cmd, stdout := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	_, event := readJSONCheckpointResult(t, stdout)
	require.Len(t, event.SupersedesCheckpointIDs, 2)
	assert.Equal(t, first, event.SupersedesCheckpointIDs[0])
	assert.Equal(t, second, event.SupersedesCheckpointIDs[1])
}

func TestTaskCheckpointJSON_WithTextStdinFlag_RejectsMixedInput(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	cmd, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload(), "--description", "-")

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "--input-json cannot be combined with --description")
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithCaptureTestOutputFlag_DoesNotRunEvidence(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	cmd, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload(), "--capture-test-output")

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "--input-json cannot be combined with --capture-test-output")
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithMismatchedTaskID_DoesNotChangeTaskBytes(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	payload := jsonCheckpointPayload()
	payload["task_id"] = "different-task"
	cmd, _ := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "task_id must exactly match")
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithNoncanonicalTargetID_DoesNotChangeTaskBytes(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	payload := jsonCheckpointPayload()
	payload["target"] = "b01"
	cmd, _ := newJSONCheckpointCommand(t, payload)

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "requires the exact task and target IDs")
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithProviderReceipt_RejectsWithoutMutation(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	cmd, _ := newJSONCheckpointCommand(t, map[string]any{
		"schema": "devspecs.dispatch.receipt", "schema_version": 2,
		"receipt_id": "receipt_01KZZW1KXP4AZ7TSB0NN68HJ86",
	})

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithDraft_RendersWithoutMutation(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	before := snapshotJSONCheckpointTask(t, repo)
	cmd, stdout := newJSONCheckpointCommand(t, jsonCheckpointPayload(), "--draft")

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	var out taskCheckpointDraftOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	assert.True(t, out.Draft)
	assert.False(t, out.Mutates)
	assert.Equal(t, "promote", out.CheckpointRecord.Decision)
	before.assertUnchanged(t)
}

func TestTaskCheckpointJSON_WithStaleDraft_RejectsWithoutMutation(t *testing.T) {
	// Arrange
	repo := setupBSeriesTask(t)
	recordJSONCheckpointLegacyHead(t)
	before := snapshotJSONCheckpointTask(t, repo)
	cmd, _ := newJSONCheckpointCommand(t, jsonCheckpointPayload(), "--draft")

	// Act
	err := cmd.Execute()

	// Assert
	require.ErrorContains(t, err, "expected_heads is stale")
	before.assertUnchanged(t)
}

func TestReadJSONCheckpointDurableArtifact_WithOversizedDoc_Rejects(t *testing.T) {
	// Arrange
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "record.md"), make([]byte, (1<<20)+1), 0o600))

	// Act
	body, err := readJSONCheckpointDurableArtifact(repo, "record.md")

	// Assert
	require.ErrorContains(t, err, "at most 1 MiB")
	assert.Nil(t, body)
}

func jsonCheckpointPayload() map[string]any {
	return map[string]any{
		"schema": "devspecs.task.checkpoint_input", "schema_version": 1,
		"task_id": jsonCheckpointTask, "target": "B01", "stage": "validated",
		"decision": "promote", "expected_heads": []string{},
	}
}

func newJSONCheckpointCommand(t *testing.T, payload map[string]any, extra ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	input, err := json.Marshal(payload)
	require.NoError(t, err)
	cmd := NewTaskCmd()
	cmd.SetArgs(append([]string{"checkpoint", jsonCheckpointTask, "--input-json", "--json", "--index=false"}, extra...))
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(bytes.NewReader(input))
	return cmd, &stdout
}

func recordJSONCheckpointLegacyHead(t *testing.T) taskCheckpointOutput {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"checkpoint", jsonCheckpointTask, "--target", "B01", "--index=false", "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	require.NoError(t, cmd.Execute())
	out, _ := readJSONCheckpointResult(t, &stdout)
	return out
}

func readJSONCheckpointResult(t *testing.T, stdout *bytes.Buffer) (taskCheckpointOutput, taskCheckpointRecord) {
	t.Helper()
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out))
	var event taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &event))
	return out, event
}

func writeJSONCheckpointHead(t *testing.T, dir, id string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	record := checkpointRecordFixture(taskCheckpointSchemaVersion, id, "2026-11-10T12:00:00Z", nil)
	record.TaskID, record.Target, record.Slice = jsonCheckpointTask, "B01", "B01"
	require.NoError(t, writeTaskCheckpointRecord(filepath.Join(dir, id+".json"), record))
	return id
}

type jsonCheckpointTaskSnapshot struct {
	workspace, manifest, resultPath, result string
	events                                  int
}

func snapshotJSONCheckpointTask(t *testing.T, repo string) jsonCheckpointTaskSnapshot {
	t.Helper()
	ctx, err := loadTaskTargetContextForRepo(defaultTaskWorkspaceDir, jsonCheckpointTask, "B01", repo)
	require.NoError(t, err)
	result := filepath.Join(ctx.Workspace, ctx.Slice.Result)
	events, err := readTaskCheckpointEvents(ctx.Workspace, jsonCheckpointTask)
	require.NoError(t, err)
	return jsonCheckpointTaskSnapshot{
		workspace: ctx.Workspace, manifest: mustReadFile(t, ctx.ManifestPath),
		resultPath: result, result: mustReadFile(t, result), events: len(events),
	}
}

func (s jsonCheckpointTaskSnapshot) assertUnchanged(t *testing.T) {
	t.Helper()
	assert.Equal(t, s.manifest, mustReadFile(t, filepath.Join(s.workspace, taskManifestFilename)))
	assert.Equal(t, s.result, mustReadFile(t, s.resultPath))
	events, err := readTaskCheckpointEvents(s.workspace, jsonCheckpointTask)
	require.NoError(t, err)
	assert.Len(t, events, s.events)
}
