package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// ArtifactRow represents a row from the artifacts table.
type ArtifactRow struct {
	ID             string
	RepoID         string
	Kind           string
	Title          string
	Status         string
	CurrentRevID   string
	CreatedAt      string
	UpdatedAt      string
	LastObservedAt string
}

// RevisionRow represents a row from the artifact_revisions table.
type RevisionRow struct {
	ID            string
	ArtifactID    string
	ContentHash   string
	Body          string
	ExtractedJSON string
	ObservedAt    string
}

// SourceRow represents a row from the sources table.
type SourceRow struct {
	ID             string
	ArtifactID     string
	SourceType     string
	Path           string
	SourceIdentity string
}

// LinkRow represents a row from the links table.
type LinkRow struct {
	ID         string
	ArtifactID string
	LinkType   string
	Target     string
	CreatedAt  string
}

// TodoRow represents a row from the artifact_todos table.
type TodoRow struct {
	ID         string
	ArtifactID string
	RevisionID string
	Ordinal    int
	Text       string
	Done       bool
	SourceFile string
	SourceLine int
}

// ListArtifacts returns artifacts optionally filtered by kind, status, source type.
func (db *DB) ListArtifacts(repoRoot, kind, status, sourceType string) ([]ArtifactRow, error) {
	query := "SELECT a.id, a.repo_id, a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at FROM artifacts a"
	var conditions []string
	var args []any

	if repoRoot != "" {
		query += " JOIN repos r ON a.repo_id = r.id"
		conditions = append(conditions, "r.root_path = ?")
		args = append(args, repoRoot)
	}
	if kind != "" {
		conditions = append(conditions, "a.kind = ?")
		args = append(args, kind)
	}
	if status != "" {
		conditions = append(conditions, "a.status = ?")
		args = append(args, status)
	}
	if sourceType != "" {
		query += " JOIN sources s ON s.artifact_id = a.id"
		conditions = append(conditions, "s.source_type = ?")
		args = append(args, sourceType)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY a.last_observed_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ArtifactRow
	for rows.Next() {
		var r ArtifactRow
		if err := rows.Scan(&r.ID, &r.RepoID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetArtifact retrieves a single artifact by full or prefix ID.
func (db *DB) GetArtifact(idOrPrefix string) (*ArtifactRow, error) {
	var r ArtifactRow
	err := db.QueryRow(
		"SELECT id, repo_id, kind, title, status, COALESCE(current_revision_id,''), created_at, updated_at, last_observed_at FROM artifacts WHERE id = ?",
		idOrPrefix,
	).Scan(&r.ID, &r.RepoID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt)
	if err == nil {
		return &r, nil
	}

	// Try prefix match
	rows, err := db.Query(
		"SELECT id, repo_id, kind, title, status, COALESCE(current_revision_id,''), created_at, updated_at, last_observed_at FROM artifacts WHERE id LIKE ?",
		idOrPrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []ArtifactRow
	for rows.Next() {
		var m ArtifactRow
		if err := rows.Scan(&m.ID, &m.RepoID, &m.Kind, &m.Title, &m.Status, &m.CurrentRevID, &m.CreatedAt, &m.UpdatedAt, &m.LastObservedAt); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("artifact not found: %s", idOrPrefix)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous ID prefix %q matches %d artifacts", idOrPrefix, len(matches))
	}
}

// GetRevision retrieves a revision by ID.
func (db *DB) GetRevision(id string) (*RevisionRow, error) {
	var r RevisionRow
	err := db.QueryRow(
		"SELECT id, artifact_id, content_hash, body, COALESCE(extracted_json,''), observed_at FROM artifact_revisions WHERE id = ?",
		id,
	).Scan(&r.ID, &r.ArtifactID, &r.ContentHash, &r.Body, &r.ExtractedJSON, &r.ObservedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetSourcesForArtifact returns all sources for an artifact.
func (db *DB) GetSourcesForArtifact(artifactID string) ([]SourceRow, error) {
	rows, err := db.Query(
		"SELECT id, artifact_id, source_type, COALESCE(path,''), source_identity FROM sources WHERE artifact_id = ?",
		artifactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SourceRow
	for rows.Next() {
		var r SourceRow
		if err := rows.Scan(&r.ID, &r.ArtifactID, &r.SourceType, &r.Path, &r.SourceIdentity); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetLinksForArtifact returns all links for an artifact.
func (db *DB) GetLinksForArtifact(artifactID string) ([]LinkRow, error) {
	rows, err := db.Query(
		"SELECT id, artifact_id, link_type, target, created_at FROM links WHERE artifact_id = ?",
		artifactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []LinkRow
	for rows.Next() {
		var r LinkRow
		if err := rows.Scan(&r.ID, &r.ArtifactID, &r.LinkType, &r.Target, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetTodosForArtifact returns todos for a specific artifact.
func (db *DB) GetTodosForArtifact(artifactID string) ([]TodoRow, error) {
	rows, err := db.Query(
		"SELECT id, artifact_id, revision_id, ordinal, text, done, source_file, source_line FROM artifact_todos WHERE artifact_id = ? ORDER BY ordinal",
		artifactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TodoRow
	for rows.Next() {
		var r TodoRow
		if err := rows.Scan(&r.ID, &r.ArtifactID, &r.RevisionID, &r.Ordinal, &r.Text, &r.Done, &r.SourceFile, &r.SourceLine); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListAllTodos returns todos across all artifacts, optionally filtered.
func (db *DB) ListAllTodos(repoRoot string, openOnly, doneOnly bool) ([]TodoRow, error) {
	query := `SELECT t.id, t.artifact_id, t.revision_id, t.ordinal, t.text, t.done, t.source_file, t.source_line
		FROM artifact_todos t
		JOIN artifacts a ON a.id = t.artifact_id`
	var conditions []string
	var args []any

	if repoRoot != "" {
		query += " JOIN repos r ON a.repo_id = r.id"
		conditions = append(conditions, "r.root_path = ?")
		args = append(args, repoRoot)
	}
	if openOnly {
		conditions = append(conditions, "t.done = 0")
	}
	if doneOnly {
		conditions = append(conditions, "t.done = 1")
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY a.title, t.ordinal"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TodoRow
	for rows.Next() {
		var r TodoRow
		if err := rows.Scan(&r.ID, &r.ArtifactID, &r.RevisionID, &r.Ordinal, &r.Text, &r.Done, &r.SourceFile, &r.SourceLine); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// FindArtifacts does a text search across title, source path, and body.
// It tries FTS5 first, falling back to LIKE if FTS returns no results or errors.
func (db *DB) FindArtifacts(query string, kind string) ([]ArtifactRow, error) {
	result, err := db.findArtifactsFTS(query, kind)
	if err == nil && len(result) > 0 {
		return result, nil
	}
	return db.findArtifactsLIKE(query, kind)
}

func (db *DB) findArtifactsFTS(query string, kind string) ([]ArtifactRow, error) {
	sqlQuery := `SELECT DISTINCT a.id, a.repo_id, a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at
		FROM artifacts_fts f
		JOIN artifacts a ON a.id = f.artifact_id
		WHERE artifacts_fts MATCH ?`
	args := []any{query}

	if kind != "" {
		sqlQuery += " AND a.kind = ?"
		args = append(args, kind)
	}
	sqlQuery += " ORDER BY a.last_observed_at DESC"

	rows, err := db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ArtifactRow
	for rows.Next() {
		var r ArtifactRow
		if err := rows.Scan(&r.ID, &r.RepoID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) findArtifactsLIKE(query string, kind string) ([]ArtifactRow, error) {
	likePattern := "%" + query + "%"
	sqlQuery := `SELECT DISTINCT a.id, a.repo_id, a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at
		FROM artifacts a
		LEFT JOIN sources s ON s.artifact_id = a.id
		LEFT JOIN artifact_revisions r ON r.id = a.current_revision_id
		WHERE (a.title LIKE ? OR s.path LIKE ? OR r.body LIKE ?)`
	args := []any{likePattern, likePattern, likePattern}

	if kind != "" {
		sqlQuery += " AND a.kind = ?"
		args = append(args, kind)
	}
	sqlQuery += " ORDER BY a.last_observed_at DESC"

	rows, err := db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ArtifactRow
	for rows.Next() {
		var r ArtifactRow
		if err := rows.Scan(&r.ID, &r.RepoID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// IndexArtifactFTS inserts or updates the FTS5 index for an artifact.
func (db *DB) IndexArtifactFTS(artifactID, title, body, sourcePath string) error {
	db.Exec("DELETE FROM artifacts_fts WHERE artifact_id = ?", artifactID)
	_, err := db.Exec("INSERT INTO artifacts_fts (artifact_id, title, body, source_path) VALUES (?, ?, ?, ?)",
		artifactID, title, body, sourcePath)
	return err
}

// InsertLink adds a link for an artifact.
func (db *DB) InsertLink(id, artifactID, linkType, target, now string) error {
	_, err := db.Exec(
		"INSERT INTO links (id, artifact_id, link_type, target, created_at) VALUES (?, ?, ?, ?, ?)",
		id, artifactID, linkType, target, now,
	)
	return err
}

// UpdateArtifactStatus updates the status of an artifact.
func (db *DB) UpdateArtifactStatus(artifactID, status, now string) error {
	_, err := db.Exec("UPDATE artifacts SET status = ?, updated_at = ? WHERE id = ?", status, now, artifactID)
	return err
}

// InsertArtifactDirect allows inserting an artifact directly (for capture).
func (db *DB) InsertArtifactDirect(id, repoID, kind, title, status, revID, now string) error {
	_, err := db.Exec(
		`INSERT INTO artifacts (id, repo_id, kind, title, status, current_revision_id, created_at, updated_at, last_observed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, repoID, kind, title, status, revID, now, now, now,
	)
	return err
}

// InsertRevisionDirect inserts a revision directly.
func (db *DB) InsertRevisionDirect(id, artifactID, contentHash, body, now string) error {
	_, err := db.Exec(
		"INSERT INTO artifact_revisions (id, artifact_id, content_hash, body, observed_at) VALUES (?, ?, ?, ?, ?)",
		id, artifactID, contentHash, body, now,
	)
	return err
}

// InsertSourceDirect inserts a source directly.
func (db *DB) InsertSourceDirect(id, artifactID, repoID, sourceType, path, sourceIdentity, now string) error {
	_, err := db.Exec(
		"INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, artifactID, repoID, sourceType, path, sourceIdentity, now, now,
	)
	return err
}

// FindSourceByIdentity checks if a source_identity already exists and returns the artifact ID.
func (db *DB) FindSourceByIdentity(identity string) (string, error) {
	var artifactID string
	err := db.QueryRow("SELECT artifact_id FROM sources WHERE source_identity = ?", identity).Scan(&artifactID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return artifactID, err
}

// EnsureRepo creates or returns the repo ID for the given root path.
func (db *DB) EnsureRepo(rootPath, now string) (string, error) {
	var id string
	err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", rootPath).Scan(&id)
	if err == nil {
		return id, nil
	}
	// Not found, but we won't create here — let scan do that
	return "", fmt.Errorf("repo not found for %s", rootPath)
}

// RepoMeta holds freshness metadata for a repository.
type RepoMeta struct {
	ID             string
	RootPath       string
	LastScanCommit string
	LastScanAt     string
}

// GetRepoByRoot returns the repo row for a given root path, or nil if not found.
func (db *DB) GetRepoByRoot(rootPath string) *RepoMeta {
	var m RepoMeta
	err := db.QueryRow(
		"SELECT id, root_path, COALESCE(last_scan_commit,''), COALESCE(last_scan_at,'') FROM repos WHERE root_path = ?",
		rootPath,
	).Scan(&m.ID, &m.RootPath, &m.LastScanCommit, &m.LastScanAt)
	if err != nil {
		return nil
	}
	return &m
}

// UpdateScanMeta records the git commit and timestamp of the last scan.
func (db *DB) UpdateScanMeta(repoID, commit, now string) {
	db.Exec("UPDATE repos SET last_scan_commit = ?, last_scan_at = ? WHERE id = ?", commit, now, repoID)
}
