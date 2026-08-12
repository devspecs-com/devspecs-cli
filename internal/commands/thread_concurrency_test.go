package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCheckpoint_WhenSeparateRepoLanesPublishAcrossProcesses_RetainsBothEvents(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "off")
	repoRoot, taskWorkspace, location := writeRepoThreadFixture(t, "thread-task")
	manifest, manifestErr := readTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename))
	require.NoError(t, manifestErr)
	manifest.Artifacts.Slices[0].Result = "F01-result.md"
	manifest.Artifacts.Slices[1].Result = "F02-result.md"
	require.NoError(t, writeTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename), manifest))
	definition, definitionErr := readThreadDefinition(location.DefinitionPath)
	require.NoError(t, definitionErr)
	definition.Threads[1].After = nil
	require.NoError(t, writeThreadDefinition(location.DefinitionPath, definition))
	first, firstOutput := concurrentCheckpointProcess(t, home, repoRoot, "thread-task", "F01")
	second, secondOutput := concurrentCheckpointProcess(t, home, repoRoot, "thread-task", "F02")

	// Act
	firstStartErr := first.Start()
	require.NoError(t, firstStartErr)
	secondStartErr := second.Start()
	require.NoError(t, secondStartErr)
	firstWaitErr := first.Wait()
	secondWaitErr := second.Wait()

	// Assert
	require.NoErrorf(t, firstWaitErr, "first checkpoint output:\n%s", firstOutput.String())
	require.NoErrorf(t, secondWaitErr, "second checkpoint output:\n%s", secondOutput.String())
	events, eventsErr := readTaskCheckpointEvents(taskWorkspace, "thread-task")
	require.NoError(t, eventsErr)
	assert.Len(t, events, 2)
	targets := map[string]bool{
		events[0].Record.Target: true,
		events[1].Record.Target: true,
	}
	assert.True(t, targets["F01"])
	assert.True(t, targets["F02"])
	updated, updatedErr := readTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename))
	require.NoError(t, updatedErr)
	firstSlice, firstSliceErr := taskSliceForCheckpoint(updated, "F01")
	require.NoError(t, firstSliceErr)
	secondSlice, secondSliceErr := taskSliceForCheckpoint(updated, "F02")
	require.NoError(t, secondSliceErr)
	assert.Equal(t, "validated", firstSlice.Stage)
	assert.Equal(t, "validated", secondSlice.Stage)
}

func TestTaskCheckpoint_WhenWorkspaceReposPublishAcrossProcesses_RetainsEachRepoEvent(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "off")
	workspaceRoot, apiRoot, apiWorkspace, webRoot, webWorkspace := writeWorkspaceThreadFixture(t)
	prepareConcurrentCheckpointTask(t, apiWorkspace, "api-task", "A01")
	prepareConcurrentCheckpointTask(t, webWorkspace, "web-task", "B01")
	api, apiOutput := concurrentCheckpointProcess(t, home, apiRoot, "api-task", "A01")
	web, webOutput := concurrentCheckpointProcess(t, home, webRoot, "web-task", "B01")

	// Act
	apiStartErr := api.Start()
	require.NoError(t, apiStartErr)
	webStartErr := web.Start()
	require.NoError(t, webStartErr)
	apiWaitErr := api.Wait()
	webWaitErr := web.Wait()

	// Assert
	require.NoErrorf(t, apiWaitErr, "api checkpoint output:\n%s", apiOutput.String())
	require.NoErrorf(t, webWaitErr, "web checkpoint output:\n%s", webOutput.String())
	apiEvents, apiEventsErr := readTaskCheckpointEvents(apiWorkspace, "api-task")
	require.NoError(t, apiEventsErr)
	assert.Len(t, apiEvents, 1)
	assert.Equal(t, "A01", apiEvents[0].Record.Target)
	webEvents, webEventsErr := readTaskCheckpointEvents(webWorkspace, "web-task")
	require.NoError(t, webEventsErr)
	assert.Len(t, webEvents, 1)
	assert.Equal(t, "B01", webEvents[0].Record.Target)
	manifest, manifestErr := readWorkspaceManifest(workspaceRoot)
	require.NoError(t, manifestErr)
	location := workspaceThreadOwnerLocation(workspaceRoot, manifest, "COM-C014")
	snapshot, _, snapshotErr := loadThreadOwnerSnapshot(location, nil)
	require.NoError(t, snapshotErr)
	status, statusErr := buildThreadStatus(snapshot)
	require.NoError(t, statusErr)
	assert.Len(t, status.Threads, 1)
	assert.Equal(t, threadStateCompleted, status.Threads[0].State)
}

func TestTaskCheckpointConcurrentProcess(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_CONCURRENT_CHECKPOINT") != "1" {
		return
	}
	cmd := NewTaskCmd()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{
		"checkpoint", os.Getenv("DEVSPECS_TEST_TASK_ID"),
		"--target", os.Getenv("DEVSPECS_TEST_TARGET"),
		"--stage", "validated",
		"--decision", "promote",
		"--note", "cross-process F06 proof",
		"--index=false",
		"--json",
	})
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func concurrentCheckpointProcess(t testing.TB, home, repoRoot, taskID, target string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestTaskCheckpointConcurrentProcess$")
	command.Dir = repoRoot
	command.Env = append(os.Environ(),
		"DEVSPECS_HOME="+home,
		"DEVSPECS_TELEMETRY=off",
		"DEVSPECS_TEST_CONCURRENT_CHECKPOINT=1",
		"DEVSPECS_TEST_TASK_ID="+taskID,
		"DEVSPECS_TEST_TARGET="+target,
	)
	output := &bytes.Buffer{}
	command.Stdout = output
	command.Stderr = output
	return command, output
}

func prepareConcurrentCheckpointTask(t testing.TB, taskWorkspace, taskID, target string) {
	t.Helper()
	manifest, err := readTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename))
	require.NoError(t, err)
	manifest.Artifacts.Slices[0].Result = target + "-result.md"
	require.NoError(t, writeTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename), manifest))
}
