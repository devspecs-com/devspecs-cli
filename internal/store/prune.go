package store

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
)

// PruneOptions controls stale repository cleanup.
type PruneOptions struct {
	DryRun     bool
	Vacuum     bool
	PathExists func(string) bool
	Progress   func(PruneProgress)
}

// PruneProgress identifies a blocking maintenance phase without implying a
// percentage SQLite cannot report accurately.
type PruneProgress struct {
	Phase string
}

// PruneReport summarizes index lifecycle maintenance.
type PruneReport struct {
	DryRun             bool     `json:"dry_run"`
	Vacuumed           bool     `json:"vacuumed"`
	StaleRoots         []string `json:"stale_roots"`
	RepositoriesPruned int      `json:"repositories_pruned"`
	ArtifactsPruned    int      `json:"artifacts_pruned"`
	RevisionsCompacted int      `json:"revisions_compacted"`
	BytesBefore        int64    `json:"bytes_before"`
	BytesAfter         int64    `json:"bytes_after"`
}

// Prune removes missing root aliases and repositories with no remaining live root.
func (db *DB) Prune(opts PruneOptions) (PruneReport, error) {
	report := PruneReport{DryRun: opts.DryRun}
	if opts.DryRun && opts.Vacuum {
		return report, fmt.Errorf("--dry-run and --vacuum cannot be used together")
	}
	emitPruneProgress(opts.Progress, "inspect")
	exists := opts.PathExists
	if exists == nil {
		exists = func(path string) bool {
			_, err := os.Stat(path)
			return err == nil || !os.IsNotExist(err)
		}
	}
	dbPath := db.databasePath()
	report.BytesBefore = fileSize(dbPath)

	type rootRow struct {
		repoID string
		path   sql.NullString
	}
	rows, err := db.Query(`
		SELECT r.id, rr.root_path
		FROM repos r
		LEFT JOIN repo_roots rr ON rr.repo_id = r.id
		ORDER BY r.id, rr.root_path`)
	if err != nil {
		return report, err
	}
	var roots []rootRow
	for rows.Next() {
		var row rootRow
		if err := rows.Scan(&row.repoID, &row.path); err != nil {
			rows.Close()
			return report, err
		}
		roots = append(roots, row)
	}
	if err := rows.Close(); err != nil {
		return report, err
	}

	liveByRepo := map[string]int{}
	allRepos := map[string]struct{}{}
	for _, root := range roots {
		allRepos[root.repoID] = struct{}{}
		if root.path.Valid && exists(root.path.String) {
			liveByRepo[root.repoID]++
			continue
		}
		if root.path.Valid {
			report.StaleRoots = append(report.StaleRoots, root.path.String)
		}
	}
	sort.Strings(report.StaleRoots)
	var pruneRepoIDs []string
	for repoID := range allRepos {
		if liveByRepo[repoID] == 0 {
			pruneRepoIDs = append(pruneRepoIDs, repoID)
		}
	}
	sort.Strings(pruneRepoIDs)
	report.RepositoriesPruned = len(pruneRepoIDs)
	if len(pruneRepoIDs) > 0 {
		args := append(stringsToAny(pruneRepoIDs), stringsToAny(pruneRepoIDs)...)
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM artifacts a
			WHERE a.repo_id IN (`+questionMarks(len(pruneRepoIDs))+`)
			  AND NOT EXISTS (
				SELECT 1 FROM sources s
				WHERE s.artifact_id = a.id
				  AND s.repo_id NOT IN (`+questionMarks(len(pruneRepoIDs))+`)
			  )`, args...).Scan(&report.ArtifactsPruned); err != nil {
			return report, err
		}
	}
	duplicateArgs := stringsToAny(pruneRepoIDs)
	if err := db.QueryRow("SELECT COUNT(*) FROM ("+duplicateCaptureRevisionPairsSQL(len(pruneRepoIDs))+")", duplicateArgs...).Scan(&report.RevisionsCompacted); err != nil {
		return report, fmt.Errorf("inspect duplicate capture revisions: %w", err)
	}
	if opts.DryRun {
		report.BytesAfter = report.BytesBefore
		emitPruneProgress(opts.Progress, "complete")
		return report, nil
	}

	if len(report.StaleRoots) > 0 || len(pruneRepoIDs) > 0 {
		emitPruneProgress(opts.Progress, "delete")
	}
	tx, err := db.Begin()
	if err != nil {
		return report, err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	for _, path := range report.StaleRoots {
		if _, err := tx.Exec("DELETE FROM repo_roots WHERE root_path = ?", path); err != nil {
			return report, fmt.Errorf("delete stale root %q: %w", path, err)
		}
	}
	if len(pruneRepoIDs) > 0 {
		if err := deleteRepositories(tx, pruneRepoIDs); err != nil {
			return report, err
		}
	}
	compacted, err := compactDuplicateCaptureRevisions(tx)
	if err != nil {
		return report, err
	}
	report.RevisionsCompacted = compacted
	if _, err := tx.Exec(`
		UPDATE repos
		SET root_path = (SELECT MIN(rr.root_path) FROM repo_roots rr WHERE rr.repo_id = repos.id)
		WHERE NOT EXISTS (SELECT 1 FROM repo_roots current WHERE current.repo_id = repos.id AND current.root_path = repos.root_path)
		  AND EXISTS (SELECT 1 FROM repo_roots remaining WHERE remaining.repo_id = repos.id)`); err != nil {
		return report, fmt.Errorf("refresh repository primary roots: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	rollback = false

	if opts.Vacuum {
		emitPruneProgress(opts.Progress, "compact")
		if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			return report, fmt.Errorf("checkpoint index before vacuum: %w", err)
		}
		if _, err := db.Exec("VACUUM"); err != nil {
			return report, fmt.Errorf("vacuum index: %w", err)
		}
		report.Vacuumed = true
	}
	report.BytesAfter = fileSize(dbPath)
	emitPruneProgress(opts.Progress, "complete")
	return report, nil
}

const duplicateCaptureRevisionPairsSQLTemplate = `
	WITH revision_order AS (
		SELECT r.id, r.artifact_id, r.content_hash, r.observed_at, a.current_revision_id,
			CASE WHEN r.content_hash = LAG(r.content_hash) OVER (
				PARTITION BY r.artifact_id ORDER BY r.observed_at, r.id
			) THEN 0 ELSE 1 END AS starts_run
		FROM artifact_revisions r
		JOIN artifacts a ON a.id = r.artifact_id
		WHERE {{REPO_FILTER}}EXISTS (
			SELECT 1 FROM sources s
			WHERE s.artifact_id = r.artifact_id AND s.source_type = 'capture'
		)
	), revision_runs AS (
		SELECT *, SUM(starts_run) OVER (
			PARTITION BY artifact_id ORDER BY observed_at, id ROWS UNBOUNDED PRECEDING
		) AS run_id
		FROM revision_order
	), revision_keeps AS (
		SELECT id,
			FIRST_VALUE(id) OVER (
				PARTITION BY artifact_id, run_id
				ORDER BY CASE WHEN id = current_revision_id THEN 1 ELSE 0 END DESC, observed_at DESC, id DESC
			) AS keep_id
		FROM revision_runs
	)
	SELECT id, keep_id FROM revision_keeps WHERE id <> keep_id`

func duplicateCaptureRevisionPairsSQL(excludedRepoCount int) string {
	filter := ""
	if excludedRepoCount > 0 {
		filter = "a.repo_id NOT IN (" + questionMarks(excludedRepoCount) + ") AND "
	}
	return strings.Replace(duplicateCaptureRevisionPairsSQLTemplate, "{{REPO_FILTER}}", filter, 1)
}

func compactDuplicateCaptureRevisions(tx *sql.Tx) (int, error) {
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS prune_duplicate_revisions (
		delete_id TEXT PRIMARY KEY,
		keep_id TEXT NOT NULL
	)`); err != nil {
		return 0, fmt.Errorf("create duplicate revision work table: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM prune_duplicate_revisions"); err != nil {
		return 0, fmt.Errorf("reset duplicate revision work table: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO prune_duplicate_revisions (delete_id, keep_id) " + duplicateCaptureRevisionPairsSQL(0)); err != nil {
		return 0, fmt.Errorf("select duplicate capture revisions: %w", err)
	}
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM prune_duplicate_revisions").Scan(&count); err != nil {
		return 0, fmt.Errorf("count duplicate capture revisions: %w", err)
	}
	if count == 0 {
		return 0, nil
	}
	for _, table := range []string{"artifact_todos", "artifact_criteria", "artifact_sections"} {
		if _, err := tx.Exec(`UPDATE ` + table + `
			SET revision_id = (SELECT keep_id FROM prune_duplicate_revisions WHERE delete_id = revision_id)
			WHERE revision_id IN (SELECT delete_id FROM prune_duplicate_revisions)`); err != nil {
			return 0, fmt.Errorf("remap %s duplicate revisions: %w", table, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM artifact_revisions
		WHERE id IN (SELECT delete_id FROM prune_duplicate_revisions)`); err != nil {
		return 0, fmt.Errorf("delete duplicate capture revisions: %w", err)
	}
	return count, nil
}

func emitPruneProgress(progress func(PruneProgress), phase string) {
	if progress != nil {
		progress(PruneProgress{Phase: phase})
	}
}

func deleteRepositories(tx *sql.Tx, repoIDs []string) error {
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS prune_repo_ids (id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM prune_repo_ids`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO prune_repo_ids (id) VALUES (?)`)
	if err != nil {
		return err
	}
	for _, id := range repoIDs {
		if _, err := stmt.Exec(id); err != nil {
			stmt.Close()
			return err
		}
	}
	if err := stmt.Close(); err != nil {
		return err
	}

	// Repair legacy cross-repository source ownership before selecting artifacts
	// to delete. Older scans matched relative source identities globally.
	if _, err := tx.Exec(`
		UPDATE artifacts AS a
		SET repo_id = (
			SELECT MIN(s.repo_id) FROM sources s
			WHERE s.artifact_id = a.id
			  AND s.repo_id NOT IN (SELECT id FROM prune_repo_ids)
		)
		WHERE a.repo_id IN (SELECT id FROM prune_repo_ids)
		  AND EXISTS (
			SELECT 1 FROM sources s
			WHERE s.artifact_id = a.id
			  AND s.repo_id NOT IN (SELECT id FROM prune_repo_ids)
		  )`); err != nil {
		return fmt.Errorf("repair legacy artifact ownership: %w", err)
	}
	// A surviving artifact can also carry a source row written under a stale
	// repository ID. Preserve that evidence and normalize it to the artifact's
	// surviving owner before deleting repository-scoped rows.
	if _, err := tx.Exec(`
		UPDATE sources AS s
		SET repo_id = (SELECT a.repo_id FROM artifacts a WHERE a.id = s.artifact_id)
		WHERE s.repo_id IN (SELECT id FROM prune_repo_ids)
		  AND EXISTS (
			SELECT 1 FROM artifacts a
			WHERE a.id = s.artifact_id
			  AND a.repo_id NOT IN (SELECT id FROM prune_repo_ids)
		  )`); err != nil {
		return fmt.Errorf("repair legacy source ownership: %w", err)
	}
	if _, err := tx.Exec(`CREATE TEMP TABLE IF NOT EXISTS prune_artifact_ids (id TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM prune_artifact_ids`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO prune_artifact_ids SELECT id FROM artifacts WHERE repo_id IN (SELECT id FROM prune_repo_ids)`); err != nil {
		return err
	}

	statements := []string{
		`DELETE FROM source_manifest_fts WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id IN (SELECT id FROM prune_repo_ids))`,
		`DELETE FROM source_manifest_symbols WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id IN (SELECT id FROM prune_repo_ids))`,
		`DELETE FROM source_manifest_tests WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id IN (SELECT id FROM prune_repo_ids))`,
		`DELETE FROM source_manifest_imports WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id IN (SELECT id FROM prune_repo_ids))`,
		`DELETE FROM source_manifest WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM task_checkpoint_facts WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM git_commit_files WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM git_commits WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM artifact_edges WHERE repo_id IN (SELECT id FROM prune_repo_ids) OR src_artifact_id IN (SELECT id FROM prune_artifact_ids) OR dst_artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM concept_mentions WHERE concept_id IN (SELECT id FROM concepts WHERE repo_id IN (SELECT id FROM prune_repo_ids)) OR artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM concepts WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM artifact_sections_fts WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifacts_fts WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifact_todos WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifact_criteria WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifact_tags WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifact_sections WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM links WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM sources WHERE repo_id IN (SELECT id FROM prune_repo_ids) OR artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifact_revisions WHERE artifact_id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM artifacts WHERE id IN (SELECT id FROM prune_artifact_ids)`,
		`DELETE FROM repo_roots WHERE repo_id IN (SELECT id FROM prune_repo_ids)`,
		`DELETE FROM repos WHERE id IN (SELECT id FROM prune_repo_ids)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("prune repository index rows: %w", err)
		}
	}
	return nil
}

func (db *DB) databasePath() string {
	rows, err := db.Query("PRAGMA database_list")
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if rows.Scan(&seq, &name, &path) == nil && name == "main" {
			return path
		}
	}
	return ""
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func questionMarks(count int) string {
	if count <= 0 {
		return ""
	}
	out := "?"
	for i := 1; i < count; i++ {
		out += ",?"
	}
	return out
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}
