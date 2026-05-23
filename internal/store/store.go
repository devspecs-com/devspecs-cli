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
const SchemaVersion = 8

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
		case 6:
			if err := db.migrate6To7(now); err != nil {
				return err
			}
			maxVersion = 7
		case 7:
			if err := db.migrate7To8(now); err != nil {
				return err
			}
			maxVersion = 8
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

func (db *DB) migrate6To7(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE artifacts ADD COLUMN subtype TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrate v6→v7 subtype: %w", err)
	}
	_, err := db.Exec(`UPDATE artifacts SET kind = 'decision', subtype = 'adr' WHERE kind = 'adr'`)
	if err != nil {
		return fmt.Errorf("migrate v6→v7 remap adr: %w", err)
	}
	_, err = db.Exec(`UPDATE artifacts SET kind = 'spec', subtype = 'openspec_change' WHERE kind = 'openspec_change'`)
	if err != nil {
		return fmt.Errorf("migrate v6→v7 remap openspec: %w", err)
	}
	_, err = db.Exec(`UPDATE artifacts SET kind = 'requirements', subtype = 'prd' WHERE kind = 'prd'`)
	if err != nil {
		return fmt.Errorf("migrate v6→v7 remap prd: %w", err)
	}
	_, err = db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 7, now)
	return err
}

func (db *DB) migrate7To8(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE artifact_todos ADD COLUMN section_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrate v7â†’v8 todo section_id: %w", err)
	}
	if err := tryAlterTable(db.DB, `ALTER TABLE artifact_criteria ADD COLUMN section_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrate v7â†’v8 criteria section_id: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS artifact_sections (
			id             TEXT PRIMARY KEY,
			artifact_id    TEXT NOT NULL,
			revision_id    TEXT NOT NULL,
			source_path    TEXT NOT NULL,
			heading_path   TEXT NOT NULL,
			heading_depth  INTEGER NOT NULL,
			start_line     INTEGER NOT NULL,
			end_line       INTEGER NOT NULL,
			title          TEXT NOT NULL,
			body           TEXT NOT NULL,
			token_estimate INTEGER NOT NULL,
			section_kind   TEXT NOT NULL DEFAULT '',
			metadata_json  TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL,
			FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
			FOREIGN KEY (revision_id) REFERENCES artifact_revisions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_todos_section ON artifact_todos(section_id)`,
		`CREATE INDEX IF NOT EXISTS idx_criteria_section ON artifact_criteria(section_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_artifact ON artifact_sections(artifact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_revision ON artifact_sections(revision_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sections_source_path ON artifact_sections(source_path)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS artifact_sections_fts USING fts5(
			section_id UNINDEXED,
			artifact_id UNINDEXED,
			heading_path,
			title,
			body,
			tokenize='unicode61'
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v7â†’v8 section schema: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 8, now)
	return err
}
