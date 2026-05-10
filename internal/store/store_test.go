package store

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDB(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tables := []string{"schema_migrations", "repos", "artifacts", "artifact_revisions", "sources", "links", "artifact_todos", "artifact_tags"}
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
		t.Errorf("expected 1 migration row (version 3), got %d", count)
	}

	var version int
	db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if version != SchemaVersion {
		t.Errorf("expected version %d, got %d", SchemaVersion, version)
	}

	db.Close()
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
	if !contains(err.Error(), "schema v2") {
		t.Errorf("expected schema version mismatch error, got: %s", err)
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
