package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func TestOpen_CreatesCurrentSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")

	db, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	assertTableExists(t, db, "schema_migrations")
	assertTableExists(t, db, "repos")
	assertTableExists(t, db, "repo_roots")
	assertTableExists(t, db, "artifacts")
	assertTableExists(t, db, "artifact_revisions")
	assertTableExists(t, db, "sources")
	assertTableExists(t, db, "links")
	assertTableExists(t, db, "artifact_todos")
	assertTableExists(t, db, "artifact_criteria")
	assertTableExists(t, db, "artifact_tags")
	assertTableExists(t, db, "artifact_sections")
	assertTableExists(t, db, "artifact_sections_fts")
	assertTableExists(t, db, "concepts")
	assertTableExists(t, db, "concept_mentions")
	assertTableExists(t, db, "artifact_edges")
	assertTableExists(t, db, "git_commits")
	assertTableExists(t, db, "git_commit_files")
	assertTableExists(t, db, "source_manifest")
	assertTableExists(t, db, "source_manifest_symbols")
	assertTableExists(t, db, "source_manifest_tests")
	assertTableExists(t, db, "source_manifest_imports")
	assertTableExists(t, db, "source_manifest_fts")
	assertTableExists(t, db, "task_checkpoint_facts")
	assertTableExists(t, db, "thread_projections")
	assertTableExists(t, db, "thread_projection_links")
	assertTableExists(t, db, "thread_projection_tasks")
	assertTableExists(t, db, "thread_projection_events")
	assertTableExists(t, db, "thread_projection_sources")
	assert.True(t, indexExists(t, db, "idx_repos_git_identity"))
	assert.True(t, indexExists(t, db, "idx_sources_artifact_repo"))
	assert.True(t, indexExists(t, db, "idx_sources_repo"))
}

func TestMigrate_FromV15CreatesThreadProjectionTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	mustExecStoreTestSQL(t, db, `DROP TABLE thread_projection_sources`)
	mustExecStoreTestSQL(t, db, `DROP TABLE thread_projection_events`)
	mustExecStoreTestSQL(t, db, `DROP TABLE thread_projection_tasks`)
	mustExecStoreTestSQL(t, db, `DROP TABLE thread_projection_links`)
	mustExecStoreTestSQL(t, db, `DROP TABLE thread_projections`)
	mustExecStoreTestSQL(t, db, `UPDATE schema_migrations SET version = 15`)
	require.NoError(t, db.Close())

	db, err = Open(dbPath)

	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	assertTableExists(t, db, "thread_projections")
	assertTableExists(t, db, "thread_projection_links")
	assertTableExists(t, db, "thread_projection_tasks")
	assertTableExists(t, db, "thread_projection_events")
	assertTableExists(t, db, "thread_projection_sources")
	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version))
	assert.Equal(t, SchemaVersion, version)
}

func TestMigrate_FromV16ExposesSourceSemanticRangeColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	mustExecStoreTestSQL(t, db, `UPDATE schema_migrations SET version = 16`)
	require.NoError(t, db.Close())

	db, err = Open(dbPath)

	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	rows, err := db.Query(`SELECT parent, end_line FROM source_manifest_symbols LIMIT 0`)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
}

func TestMigrate_V14ToV15BackfillsRepositoryRoots(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	now := "2026-08-06T00:00:00Z"
	mustExecStoreTestSQL(t, db, `INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('legacy', '/tmp/legacy', ?, ?)`, now, now)
	mustExecStoreTestSQL(t, db, `DROP TRIGGER repos_record_initial_root`)
	mustExecStoreTestSQL(t, db, `DROP TABLE repo_roots`)
	mustExecStoreTestSQL(t, db, `UPDATE schema_migrations SET version = 14`)
	require.NoError(t, db.Close())

	db, err = Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	var repoID string
	require.NoError(t, db.QueryRow(`SELECT repo_id FROM repo_roots WHERE root_path = '/tmp/legacy'`).Scan(&repoID))
	assert.Equal(t, "legacy", repoID)
}

func TestMigrate_WhenAlreadyCurrent_RemainsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)

	err = db.migrate()
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	assert.Equal(t, 1, count)
	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version))
	assert.Equal(t, SchemaVersion, version)
	require.NoError(t, db.Close())
}

func TestMigrate_ExistingV15_RecreatesSourceOwnershipIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	mustExecStoreTestSQL(t, db, "DROP INDEX idx_sources_artifact_repo")
	mustExecStoreTestSQL(t, db, "DROP INDEX idx_sources_repo")
	require.NoError(t, db.Close())

	db, err = Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	assert.True(t, indexExists(t, db, "idx_sources_artifact_repo"))
	assert.True(t, indexExists(t, db, "idx_sources_repo"))
}

