package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestOpen_CreatesDB(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tables := []string{"schema_migrations", "repos", "artifacts", "artifact_revisions", "sources", "links", "artifact_todos", "artifact_criteria", "artifact_tags", "artifact_sections", "artifact_sections_fts", "concepts", "concept_mentions", "artifact_edges", "git_commits", "git_commit_files", "source_manifest", "source_manifest_symbols", "source_manifest_tests", "source_manifest_imports", "source_manifest_fts", "task_checkpoint_facts"}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Apply migration again — should not error
	if err := db.migrate(); err != nil {
		t.Fatal("second migration failed:", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 migration row (version %d), got %d", SchemaVersion, count)
	}

	var version int
	db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if version != SchemaVersion {
		t.Errorf("expected version %d, got %d", SchemaVersion, version)
	}

	db.Close()
}

func TestMigrate_V12ToV13DropsSourceManifestCompactionIndexes(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_path ON source_manifest(repo_id, path)`,
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_root ON source_manifest(repo_id, source_root)`,
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_repo_role ON source_manifest(repo_id, source_role)`,
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_symbols_file ON source_manifest_symbols(file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_tests_file ON source_manifest_tests(file_id)`,
		`CREATE INDEX IF NOT EXISTS idx_source_manifest_imports_file ON source_manifest_imports(file_id)`,
		`UPDATE schema_migrations SET version = 12`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	db, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{
		"idx_source_manifest_repo_path",
		"idx_source_manifest_repo_root",
		"idx_source_manifest_repo_role",
		"idx_source_manifest_symbols_file",
		"idx_source_manifest_tests_file",
		"idx_source_manifest_imports_file",
	} {
		if indexExists(t, db, name) {
			t.Fatalf("expected migration to drop %s", name)
		}
	}
	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", SchemaVersion, version)
	}
}

func TestMigrate_FromV3ToV4(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "legacy.db")
	raw, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(OFF)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (3, '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE sources (
		id TEXT PRIMARY KEY,
		artifact_id TEXT NOT NULL,
		repo_id TEXT,
		source_type TEXT NOT NULL,
		path TEXT,
		url TEXT,
		source_identity TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != SchemaVersion {
		t.Fatalf("expected schema version %d after migrate, got %d", SchemaVersion, v)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('sources') WHERE name IN ('format_profile', 'layout_group')").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected format_profile and layout_group columns on sources, got count %d", n)
	}
}

func TestOpen_OldSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Downgrade the recorded version to 2
	db.Exec("UPDATE schema_migrations SET version = 2")
	db.Close()

	_, err = Open(dbPath)
	if err == nil {
		t.Fatal("expected error for old schema version")
	}
	if !contains(err.Error(), "schema v2") || !contains(err.Error(), "scan --rebuild") {
		t.Errorf("expected schema version mismatch error mentioning rebuild, got: %s", err)
	}
}

func TestOpen_CreatesParentDir(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "subdir", "nested", "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}

func indexExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}
