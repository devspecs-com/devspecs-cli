package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func TestCreateIndexBackup_WithWALContent_CreatesSelfContainedSnapshot(t *testing.T) {
	home := t.TempDir()
	activePath := filepath.Join(home, "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('wal-repo', '/tmp/wal', '2026-08-12T00:00:00Z', '2026-08-12T00:00:00Z')`)
	require.NoError(t, err)

	backup, err := CreateIndexBackup(t.Context(), activePath, "", IndexBackupReasonManual, IndexBackupRetentionManual)

	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	assert.Equal(t, SchemaVersion, backup.SourceSchema)
	assert.Equal(t, IndexBackupRetentionManual, backup.Retention)
	assert.FileExists(t, backup.Path)
	assert.FileExists(t, backup.ManifestPath)
	backupDatabase, err := sql.Open("sqlite", readOnlyIndexDSN(backup.Path))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backupDatabase.Close()) })
	var repoCount int
	require.NoError(t, backupDatabase.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = 'wal-repo'`).Scan(&repoCount))
	assert.Equal(t, 1, repoCount)
}

func TestCreateIndexBackup_WithNewerSchema_PreservesNewerSchemaWithoutMigration(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	raw, err := sql.Open("sqlite", activePath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	backup, err := CreateIndexBackup(t.Context(), activePath, "", IndexBackupReasonManual, IndexBackupRetentionManual)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion+1, backup.SourceSchema)
	validation, err := ValidateIndex(t.Context(), backup.Path)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion+1, validation.Schema)
	assert.Equal(t, IndexCompatibilityNewer, validation.Compatible)
}

func TestCreateIndexBackup_WithInsufficientSpace_LeavesDestinationAbsent(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	outputPath := filepath.Join(filepath.Dir(activePath), "manual.db")
	originalDiskProbe := availableIndexDiskBytes
	availableIndexDiskBytes = func(string) (uint64, error) { return 1, nil }
	t.Cleanup(func() { availableIndexDiskBytes = originalDiskProbe })

	_, err = CreateIndexBackup(t.Context(), activePath, outputPath, IndexBackupReasonManual, IndexBackupRetentionManual)

	require.Error(t, err)
	assert.ErrorContains(t, err, "insufficient free space")
	assert.NoFileExists(t, outputPath)
	assert.NoFileExists(t, outputPath+".json")
}

func TestRequireIndexRebuildSpace_WithInsufficientSpace_RefusesBeforeStageCreation(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	originalDiskProbe := availableIndexDiskBytes
	availableIndexDiskBytes = func(string) (uint64, error) { return 1, nil }
	t.Cleanup(func() { availableIndexDiskBytes = originalDiskProbe })

	err := RequireIndexRebuildSpace(t.Context(), activePath)

	require.Error(t, err)
	assert.ErrorContains(t, err, "insufficient free space")
	assert.NoFileExists(t, activePath)
}

func TestOpen_WithSupportedOlderSchema_PreservesVerifiedRollbackBackup(t *testing.T) {
	home := t.TempDir()
	activePath := filepath.Join(home, "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	_, err = database.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion-1)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	migrated, err := Open(activePath)

	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, migrated.Close()) })
	var activeSchema int
	require.NoError(t, migrated.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&activeSchema))
	assert.Equal(t, SchemaVersion, activeSchema)
	backupPaths, err := filepath.Glob(filepath.Join(home, "backups", "index", "*.db"))
	require.NoError(t, err)
	require.Len(t, backupPaths, 1)
	backup, err := ValidateIndex(t.Context(), backupPaths[0])
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion-1, backup.Schema)
	assert.Equal(t, "ok", backup.Integrity)
}

func TestOpenContext_WithSupportedOlderSchemaAndCanceledWriterWait_LeavesOriginalUnchanged(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	_, err = database.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion-1)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	before := indexFileDigest(t, activePath)
	lease, err := AcquireIndexWriter(t.Context(), activePath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lease.Release()) })
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, err = OpenContext(ctx, activePath)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	after := indexFileDigest(t, activePath)
	assert.Equal(t, before, after)
}

func TestOpen_WithUnsupportedOlderSchema_LeavesOriginalDatabaseUnchanged(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	_, err = database.Exec("UPDATE schema_migrations SET version = 2")
	require.NoError(t, err)
	require.NoError(t, database.Close())
	before := indexFileDigest(t, activePath)

	_, err = Open(activePath)

	require.Error(t, err)
	assert.ErrorContains(t, err, "schema v2")
	after := indexFileDigest(t, activePath)
	assert.Equal(t, before, after)
}

func TestRestoreIndex_WithOlderSnapshot_RestoresExactlyAndBacksUpDisplacedIndex(t *testing.T) {
	home := t.TempDir()
	activePath := filepath.Join(home, "devspecs.db")
	olderSource := filepath.Join(home, "older-source.db")
	olderDatabase, err := Open(olderSource)
	require.NoError(t, err)
	_, err = olderDatabase.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion-1)
	require.NoError(t, err)
	require.NoError(t, olderDatabase.Close())
	selected, err := CreateIndexBackup(t.Context(), olderSource, filepath.Join(home, "selected.db"), IndexBackupReasonManual, IndexBackupRetentionManual)
	require.NoError(t, err)
	activeDatabase, err := Open(activePath)
	require.NoError(t, err)
	require.NoError(t, activeDatabase.Close())

	replacement, err := RestoreIndex(t.Context(), activePath, selected.Path)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion-1, replacement.ActiveSchema)
	assert.Equal(t, IndexCompatibilityOlder, replacement.Compatibility)
	require.NotNil(t, replacement.DisplacedIndexBackup)
	assert.Equal(t, SchemaVersion, replacement.DisplacedIndexBackup.SourceSchema)
	assert.FileExists(t, selected.Path)
	active, err := ValidateIndex(t.Context(), activePath)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion-1, active.Schema)
}

func TestReplaceIndexWithStage_WhenPublicationFails_RestoresOriginalActiveIndex(t *testing.T) {
	home := t.TempDir()
	activePath := filepath.Join(home, "devspecs.db")
	activeDatabase, err := Open(activePath)
	require.NoError(t, err)
	_, err = activeDatabase.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('original', '/tmp/original', '2026-08-12T00:00:00Z', '2026-08-12T00:00:00Z')`)
	require.NoError(t, err)
	require.NoError(t, activeDatabase.Close())
	stagedPath, err := NewIndexStagePath(activePath)
	require.NoError(t, err)
	stagedDatabase, err := OpenWithWriterLease(stagedPath)
	require.NoError(t, err)
	require.NoError(t, stagedDatabase.Close())
	originalPublisher := publishStagedIndex
	publishStagedIndex = func(string, string) error { return errors.New("injected publication failure") }
	t.Cleanup(func() { publishStagedIndex = originalPublisher })

	_, err = ReplaceIndexWithStage(context.Background(), activePath, stagedPath, IndexBackupReasonRebuild, SchemaVersion)

	require.Error(t, err)
	assert.ErrorContains(t, err, "injected publication failure")
	assert.NoFileExists(t, IndexRecoveryJournalPath(activePath))
	restored, err := Open(activePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restored.Close()) })
	var originalCount int
	require.NoError(t, restored.QueryRow(`SELECT COUNT(*) FROM repos WHERE id = 'original'`).Scan(&originalCount))
	assert.Equal(t, 1, originalCount)
}

