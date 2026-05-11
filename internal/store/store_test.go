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

	tables := []string{"schema_migrations", "repos", "artifacts", "artifact_revisions", "sources", "links", "artifact_todos", "artifact_criteria", "artifact_tags"}
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
