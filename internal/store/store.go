// Package store manages the DevSpecs SQLite database: opening, migrations, and queries.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
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
const SchemaVersion = 17

// SQLiteBusyTimeoutMS is the fallback for short writes that contend outside the
// process-level index writer queue. Command contexts own longer operation limits.
const SQLiteBusyTimeoutMS = 60 * 1000

// DB wraps *sql.DB with DevSpecs-specific operations.
type DB struct {
	*sql.DB
	path string
}

// NewerSchemaError reports that an older CLI cannot safely open an index
// written by a newer DevSpecs schema.
type NewerSchemaError struct {
	DatabaseVersion  int
	SupportedVersion int
}

func (err *NewerSchemaError) Error() string {
	return fmt.Sprintf("database schema v%d is newer than this CLI (v%d)", err.DatabaseVersion, err.SupportedVersion)
}

// Open opens or creates the SQLite database at the given path.
// It ensures the parent directory exists and coordinates backup-first migrations.
func Open(dbPath string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return OpenContext(ctx, dbPath)
}

// OpenContext opens an index and bounds any coordinated forward migration by ctx.
func OpenContext(ctx context.Context, dbPath string) (*DB, error) {
	return openIndex(ctx, dbPath, false)
}

// OpenWithWriterLease opens an index while the caller holds its writer lease.
// This avoids reacquiring the same process-wide lease during forward migration.
func OpenWithWriterLease(dbPath string) (*DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return OpenWithWriterLeaseContext(ctx, dbPath)
}

// OpenWithWriterLeaseContext opens an index while the caller holds its writer
// lease and bounds any migration by ctx.
func OpenWithWriterLeaseContext(ctx context.Context, dbPath string) (*DB, error) {
	return openIndex(ctx, dbPath, true)
}

func openIndex(ctx context.Context, dbPath string, writerLeaseHeld bool) (*DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	if err := blockPendingIndexRecovery(dbPath); err != nil {
		return nil, err
	}

	db, err := openSQLiteIndex(dbPath)
	if err != nil {
		return nil, err
	}
	maxVersion, err := existingSchemaVersion(db.DB)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("inspect schema version: %w", err)
	}
	if maxVersion > SchemaVersion {
		db.Close()
		return nil, &NewerSchemaError{DatabaseVersion: maxVersion, SupportedVersion: SchemaVersion}
	}
	if maxVersion > 0 && maxVersion < SchemaVersion {
		if err := db.Close(); err != nil {
			return nil, fmt.Errorf("close index before migration: %w", err)
		}
		if writerLeaseHeld {
			if err := migrateIndexCopyOnWrite(ctx, dbPath); err != nil {
				return nil, err
			}
		} else {
			lease, err := AcquireIndexWriter(ctx, dbPath, nil)
			if err != nil {
				return nil, err
			}
			migrationErr := migrateIndexCopyOnWrite(ctx, dbPath)
			releaseErr := lease.Release()
			if err := errors.Join(migrationErr, releaseErr); err != nil {
				return nil, err
			}
		}
		return openIndex(ctx, dbPath, writerLeaseHeld)
	}
	if err := db.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func openSQLiteIndex(dbPath string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(%d)", dbPath, SQLiteBusyTimeoutMS)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &DB{DB: sqlDB, path: dbPath}, nil
}

func existingSchemaVersion(db *sql.DB) (int, error) {
	var tableCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'").Scan(&tableCount); err != nil {
		return 0, err
	}
	if tableCount == 0 {
		return 0, nil
	}
	var maxVersion int
	if err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&maxVersion); err != nil {
		return 0, err
	}
	return maxVersion, nil
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
		if _, err = db.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)", SchemaVersion, now); err != nil {
			return err
		}
		maxVersion = SchemaVersion
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
		case 14:
			if err := db.migrate14To15(now); err != nil {
				return err
			}
			maxVersion = 15
		case 15:
			if err := db.migrate15To16(now); err != nil {
				return err
			}
			maxVersion = 16
		case 16:
			if err := db.migrate16To17(now); err != nil {
				return err
			}
			maxVersion = 17
		default:
			return fmt.Errorf(
				"index was created with schema v%d but this CLI requires v%d. Run 'ds index backup', then 'ds index rebuild --path <repo>' to rebuild safely",
				maxVersion, SchemaVersion,
			)
		}
	}

	if maxVersion > SchemaVersion {
		return &NewerSchemaError{DatabaseVersion: maxVersion, SupportedVersion: SchemaVersion}
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_repos_git_identity
		ON repos(git_identity)
		WHERE git_identity IS NOT NULL AND git_identity <> ''`); err != nil {
		return fmt.Errorf("ensure repository identity index: %w", err)
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

func (db *DB) migrate14To15(now string) error {
	if err := tryAlterTable(db.DB, `ALTER TABLE repos ADD COLUMN git_root_commit TEXT`); err != nil {
		return fmt.Errorf("migrate v14->v15 git root commit: %w", err)
	}
	if err := tryAlterTable(db.DB, `ALTER TABLE repos ADD COLUMN git_identity TEXT`); err != nil {
		return fmt.Errorf("migrate v14->v15 git identity: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS repo_roots (
			root_path     TEXT PRIMARY KEY,
			repo_id       TEXT NOT NULL,
			first_seen_at TEXT NOT NULL,
			last_seen_at  TEXT NOT NULL,
			FOREIGN KEY (repo_id) REFERENCES repos(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_repo_roots_repo ON repo_roots(repo_id)`,
		`INSERT OR IGNORE INTO repo_roots (root_path, repo_id, first_seen_at, last_seen_at)
		 SELECT root_path, id, created_at, updated_at FROM repos`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v14->v15 repository roots: %w", err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 15, now)
	return err
}