func TestReplaceIndexWithStage_WhenActiveIndexIsUnreadable_PreservesExactEvidenceAndPublishesStage(t *testing.T) {
	home := t.TempDir()
	activePath := filepath.Join(home, "devspecs.db")
	originalBytes := []byte("unreadable index evidence")
	require.NoError(t, os.WriteFile(activePath, originalBytes, 0o600))
	stagedPath, err := NewIndexStagePath(activePath)
	require.NoError(t, err)
	stagedDatabase, err := OpenWithWriterLease(stagedPath)
	require.NoError(t, err)
	require.NoError(t, stagedDatabase.Close())

	replacement, err := ReplaceIndexWithStage(t.Context(), activePath, stagedPath, IndexBackupReasonRebuild, SchemaVersion)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, replacement.ActiveSchema)
	assert.Nil(t, replacement.DisplacedIndexBackup)
	assert.NotEmpty(t, replacement.PreservedIndexPath)
	assert.FileExists(t, replacement.PreservedIndexPath)
	preservedBytes, err := os.ReadFile(replacement.PreservedIndexPath)
	require.NoError(t, err)
	assert.Equal(t, string(originalBytes), string(preservedBytes))
	active, err := ValidateIndex(t.Context(), activePath)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, active.Schema)
	assert.NoFileExists(t, IndexRecoveryJournalPath(activePath))
}

func TestOpen_WithInterruptedRecoveryJournal_BlocksNormalIndexUse(t *testing.T) {
	activePath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(activePath)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	journal := IndexRecoveryJournal{
		Version:     indexRecoveryJournalVersion,
		OperationID: "interrupted",
		Operation:   IndexBackupReasonRestore,
		ActivePath:  activePath,
		Phase:       "active_preserved",
	}
	require.NoError(t, writeIndexJSON(IndexRecoveryJournalPath(activePath), journal))

	_, err = Open(activePath)

	require.Error(t, err)
	var pending *RecoveryPendingError
	require.True(t, errors.As(err, &pending))
	assert.Equal(t, "active_preserved", pending.Journal.Phase)
	assert.Equal(t, IndexRecoveryJournalPath(activePath), pending.JournalPath)
}

func indexFileDigest(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	digest := sha256.Sum256(body)
	return fmt.Sprintf("%x", digest)
}