func TestMigrate_V12ToV13DropsSourceManifestCompactionIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_path ON source_manifest(repo_id, path)`)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_root ON source_manifest(repo_id, source_root)`)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_role ON source_manifest(repo_id, source_role)`)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_symbols_file ON source_manifest_symbols(file_id)`)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_tests_file ON source_manifest_tests(file_id)`)
	mustExecStoreTestSQL(t, db, `CREATE INDEX IF NOT EXISTS idx_source_manifest_imports_file ON source_manifest_imports(file_id)`)
	mustExecStoreTestSQL(t, db, `UPDATE schema_migrations SET version = 12`)
	require.NoError(t, db.Close())

	db, err = Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	assert.False(t, indexExists(t, db, "idx_source_manifest_repo_path"))
	assert.False(t, indexExists(t, db, "idx_source_manifest_repo_root"))
	assert.False(t, indexExists(t, db, "idx_source_manifest_repo_role"))
	assert.False(t, indexExists(t, db, "idx_source_manifest_symbols_file"))
	assert.False(t, indexExists(t, db, "idx_source_manifest_tests_file"))
	assert.False(t, indexExists(t, db, "idx_source_manifest_imports_file"))
	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version))
	assert.Equal(t, SchemaVersion, version)
}

func TestMigrate_FromV3ToV4(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(OFF)")
	require.NoError(t, err)
	mustExecRawStoreTestSQL(t, raw, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	mustExecRawStoreTestSQL(t, raw, `INSERT INTO schema_migrations (version, applied_at) VALUES (3, '2020-01-01T00:00:00Z')`)
	mustExecRawStoreTestSQL(t, raw, `CREATE TABLE sources (
		id TEXT PRIMARY KEY,
		artifact_id TEXT NOT NULL,
		repo_id TEXT,
		source_type TEXT NOT NULL,
		path TEXT,
		url TEXT,
		source_identity TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`)
	require.NoError(t, raw.Close())

	db, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	var version int
	require.NoError(t, db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version))
	assert.Equal(t, SchemaVersion, version)
	var columns int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sources') WHERE name IN ('format_profile', 'layout_group')").Scan(&columns))
	assert.Equal(t, 2, columns)
}

func TestOpen_WithUnsupportedOldSchemaVersion_ReturnsSafeRecoveryGuidance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	mustExecStoreTestSQL(t, db, "UPDATE schema_migrations SET version = 2")
	require.NoError(t, db.Close())

	_, err = Open(dbPath)

	require.Error(t, err)
	assert.ErrorContains(t, err, "schema v2")
	assert.ErrorContains(t, err, "ds index backup")
	assert.ErrorContains(t, err, "ds index rebuild")
	assert.NotContains(t, err.Error(), "delete")
}

func TestOpen_WithNewerSchemaVersion_ReturnsTypedCompatibilityError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	raw, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	_, err = Open(dbPath)

	require.Error(t, err)
	var newer *NewerSchemaError
	require.True(t, errors.As(err, &newer))
	assert.Equal(t, SchemaVersion+1, newer.DatabaseVersion)
	assert.Equal(t, SchemaVersion, newer.SupportedVersion)
}

func TestOpen_WithNewerSchemaVersion_DoesNotApplySchemaDDL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	db, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	raw, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = raw.Exec("DROP INDEX idx_sources_repo")
	require.NoError(t, err)
	_, err = raw.Exec("UPDATE schema_migrations SET version = ?", SchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	_, err = Open(dbPath)

	require.Error(t, err)
	raw, err = sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	var indexCount int
	require.NoError(t, raw.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_sources_repo'").Scan(&indexCount))
	assert.Zero(t, indexCount)
}

func TestOpen_WithNestedDatabasePath_CreatesParentDirectory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subdir", "nested", "devspecs.db")

	db, err := Open(dbPath)
	require.NoError(t, err)

	require.NoError(t, db.Close())
}

func indexExists(t *testing.T, db *DB, name string) bool {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&count))
	return count > 0
}

func assertTableExists(t *testing.T, db *DB, table string) {
	t.Helper()

	var name string
	require.NoError(t, db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name))
	assert.Equal(t, table, name)
}

func mustExecStoreTestSQL(t *testing.T, db *DB, query string, args ...any) {
	t.Helper()

	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}

func mustExecRawStoreTestSQL(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()

	_, err := db.Exec(query, args...)
	require.NoError(t, err)
}
