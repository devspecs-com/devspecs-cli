package store

import (
	"database/sql"
	"fmt"
)

// ThreadProjection is a rebuildable query snapshot of durable thread artifacts.
type ThreadProjection struct {
	OwnerID            string
	OwnerKind          string
	TaskID             string
	WorkspaceID        string
	ChangeID           string
	RepoRoot           string
	WorkspaceRoot      string
	DefinitionPath     string
	DefinitionRevision int
	DefinitionJSON     string
	ProjectedAt        string
	Links              []ThreadProjectionLink
	Tasks              []ThreadProjectionTask
	Events             []ThreadProjectionEvent
	Sources            []ThreadProjectionSource
}

type ThreadProjectionLink struct {
	Position      int
	RepoAlias     string
	TaskID        string
	Target        string
	Name          string
	Status        string
	RepoRoot      string
	TaskWorkspace string
}

type ThreadProjectionTask struct {
	RepoAlias     string
	TaskID        string
	RepoRoot      string
	TaskWorkspace string
	ManifestPath  string
	ManifestJSON  string
}

type ThreadProjectionEvent struct {
	RepoAlias    string
	TaskID       string
	CheckpointID string
	Target       string
	JSONPath     string
	MarkdownPath string
	RecordJSON   string
}

type ThreadProjectionSource struct {
	SourceKind  string
	SourceKey   string
	Path        string
	SizeBytes   int64
	ModifiedNS  int64
	ContentHash string
}

