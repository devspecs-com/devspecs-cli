package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_WhenCurrentSchemaWriterIsActive_ReadsLastCommittedSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	writer, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	mustExecStoreTestSQL(t, writer, `INSERT INTO repos (id, root_path, created_at, updated_at)
		VALUES ('repo-1', '/repo', '2026-08-11T00:00:00Z', '2026-08-11T00:00:00Z')`)
	mustExecStoreTestSQL(t, writer, "BEGIN IMMEDIATE")
	t.Cleanup(func() { _, _ = writer.Exec("ROLLBACK") })
	mustExecStoreTestSQL(t, writer, "UPDATE repos SET root_path = '/uncommitted' WHERE id = 'repo-1'")

	reader, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	var rootPath string
	err = reader.QueryRow("SELECT root_path FROM repos WHERE id = 'repo-1'").Scan(&rootPath)

	require.NoError(t, err)
	assert.Equal(t, "/repo", rootPath)
}

func TestConcurrentWriter_WhenExistingWriteExceedsLegacyTimeout_WaitsAndBegins(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	writer, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	mustExecStoreTestSQL(t, writer, "BEGIN IMMEDIATE")
	t.Cleanup(func() { _, _ = writer.Exec("ROLLBACK") })
	contender, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = contender.Close() })
	releaseResult := make(chan error, 1)

	go func() {
		time.Sleep(6 * time.Second)
		_, releaseErr := writer.Exec("ROLLBACK")
		releaseResult <- releaseErr
	}()
	_, beginErr := contender.Exec("BEGIN IMMEDIATE")
	releaseErr := <-releaseResult
	var rollbackErr error
	if beginErr == nil {
		_, rollbackErr = contender.Exec("ROLLBACK")
	}

	require.NoError(t, releaseErr)
	require.NoError(t, beginErr)
	assert.NoError(t, rollbackErr)
}
