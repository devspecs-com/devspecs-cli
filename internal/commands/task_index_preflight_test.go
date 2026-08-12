package commands

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStart_WithNewerIndexSchema_DoesNotCreateWorkspace(t *testing.T) {
	repoRoot := setupTaskCommandRepo(t)
	setTaskTestIndexSchemaVersion(t, store.SchemaVersion+1)
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "newer-schema-start")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "newer-schema-start", "--no-refresh", "preserve task files"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assertTaskIndexPreflightError(t, err)
	assert.NoDirExists(t, workspace)
}

func TestTaskSliceAdd_WithNewerIndexSchema_DoesNotCreateSliceOrChangeManifest(t *testing.T) {
	fixture := setupLifecycleTask(t)
	manifestPath := filepath.Join(fixture.Workspace, taskManifestFilename)
	manifestBefore := mustReadFile(t, manifestPath)
	setTaskTestIndexSchemaVersion(t, store.SchemaVersion+1)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"slice", "add", "lifecycle-add-test", "second lifecycle slice"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assertTaskIndexPreflightError(t, err)
	assert.Equal(t, manifestBefore, mustReadFile(t, manifestPath))
	assert.NoFileExists(t, filepath.Join(fixture.Workspace, "B02-second-lifecycle-slice-plan.md"))
	assert.NoFileExists(t, filepath.Join(fixture.Workspace, "B02-second-lifecycle-slice-result.md"))
}

func TestTaskCheckpoint_WithNewerIndexSchema_DoesNotCreateCheckpointOrChangeTaskFiles(t *testing.T) {
	fixture := setupLifecycleTask(t)
	manifestPath := filepath.Join(fixture.Workspace, taskManifestFilename)
	resultPath := filepath.Join(fixture.Workspace, "B01-first-lifecycle-slice-result.md")
	manifestBefore := mustReadFile(t, manifestPath)
	resultBefore := mustReadFile(t, resultPath)
	setTaskTestIndexSchemaVersion(t, store.SchemaVersion+1)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"checkpoint", "lifecycle-add-test", "--target", "B01", "--stage", "validated", "--decision", "promote"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assertTaskIndexPreflightError(t, err)
	assert.Equal(t, manifestBefore, mustReadFile(t, manifestPath))
	assert.Equal(t, resultBefore, mustReadFile(t, resultPath))
	assert.NoDirExists(t, filepath.Join(fixture.Workspace, "checkpoints"))
}

func TestTaskDecide_WithNewerIndexSchema_DoesNotChangeManifest(t *testing.T) {
	fixture := setupLifecycleTask(t)
	manifestPath := filepath.Join(fixture.Workspace, taskManifestFilename)
	manifestBefore := mustReadFile(t, manifestPath)
	setTaskTestIndexSchemaVersion(t, store.SchemaVersion+1)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"decide", "lifecycle-add-test", "--target", "B01", "--decision", "promote"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assertTaskIndexPreflightError(t, err)
	assert.Equal(t, manifestBefore, mustReadFile(t, manifestPath))
}

func TestTaskSliceAdd_WithIndexDisabledAndNewerIndexSchema_CreatesSlice(t *testing.T) {
	fixture := setupLifecycleTask(t)
	setTaskTestIndexSchemaVersion(t, store.SchemaVersion+1)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"slice", "add", "lifecycle-add-test", "second lifecycle slice", "--index=false"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(fixture.Workspace, "B02-second-lifecycle-slice-plan.md"))
	assert.FileExists(t, filepath.Join(fixture.Workspace, "B02-second-lifecycle-slice-result.md"))
}

func setTaskTestIndexSchemaVersion(t *testing.T, schemaVersion int) {
	t.Helper()
	dbPath, err := config.DBPath()
	require.NoError(t, err)
	raw, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", schemaVersion)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
}

func assertTaskIndexPreflightError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	message := err.Error()
	assert.Contains(t, message, "cannot safely update task files")
	assert.Contains(t, message, "Database:")
	assert.Contains(t, message, "Database schema: v")
	assert.Contains(t, message, "CLI:")
	assert.Contains(t, message, "supports schema v")
	assert.Contains(t, message, "Executable:")
	assert.Contains(t, message, "Repository files written: no")
	assert.Contains(t, message, "ds update")
	assert.Contains(t, message, "--index=false")
	assert.NotContains(t, message, "index task artifact")
}

func TestTaskIndexPreflightError_WithNewerSchema_IncludesExecutableAndRecovery(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	base := &store.NewerSchemaError{DatabaseVersion: store.SchemaVersion + 1, SupportedVersion: store.SchemaVersion}

	err := taskIndexPreflightError(dbPath, base)

	require.Error(t, err)
	assertTaskIndexPreflightError(t, err)
	executable, executableErr := os.Executable()
	require.NoError(t, executableErr)
	assert.Contains(t, err.Error(), executable)
}
