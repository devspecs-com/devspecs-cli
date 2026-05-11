// Package store manages the DevSpecs SQLite database: opening, migrations, and queries.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaDDL string

// SchemaVersion is the current schema version. Bump when schema.sql changes.
const SchemaVersion = 6

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

	now := time.Now().UTC().Format(time.RFC3339)

	var maxVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&maxVersion)
	if err != nil {
		return err
	}

	if maxVersion == 0 {
		// Fresh DB — record current version
		_, err = db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", SchemaVersion, now)
		return err
	}

	for maxVersion < SchemaVersion {
		switch maxVersion {
		case 3:
			if err := db.migrate3To4(now); err != nil {
				return err
			}
			maxVersion = 4
		case 4:
			if err := db.migrate4To5(now); err != nil {
				return err
			}
			maxVersion = 5
		case 5:
			if err := db.migrate5To6(now); err != nil {
				return err
			}
			maxVersion = 6
		default:
			return fmt.Errorf(
				"index was created with schema v%d but this CLI requires v%d. Run 'ds scan --rebuild' or delete ~/.devspecs/devspecs.db and run 'ds scan' to rebuild",
				maxVersion, SchemaVersion,
			)
		}
	}

	if maxVersion > SchemaVersion {
		return fmt.Errorf("database schema v%d is newer than this CLI (v%d)", maxVersion, SchemaVersion)
	}

	return nil
}

func tryAlterTable(db *sql.DB, stmt string) error {
	_, err := db.Exec(stmt)
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate column") {
		return nil
	}
	return err
}

func (db *DB) migrate3To4(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE sources ADD COLUMN format_profile TEXT NOT NULL DEFAULT 'generic'`); err != nil {
		return fmt.Errorf("migrate v3→v4 format_profile: %w", err)
	}
	if err := tryAlterTable(db.DB, `ALTER TABLE sources ADD COLUMN layout_group TEXT`); err != nil {
		return fmt.Errorf("migrate v3→v4 layout_group: %w", err)
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 4, now)
	return err
}

func (db *DB) migrate4To5(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE artifacts ADD COLUMN authored_at TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrate v4→v5 authored_at: %w", err)
	}
	if _, err := db.Exec(`UPDATE artifacts SET authored_at = created_at WHERE authored_at = ''`); err != nil {
		return fmt.Errorf("migrate v4→v5 backfill authored_at: %w", err)
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 5, now)
	return err
}

func (db *DB) migrate5To6(now string) error {
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 6, now)
	return err
}
