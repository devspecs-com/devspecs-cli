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

	// Verify tables exist
	tables := []string{"schema_migrations", "repos", "artifacts", "artifact_revisions", "sources", "links", "artifact_todos"}
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

	// Apply migration again
	if err := db.migrate(); err != nil {
		t.Fatal("second migration failed:", err)
	}

	// Should still have exactly two migration rows (v1 + v2)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 migration rows, got %d", count)
	}

	db.Close()
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