// ReplaceThreadProjection atomically replaces one owner's derived projection.
func (db *DB) ReplaceThreadProjection(projection ThreadProjection) error {
	if projection.OwnerID == "" || projection.OwnerKind == "" || projection.DefinitionPath == "" {
		return fmt.Errorf("thread projection requires owner ID, owner kind, and definition path")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.Exec(`DELETE FROM thread_projections WHERE owner_id = ?`, projection.OwnerID); err != nil {
		return fmt.Errorf("delete prior thread projection: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO thread_projections (
		owner_id, owner_kind, task_id, workspace_id, change_id, repo_root,
		workspace_root, definition_path, definition_revision, definition_json, projected_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projection.OwnerID, projection.OwnerKind, projection.TaskID, projection.WorkspaceID,
		projection.ChangeID, projection.RepoRoot, projection.WorkspaceRoot,
		projection.DefinitionPath, projection.DefinitionRevision,
		nonEmptyJSON(projection.DefinitionJSON, "{}"), projection.ProjectedAt,
	); err != nil {
		return fmt.Errorf("insert thread projection: %w", err)
	}
	if err := insertThreadProjectionLinks(tx, projection.OwnerID, projection.Links); err != nil {
		return err
	}
	if err := insertThreadProjectionTasks(tx, projection.OwnerID, projection.Tasks); err != nil {
		return err
	}
	if err := insertThreadProjectionEvents(tx, projection.OwnerID, projection.Events); err != nil {
		return err
	}
	if err := insertThreadProjectionSources(tx, projection.OwnerID, projection.Sources); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func insertThreadProjectionLinks(tx *sql.Tx, ownerID string, links []ThreadProjectionLink) error {
	statement, err := tx.Prepare(`INSERT INTO thread_projection_links (
		owner_id, position, repo_alias, task_id, target, name, status, repo_root, task_workspace
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, link := range links {
		if _, err := statement.Exec(ownerID, link.Position, link.RepoAlias, link.TaskID, link.Target,
			link.Name, link.Status, link.RepoRoot, link.TaskWorkspace); err != nil {
			return fmt.Errorf("insert thread projection link: %w", err)
		}
	}
	return nil
}

func insertThreadProjectionTasks(tx *sql.Tx, ownerID string, tasks []ThreadProjectionTask) error {
	statement, err := tx.Prepare(`INSERT INTO thread_projection_tasks (
		owner_id, repo_alias, task_id, repo_root, task_workspace, manifest_path, manifest_json
	) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, task := range tasks {
		if _, err := statement.Exec(ownerID, task.RepoAlias, task.TaskID, task.RepoRoot,
			task.TaskWorkspace, task.ManifestPath, nonEmptyJSON(task.ManifestJSON, "{}")); err != nil {
			return fmt.Errorf("insert thread projection task: %w", err)
		}
	}
	return nil
}

func insertThreadProjectionEvents(tx *sql.Tx, ownerID string, events []ThreadProjectionEvent) error {
	statement, err := tx.Prepare(`INSERT INTO thread_projection_events (
		owner_id, repo_alias, task_id, checkpoint_id, target, json_path, markdown_path, record_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, event := range events {
		if _, err := statement.Exec(ownerID, event.RepoAlias, event.TaskID, event.CheckpointID,
			event.Target, event.JSONPath, event.MarkdownPath, nonEmptyJSON(event.RecordJSON, "{}")); err != nil {
			return fmt.Errorf("insert thread projection event: %w", err)
		}
	}
	return nil
}

func insertThreadProjectionSources(tx *sql.Tx, ownerID string, sources []ThreadProjectionSource) error {
	statement, err := tx.Prepare(`INSERT INTO thread_projection_sources (
		owner_id, source_kind, source_key, path, size_bytes, modified_ns, content_hash
	) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, source := range sources {
		if _, err := statement.Exec(ownerID, source.SourceKind, source.SourceKey, source.Path,
			source.SizeBytes, source.ModifiedNS, source.ContentHash); err != nil {
			return fmt.Errorf("insert thread projection source: %w", err)
		}
	}
	return nil
}

// GetThreadProjection loads one projected owner and its ordered source records.
func (db *DB) GetThreadProjection(ownerID string) (ThreadProjection, bool, error) {
	var projection ThreadProjection
	err := db.QueryRow(`SELECT owner_id, owner_kind, task_id, workspace_id, change_id,
		repo_root, workspace_root, definition_path, definition_revision, definition_json, projected_at
		FROM thread_projections WHERE owner_id = ?`, ownerID).Scan(
		&projection.OwnerID, &projection.OwnerKind, &projection.TaskID, &projection.WorkspaceID,
		&projection.ChangeID, &projection.RepoRoot, &projection.WorkspaceRoot,
		&projection.DefinitionPath, &projection.DefinitionRevision,
		&projection.DefinitionJSON, &projection.ProjectedAt,
	)
	if err == sql.ErrNoRows {
		return ThreadProjection{}, false, nil
	}
	if err != nil {
		return ThreadProjection{}, false, err
	}
	if projection.Links, err = db.listThreadProjectionLinks(ownerID); err != nil {
		return ThreadProjection{}, false, err
	}
	if projection.Tasks, err = db.listThreadProjectionTasks(ownerID); err != nil {
		return ThreadProjection{}, false, err
	}
	if projection.Events, err = db.listThreadProjectionEvents(ownerID); err != nil {
		return ThreadProjection{}, false, err
	}
	if projection.Sources, err = db.listThreadProjectionSources(ownerID); err != nil {
		return ThreadProjection{}, false, err
	}
	return projection, true, nil
}

func (db *DB) listThreadProjectionLinks(ownerID string) ([]ThreadProjectionLink, error) {
	rows, err := db.Query(`SELECT position, repo_alias, task_id, target, name, status, repo_root, task_workspace
		FROM thread_projection_links WHERE owner_id = ? ORDER BY position`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []ThreadProjectionLink
	for rows.Next() {
		var link ThreadProjectionLink
		if err := rows.Scan(&link.Position, &link.RepoAlias, &link.TaskID, &link.Target,
			&link.Name, &link.Status, &link.RepoRoot, &link.TaskWorkspace); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (db *DB) listThreadProjectionTasks(ownerID string) ([]ThreadProjectionTask, error) {
	rows, err := db.Query(`SELECT repo_alias, task_id, repo_root, task_workspace, manifest_path, manifest_json
		FROM thread_projection_tasks WHERE owner_id = ? ORDER BY repo_alias, task_id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []ThreadProjectionTask
	for rows.Next() {
		var task ThreadProjectionTask
		if err := rows.Scan(&task.RepoAlias, &task.TaskID, &task.RepoRoot, &task.TaskWorkspace,
			&task.ManifestPath, &task.ManifestJSON); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (db *DB) listThreadProjectionEvents(ownerID string) ([]ThreadProjectionEvent, error) {
	rows, err := db.Query(`SELECT repo_alias, task_id, checkpoint_id, target, json_path, markdown_path, record_json
		FROM thread_projection_events WHERE owner_id = ? ORDER BY repo_alias, task_id, checkpoint_id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []ThreadProjectionEvent
	for rows.Next() {
		var event ThreadProjectionEvent
		if err := rows.Scan(&event.RepoAlias, &event.TaskID, &event.CheckpointID, &event.Target,
			&event.JSONPath, &event.MarkdownPath, &event.RecordJSON); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (db *DB) listThreadProjectionSources(ownerID string) ([]ThreadProjectionSource, error) {
	rows, err := db.Query(`SELECT source_kind, source_key, path, size_bytes, modified_ns, content_hash
		FROM thread_projection_sources WHERE owner_id = ? ORDER BY source_kind, source_key`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []ThreadProjectionSource
	for rows.Next() {
		var source ThreadProjectionSource
		if err := rows.Scan(&source.SourceKind, &source.SourceKey, &source.Path,
			&source.SizeBytes, &source.ModifiedNS, &source.ContentHash); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// DeleteThreadProjection removes one rebuildable owner snapshot.
func (db *DB) DeleteThreadProjection(ownerID string) error {
	_, err := db.Exec(`DELETE FROM thread_projections WHERE owner_id = ?`, ownerID)
	return err
}
