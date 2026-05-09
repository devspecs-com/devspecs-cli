// Package store manages the DevSpecs SQLite database: opening, migrations, and queries.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaDDL string

// DB wraps *sql.DB with DevSpecs-specific operations.
type DB struct {
	*sql.DB
}

// Open opens or creates the SQLite database at the given path.
// It ensures the parent directory exists and applies migrations.
func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db := &DB{DB: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	if _, err := db.Exec(schemaDDL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}

	// Record migration version 1 if not present
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (1, ?)",
			time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return nil
}