func (db *DB) migrate15To16(now string) error {
	if err := createThreadProjectionSchema(db.DB); err != nil {
		return fmt.Errorf("migrate v15->v16 thread projections: %w", err)
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 16, now)
	return err
}

func (db *DB) migrate16To17(now string) error {
	statements := []struct {
		label string
		sql   string
	}{
		{label: "source symbol parent", sql: `ALTER TABLE source_manifest_symbols ADD COLUMN parent TEXT NOT NULL DEFAULT ''`},
		{label: "source symbol end line", sql: `ALTER TABLE source_manifest_symbols ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0`},
		{label: "source test end line", sql: `ALTER TABLE source_manifest_tests ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0`},
		{label: "source import end line", sql: `ALTER TABLE source_manifest_imports ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0`},
	}
	for _, statement := range statements {
		if err := tryAlterTable(db.DB, statement.sql); err != nil {
			return fmt.Errorf("migrate v16->v17 %s: %w", statement.label, err)
		}
	}
	_, err := db.Exec("UPDATE schema_migrations SET version = ?, applied_at = ?", 17, now)
	return err
}

func createThreadProjectionSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS thread_projections (
			owner_id            TEXT PRIMARY KEY,
			owner_kind          TEXT NOT NULL,
			task_id             TEXT NOT NULL DEFAULT '',
			workspace_id        TEXT NOT NULL DEFAULT '',
			change_id           TEXT NOT NULL DEFAULT '',
			repo_root           TEXT NOT NULL DEFAULT '',
			workspace_root      TEXT NOT NULL DEFAULT '',
			definition_path     TEXT NOT NULL,
			definition_revision INTEGER NOT NULL,
			definition_json     TEXT NOT NULL,
			projected_at        TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS thread_projection_links (
			owner_id       TEXT NOT NULL,
			position       INTEGER NOT NULL,
			repo_alias     TEXT NOT NULL DEFAULT '',
			task_id        TEXT NOT NULL,
			target         TEXT NOT NULL,
			name           TEXT NOT NULL DEFAULT '',
			status         TEXT NOT NULL DEFAULT '',
			repo_root      TEXT NOT NULL,
			task_workspace TEXT NOT NULL,
			PRIMARY KEY (owner_id, position),
			FOREIGN KEY (owner_id) REFERENCES thread_projections(owner_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS thread_projection_tasks (
			owner_id       TEXT NOT NULL,
			repo_alias     TEXT NOT NULL DEFAULT '',
			task_id        TEXT NOT NULL,
			repo_root      TEXT NOT NULL,
			task_workspace TEXT NOT NULL,
			manifest_path  TEXT NOT NULL,
			manifest_json  TEXT NOT NULL,
			PRIMARY KEY (owner_id, repo_alias, task_id),
			FOREIGN KEY (owner_id) REFERENCES thread_projections(owner_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS thread_projection_events (
			owner_id       TEXT NOT NULL,
			repo_alias     TEXT NOT NULL DEFAULT '',
			task_id        TEXT NOT NULL,
			checkpoint_id  TEXT NOT NULL,
			target         TEXT NOT NULL,
			json_path      TEXT NOT NULL,
			markdown_path  TEXT NOT NULL DEFAULT '',
			record_json    TEXT NOT NULL,
			PRIMARY KEY (owner_id, repo_alias, task_id, checkpoint_id),
			FOREIGN KEY (owner_id) REFERENCES thread_projections(owner_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS thread_projection_sources (
			owner_id      TEXT NOT NULL,
			source_kind   TEXT NOT NULL,
			source_key    TEXT NOT NULL,
			path          TEXT NOT NULL,
			size_bytes    INTEGER NOT NULL,
			modified_ns   INTEGER NOT NULL,
			content_hash  TEXT NOT NULL,
			PRIMARY KEY (owner_id, source_kind, source_key),
			FOREIGN KEY (owner_id) REFERENCES thread_projections(owner_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_projection_tasks_owner ON thread_projection_tasks(owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_projection_events_owner_target ON thread_projection_events(owner_id, task_id, target)`,
		`CREATE INDEX IF NOT EXISTS idx_thread_projection_sources_owner_path ON thread_projection_sources(owner_id, path)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}
