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
const SchemaVersion = 14

// SQLiteBusyTimeoutMS is the local index write-wait window for concurrent CLI
// commands before SQLite returns a busy/locked error.
const SQLiteBusyTimeoutMS = 5000

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

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(%d)", dbPath, SQLiteBusyTimeoutMS)
	sqlDB, err := sql.Open("sqlite", dsn)
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

// IsSQLiteBusyError reports whether err looks like SQLite write contention.
func IsSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database is busy") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "busy timeout")
}

// FriendlySQLiteBusyError adds an operator-facing hint to SQLite lock errors.
func FriendlySQLiteBusyError(err error) error {
	if !IsSQLiteBusyError(err) {
		return err
	}
	return fmt.Errorf("local DevSpecs index is busy; another ds command is writing. Wait for scan, capture, or task sync to finish, then retry: %w", err)
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
		case 8:
			if err := db.migrate8To9(now); err != nil {
				return err
			}
			maxVersion = 9
		case 9:
			if err := db.migrate9To10(now); err != nil {
				return err
			}
			maxVersion = 10
		case 10:
			if err := db.migrate10To11(now); err != nil {
				return err
			}
			maxVersion = 11
		case 11:
			if err := db.migrate11To12(now); err != nil {
				return err
			}
			maxVersion = 12
		case 12:
			if err := db.migrate12To13(now); err != nil {
				return err
			}
			maxVersion = 13
		case 13:
			if err := db.migrate13To14(now); err != nil {
				return err
			}
			maxVersion = 14
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

func (db *DB) migrate8To9(now string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS concepts (
			id                         TEXT PRIMARY KEY,
			repo_id                    TEXT NOT NULL,
			canonical                  TEXT NOT NULL,
			kind                       TEXT NOT NULL,
			forms_json                 TEXT NOT NULL DEFAULT '[]',
			document_frequency         INTEGER NOT NULL DEFAULT 0,
			inverse_document_frequency REAL NOT NULL DEFAULT 0,
			created_at                 TEXT NOT NULL,
			updated_at                 TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS concept_mentions (
			id            TEXT PRIMARY KEY,
			concept_id    TEXT NOT NULL,
			artifact_id   TEXT NOT NULL,
			section_id    TEXT NOT NULL DEFAULT '',
			field         TEXT NOT NULL,
			weight        REAL NOT NULL,
			evidence_json TEXT NOT NULL DEFAULT '{}',
			created_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_edges (
			id              TEXT PRIMARY KEY,
			repo_id         TEXT NOT NULL,
			src_artifact_id TEXT NOT NULL,
			dst_artifact_id TEXT NOT NULL,
			edge_type       TEXT NOT NULL,
			weight          REAL NOT NULL,
			confidence      REAL NOT NULL,
			evidence_count  INTEGER NOT NULL DEFAULT 1,
			freshness       TEXT NOT NULL DEFAULT '',
			source_signal   TEXT NOT NULL,
			explanation     TEXT NOT NULL DEFAULT '',
			metadata_json   TEXT NOT NULL DEFAULT '{}',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_concepts_repo_kind_canonical ON concepts(repo_id, kind, canonical)`,
		`CREATE INDEX IF NOT EXISTS idx_concept_mentions_concept ON concept_mentions(concept_id)`,
		`CREATE INDEX IF NOT EXISTS idx_concept_mentions_artifact ON concept_mentions(artifact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_concept_mentions_artifact_section ON concept_mentions(artifact_id, section_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifact_edges_src ON artifact_edges(repo_id, src_artifact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifact_edges_dst ON artifact_edges(repo_id, dst_artifact_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifact_edges_type ON artifact_edges(repo_id, edge_type)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_edges_identity ON artifact_edges(repo_id, src_artifact_id, dst_artifact_id, edge_type, source_signal)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v8->v9 evidence graph schema: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 9, now)
	return err
}

func (db *DB) migrate9To10(now string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS git_commits (
			repo_id       TEXT NOT NULL,
			sha           TEXT NOT NULL,
			branch        TEXT NOT NULL DEFAULT '',
			author_name   TEXT NOT NULL DEFAULT '',
			author_email  TEXT NOT NULL DEFAULT '',
			message       TEXT NOT NULL,
			body_preview  TEXT NOT NULL DEFAULT '',
			committed_at  TEXT NOT NULL,
			files_changed INTEGER NOT NULL DEFAULT 0,
			is_merge      INTEGER NOT NULL DEFAULT 0,
			history_shape TEXT NOT NULL DEFAULT '',
			indexed_at    TEXT NOT NULL,
			PRIMARY KEY (repo_id, sha)
		)`,
		`CREATE TABLE IF NOT EXISTS git_commit_files (
			repo_id     TEXT NOT NULL,
			commit_sha  TEXT NOT NULL,
			file_path   TEXT NOT NULL,
			change_type TEXT NOT NULL DEFAULT '',
			old_path    TEXT NOT NULL DEFAULT '',
			indexed_at  TEXT NOT NULL,
			PRIMARY KEY (repo_id, commit_sha, file_path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_git_commits_repo_committed ON git_commits(repo_id, committed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_git_commit_files_repo_file ON git_commit_files(repo_id, file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_git_commit_files_commit ON git_commit_files(commit_sha)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v9->v10 git fact schema: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 10, now)
	return err
}

func (db *DB) migrate10To11(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE git_commits ADD COLUMN body_preview TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("migrate v10->v11 git commit body_preview: %w", err)
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 11, now)
	return err
}

func (db *DB) migrate11To12(now string) error {
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 12, now)
	return err
}

func (db *DB) migrate12To13(now string) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_source_manifest_repo_path`,
		`DROP INDEX IF EXISTS idx_source_manifest_repo_root`,
		`DROP INDEX IF EXISTS idx_source_manifest_repo_role`,
		`DROP INDEX IF EXISTS idx_source_manifest_symbols_file`,
		`DROP INDEX IF EXISTS idx_source_manifest_tests_file`,
		`DROP INDEX IF EXISTS idx_source_manifest_imports_file`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v12->v13 source manifest index compaction: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 13, now)
	return err
}

func (db *DB) migrate13To14(now string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS task_checkpoint_facts (
			repo_id              TEXT NOT NULL,
			task_id              TEXT NOT NULL,
			checkpoint_id        TEXT NOT NULL,
			target               TEXT NOT NULL,
			series               TEXT NOT NULL DEFAULT '',
			stage                TEXT NOT NULL DEFAULT '',
			decision             TEXT NOT NULL DEFAULT '',
			checkpoint_path      TEXT NOT NULL DEFAULT '',
			checkpoint_json_path TEXT NOT NULL DEFAULT '',
			created_at           TEXT NOT NULL,
			actual_context_json  TEXT NOT NULL DEFAULT '{}',
			feedback_json        TEXT NOT NULL DEFAULT '{}',
			evidence_json        TEXT NOT NULL DEFAULT '{}',
			learnings_json       TEXT NOT NULL DEFAULT '[]',
			next_json            TEXT NOT NULL DEFAULT '{}',
			indexed_at           TEXT NOT NULL,
			PRIMARY KEY (repo_id, task_id, checkpoint_id),
			FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_checkpoint_facts_task ON task_checkpoint_facts(repo_id, task_id, target, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_task_checkpoint_facts_stage ON task_checkpoint_facts(repo_id, stage, decision)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v13->v14 task checkpoint facts: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 14, now)
	return err
}
