package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectIndex_WhenDatabaseIsMissing_DoesNotCreateHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing-home")
	dbPath := filepath.Join(home, "devspecs.db")

	inspection, err := InspectIndex(context.Background(), dbPath)

	require.NoError(t, err)
	assert.False(t, inspection.Exists)
	assert.Equal(t, IndexCompatibilityAbsent, inspection.Compatibility)
	_, statErr := os.Stat(home)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestInspectIndex_WithCurrentSchema_ReturnsCurrentWithoutMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	inspection, err := InspectIndex(context.Background(), dbPath)

	require.NoError(t, err)
	assert.True(t, inspection.Exists)
	assert.Equal(t, SchemaVersion, inspection.DatabaseSchema)
	assert.Equal(t, SchemaVersion, inspection.SupportedSchema)
	assert.Equal(t, IndexCompatibilityCurrent, inspection.Compatibility)
	assert.Positive(t, inspection.DatabaseBytes)
}

func TestInspectIndex_WithOlderSchema_ReturnsOlderWithoutMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = database.Exec("CREATE TABLE schema_migrations (version INTEGER NOT NULL)")
	require.NoError(t, err)
	_, err = database.Exec("INSERT INTO schema_migrations (version) VALUES (?)", SchemaVersion-1)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	inspection, err := InspectIndex(context.Background(), dbPath)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion-1, inspection.DatabaseSchema)
	assert.Equal(t, IndexCompatibilityOlder, inspection.Compatibility)
	assert.Equal(t, SchemaVersion-1, readInspectionTestSchema(t, dbPath))
}

func TestInspectIndex_WithNewerSchema_ReturnsNewerWithoutMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	database, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = database.Exec("CREATE TABLE schema_migrations (version INTEGER NOT NULL)")
	require.NoError(t, err)
	_, err = database.Exec("INSERT INTO schema_migrations (version) VALUES (?)", SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	inspection, err := InspectIndex(context.Background(), dbPath)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion+1, inspection.DatabaseSchema)
	assert.Equal(t, IndexCompatibilityNewer, inspection.Compatibility)
	assert.Equal(t, SchemaVersion+1, readInspectionTestSchema(t, dbPath))
}

func TestInspectIndex_WithMalformedDatabase_ReturnsReadError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("not sqlite"), 0o600))

	inspection, err := InspectIndex(context.Background(), dbPath)

	require.Error(t, err)
	assert.True(t, inspection.Exists)
	assert.Positive(t, inspection.DatabaseBytes)
	assert.ErrorContains(t, err, "inspect schema table")
}

func TestInspectIndexWriter_WhenLockFileIsMissing_DoesNotCreateIt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	lockPath := dbPath + ".writer.lock"

	state, err := InspectIndexWriter(dbPath)

	require.NoError(t, err)
	assert.Equal(t, IndexWriterStateNotPresent, state)
	_, statErr := os.Stat(lockPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestInspectIndexWriter_WhenExistingLockIsAvailable_ReturnsAvailable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	lockPath := dbPath + ".writer.lock"
	require.NoError(t, os.WriteFile(lockPath, nil, 0o600))

	state, err := InspectIndexWriter(dbPath)

	require.NoError(t, err)
	assert.Equal(t, IndexWriterStateAvailable, state)
}

func TestInspectIndexWriter_WhenLeaseIsHeld_ReturnsHeld(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	lease, err := AcquireIndexWriter(context.Background(), dbPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release() })

	state, err := InspectIndexWriter(dbPath)

	require.NoError(t, err)
	assert.Equal(t, IndexWriterStateHeld, state)
}

func readInspectionTestSchema(t *testing.T, dbPath string) int {
	t.Helper()
	database, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	var version int
	require.NoError(t, database.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version))
	return version
}
