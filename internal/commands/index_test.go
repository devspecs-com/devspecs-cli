package commands

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexBackupCommand_WithNewerSchemaJSON_CreatesSchemaAgnosticBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	activePath := filepath.Join(home, "devspecs.db")
	database, err := store.Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	raw, err := sql.Open("sqlite", activePath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", store.SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	cmd := NewIndexCmd()
	cmd.SetArgs([]string{"backup", "--json"})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)

	err = cmd.Execute()

	require.NoError(t, err)
	var report indexBackupReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Equal(t, "backup", report.Operation)
	assert.Equal(t, store.SchemaVersion+1, report.Backup.SourceSchema)
	assert.Equal(t, store.IndexBackupRetentionManual, report.Backup.Retention)
	assert.False(t, report.RepositoryFilesWritten)
	assert.FileExists(t, report.Backup.Path)
	assert.FileExists(t, report.Backup.ManifestPath)
}

func TestIndexRebuildCommand_WithNewerSchema_UsesFullColdScanAndPreservesNewerBackup(t *testing.T) {
	repoRoot := setupE2ERepo(t)
	home := os.Getenv("DEVSPECS_HOME")
	activePath := filepath.Join(home, "devspecs.db")
	database, err := store.Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	raw, err := sql.Open("sqlite", activePath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", store.SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	cmd := NewIndexCmd()
	cmd.SetArgs([]string{"rebuild", "--path", repoRoot, "--json"})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)

	err = cmd.Execute()

	require.NoError(t, err)
	var report indexReplacementReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Equal(t, "rebuild", report.Operation)
	assert.Equal(t, repoRoot, report.RebuiltRoot)
	assert.Equal(t, store.SchemaVersion, report.Replacement.ActiveSchema)
	require.NotNil(t, report.Replacement.DisplacedIndexBackup)
	assert.Equal(t, store.SchemaVersion+1, report.Replacement.DisplacedIndexBackup.SourceSchema)
	active, err := store.Open(activePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, active.Close()) })
	var artifactCount int
	require.NoError(t, active.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&artifactCount))
	assert.Positive(t, artifactCount)
}

func TestScanRebuild_WithNewerSchema_UsesSafeReplacementAndPreservesNewerBackup(t *testing.T) {
	repoRoot := setupE2ERepo(t)
	home := os.Getenv("DEVSPECS_HOME")
	activePath := filepath.Join(home, "devspecs.db")
	database, err := store.Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	raw, err := sql.Open("sqlite", activePath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", store.SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--rebuild", "--path", repoRoot, "--json"})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.Execute()

	require.NoError(t, err)
	var scanResult map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &scanResult))
	assert.Contains(t, scanResult, "Found")
	active, err := store.ValidateIndex(t.Context(), activePath)
	require.NoError(t, err)
	assert.Equal(t, store.SchemaVersion, active.Schema)
	backupPaths, err := filepath.Glob(filepath.Join(home, "backups", "index", "*.db"))
	require.NoError(t, err)
	require.Len(t, backupPaths, 1)
	backup, err := store.ValidateIndex(t.Context(), backupPaths[0])
	require.NoError(t, err)
	assert.Equal(t, store.SchemaVersion+1, backup.Schema)
}

func TestScanRebuild_WhenReplacementScanFails_LeavesActiveIndexUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	activePath := filepath.Join(home, "devspecs.db")
	database, err := store.Open(activePath)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('original', '/tmp/original', '2026-08-12T00:00:00Z', '2026-08-12T00:00:00Z')`)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	missingRepo := filepath.Join(home, "missing-repository")
	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--rebuild", "--path", missingRepo, "--json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err = cmd.Execute()

	require.Error(t, err)
	reopened, err := store.Open(activePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	var originalCount int
	require.NoError(t, reopened.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = 'original'`).Scan(&originalCount))
	assert.Equal(t, 1, originalCount)
	backupPaths, err := filepath.Glob(filepath.Join(home, "backups", "index", "*.db"))
	require.NoError(t, err)
	assert.Empty(t, backupPaths)
}

func TestIndexRestoreCommand_WithOlderSnapshotJSON_ReportsExactCompatibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	activePath := filepath.Join(home, "devspecs.db")
	active, err := store.Open(activePath)
	require.NoError(t, err)
	require.NoError(t, active.Close())
	olderPath := filepath.Join(home, "older.db")
	older, err := store.Open(olderPath)
	require.NoError(t, err)
	_, err = older.Exec("UPDATE schema_migrations SET version = ?", store.SchemaVersion-1)
	require.NoError(t, err)
	require.NoError(t, older.Close())
	selected, err := store.CreateIndexBackup(t.Context(), olderPath, filepath.Join(home, "selected.db"), store.IndexBackupReasonManual, store.IndexBackupRetentionManual)
	require.NoError(t, err)
	cmd := NewIndexCmd()
	cmd.SetArgs([]string{"restore", selected.Path, "--json"})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)

	err = cmd.Execute()

	require.NoError(t, err)
	var report indexReplacementReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Equal(t, "restore", report.Operation)
	assert.Equal(t, store.SchemaVersion-1, report.Replacement.ActiveSchema)
	assert.Equal(t, store.IndexCompatibilityOlder, report.Replacement.Compatibility)
	assert.False(t, report.Replacement.RepositoryFilesWritten)
	assert.FileExists(t, selected.Path)
	assert.NoFileExists(t, selected.Path+"-wal")
	assert.NoFileExists(t, selected.Path+"-shm")
}

func TestIndexRebuildCommand_WithUnreadableIndex_PreservesExactLocalEvidence(t *testing.T) {
	repoRoot := setupE2ERepo(t)
	home := os.Getenv("DEVSPECS_HOME")
	activePath := filepath.Join(home, "devspecs.db")
	originalBytes := []byte("unreadable command index")
	require.NoError(t, os.MkdirAll(home, 0o755))
	require.NoError(t, os.WriteFile(activePath, originalBytes, 0o600))
	cmd := NewIndexCmd()
	cmd.SetArgs([]string{"rebuild", "--path", repoRoot, "--json"})
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)

	err := cmd.Execute()

	require.NoError(t, err)
	var report indexReplacementReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Equal(t, store.SchemaVersion, report.Replacement.ActiveSchema)
	assert.Nil(t, report.Replacement.DisplacedIndexBackup)
	assert.NotEmpty(t, report.Replacement.PreservedIndexPath)
	preservedBytes, err := os.ReadFile(report.Replacement.PreservedIndexPath)
	require.NoError(t, err)
	assert.Equal(t, string(originalBytes), string(preservedBytes))
}
