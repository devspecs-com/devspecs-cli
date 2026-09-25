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

func TestTaskCheckpointDescription_WhenReadFromStdin_StoresStdinTextInRecord(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "-",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("Traced the checkpoint writer end to end.\n"))

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "Traced the checkpoint writer end to end.", record.Description)
}

func TestTaskCheckpointDescription_WhenReadFromFile_StoresFileTextInRecord(t *testing.T) {
	repoDir := setupBSeriesTask(t)
	descriptionPath := filepath.Join(repoDir, "description.txt")
	require.NoError(t, os.WriteFile(descriptionPath, []byte("Description supplied by --description-file.\n"), 0o644))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description-file", descriptionPath,
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "Description supplied by --description-file.", record.Description)
}

func TestTaskCheckpointDescription_WhenFlagAndFileBothSet_ReturnsConflictError(t *testing.T) {
	repoDir := setupBSeriesTask(t)
	descriptionPath := filepath.Join(repoDir, "description.txt")
	require.NoError(t, os.WriteFile(descriptionPath, []byte("from file\n"), 0o644))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "from flag",
		"--description-file", descriptionPath,
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "--description")
	assert.ErrorContains(t, err, "--description-file")
}

func TestTaskCheckpointNote_WhenFlagAndFileBothSet_ReturnsConflictError(t *testing.T) {
	repoDir := setupBSeriesTask(t)
	notePath := filepath.Join(repoDir, "note.txt")
	require.NoError(t, os.WriteFile(notePath, []byte("from file\n"), 0o644))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--note", "from flag",
		"--note-file", notePath,
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "--note")
	assert.ErrorContains(t, err, "--note-file")
}

func TestTaskCheckpointNote_WhenDescriptionAlreadyReadsStdin_ReturnsStdinConsumedError(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "-",
		"--note", "-",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("only one flag may read stdin\n"))

	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, "stdin already consumed by --description", err.Error())
}

func TestTaskCheckpointDescription_WhenStdinIsEmpty_ReturnsEmptyInputError(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "-",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("   \n\n"))

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "--description")
	assert.ErrorContains(t, err, "empty stdin")
}

func TestTaskCheckpointDescription_WhenStdinIsMultiLine_PreservesInteriorNewlines(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "-",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("First paragraph line.\n\n- bullet one\n- bullet two\n"))

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "First paragraph line.\n\n- bullet one\n- bullet two", record.Description)
	assert.Contains(t, mustReadFile(t, out.CheckpointPath), "First paragraph line.\n\n- bullet one\n- bullet two")
}

func TestTaskCheckpointDescription_WhenStdinUsesCRLF_StoresLineFeedsOnly(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--description", "-",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("Windows line one.\r\nWindows line two.\r\n"))

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "Windows line one.\nWindows line two.", record.Description)
}

func TestTaskCheckpointNote_WhenReadFromStdin_StoresStdinTextInRecord(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--note", "-",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("Note read from stdin.\n"))

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "Note read from stdin.", record.Note)
}

func TestTaskCheckpointDraft_WhenDescriptionFromStdin_PreviewsWithoutWritingFiles(t *testing.T) {
	repoDir := setupBSeriesTask(t)
	checkpointsDir := filepath.Join(repoDir, "devspecs", "tasks", "b-series-test", "checkpoints")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--target", "B01",
		"--draft",
		"--description", "-",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetIn(strings.NewReader("Draft description line one.\nDraft description line two.\n"))

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointDraftOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.False(t, out.Mutates)
	assert.Equal(t, "Draft description line one.\nDraft description line two.", out.CheckpointRecord.Description)
	assert.Contains(t, out.CheckpointMarkdown, "Draft description line one.\nDraft description line two.")
	assert.Contains(t, out.ResultAppendMarkdown, "- What changed:\n\n  Draft description line one.\n  Draft description line two.\n- Evidence for decision:")
	assert.NoDirExists(t, checkpointsDir)
}
