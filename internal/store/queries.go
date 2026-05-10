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
	ShortID        string
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

// FilterParams groups all query filters.
type FilterParams struct {
	RepoRoot   string
	Kind       string
	Status     string
	SourceType string
	Tag        string
	Branch     string
	User       string
}

// TagRow represents a row from the artifact_tags table.
type TagRow struct {
	ArtifactID string
	Tag        string
	Source     string
	CreatedAt  string
}

// ListArtifacts returns artifacts filtered by the given parameters.
func (db *DB) ListArtifacts(fp FilterParams) ([]ArtifactRow, error) {
	query := `SELECT a.id, a.repo_id, COALESCE(a.short_id,''), a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at FROM artifacts a`
	var joins []string
	var conditions []string
	var args []any

	needsRepoJoin := fp.RepoRoot != "" || fp.Branch != "" || fp.User != ""
	if needsRepoJoin {
		joins = append(joins, "JOIN repos r ON a.repo_id = r.id")
	}
	if fp.RepoRoot != "" {
		conditions = append(conditions, "r.root_path = ?")
		args = append(args, fp.RepoRoot)
	}
	if fp.Branch != "" {
		conditions = append(conditions, "r.git_current_branch = ?")
		args = append(args, fp.Branch)
	}
	if fp.User != "" {
		conditions = append(conditions, "r.scanned_by = ?")
		args = append(args, fp.User)
	}
	if fp.Kind != "" {
		conditions = append(conditions, "a.kind = ?")
		args = append(args, fp.Kind)
	}
	if fp.Status != "" {
		conditions = append(conditions, "a.status = ?")
		args = append(args, fp.Status)
	}
	if fp.SourceType != "" {
		joins = append(joins, "JOIN sources s ON s.artifact_id = a.id")
		conditions = append(conditions, "s.source_type = ?")
		args = append(args, fp.SourceType)
	}
	if fp.Tag != "" {
		joins = append(joins, "JOIN artifact_tags at ON at.artifact_id = a.id")
		conditions = append(conditions, "at.tag = ?")
		args = append(args, fp.Tag)
	}

	for _, j := range joins {
		query += " " + j
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
		if err := rows.Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetArtifact retrieves a single artifact by full ID, short_id, or prefix.
func (db *DB) GetArtifact(idOrPrefix string) (*ArtifactRow, error) {
	const cols = `id, repo_id, COALESCE(short_id,''), kind, title, status, COALESCE(current_revision_id,''), created_at, updated_at, last_observed_at`

	// 1. Exact full ID match
	var r ArtifactRow
	err := db.QueryRow(
		"SELECT "+cols+" FROM artifacts WHERE id = ?", idOrPrefix,
	).Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt)
	if err == nil {
		return &r, nil
	}

	// 2. Exact short_id match
	err = db.QueryRow(
		"SELECT "+cols+" FROM artifacts WHERE short_id = ?", idOrPrefix,
	).Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt)
	if err == nil {
		return &r, nil
	}

	// 3. Prefix match on full ID
	rows, err := db.Query(
		"SELECT "+cols+" FROM artifacts WHERE id LIKE ?", idOrPrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []ArtifactRow
	for rows.Next() {
		var m ArtifactRow
		if err := rows.Scan(&m.ID, &m.RepoID, &m.ShortID, &m.Kind, &m.Title, &m.Status, &m.CurrentRevID, &m.CreatedAt, &m.UpdatedAt, &m.LastObservedAt); err != nil {
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
func (db *DB) ListAllTodos(fp FilterParams, openOnly, doneOnly bool) ([]TodoRow, error) {
	query := `SELECT t.id, t.artifact_id, t.revision_id, t.ordinal, t.text, t.done, t.source_file, t.source_line
		FROM artifact_todos t
		JOIN artifacts a ON a.id = t.artifact_id`
	var joins []string
	var conditions []string
	var args []any

	needsRepoJoin := fp.RepoRoot != "" || fp.Branch != "" || fp.User != ""
	if needsRepoJoin {
		joins = append(joins, "JOIN repos r ON a.repo_id = r.id")
	}
	if fp.RepoRoot != "" {
		conditions = append(conditions, "r.root_path = ?")
		args = append(args, fp.RepoRoot)
	}
	if fp.Branch != "" {
		conditions = append(conditions, "r.git_current_branch = ?")
		args = append(args, fp.Branch)
	}
	if fp.User != "" {
		conditions = append(conditions, "r.scanned_by = ?")
		args = append(args, fp.User)
	}
	if fp.Tag != "" {
		joins = append(joins, "JOIN artifact_tags at ON at.artifact_id = a.id")
		conditions = append(conditions, "at.tag = ?")
		args = append(args, fp.Tag)
	}
	if openOnly {
		conditions = append(conditions, "t.done = 0")
	}
	if doneOnly {
		conditions = append(conditions, "t.done = 1")
	}

	for _, j := range joins {
		query += " " + j
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
func (db *DB) FindArtifacts(query string, fp FilterParams) ([]ArtifactRow, error) {
	result, err := db.findArtifactsFTS(query, fp)
	if err == nil && len(result) > 0 {
		return result, nil
	}
	return db.findArtifactsLIKE(query, fp)
}

func (db *DB) findArtifactsFTS(query string, fp FilterParams) ([]ArtifactRow, error) {
	sqlQuery := `SELECT DISTINCT a.id, a.repo_id, COALESCE(a.short_id,''), a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at
		FROM artifacts_fts f
		JOIN artifacts a ON a.id = f.artifact_id`
	var conditions []string
	args := []any{query}
	conditions = append(conditions, "artifacts_fts MATCH ?")

	if fp.Kind != "" {
		conditions = append(conditions, "a.kind = ?")
		args = append(args, fp.Kind)
	}
	if fp.Tag != "" {
		sqlQuery += " JOIN artifact_tags at ON at.artifact_id = a.id"
		conditions = append(conditions, "at.tag = ?")
		args = append(args, fp.Tag)
	}
	if fp.RepoRoot != "" || fp.Branch != "" || fp.User != "" {
		sqlQuery += " JOIN repos r ON a.repo_id = r.id"
		if fp.RepoRoot != "" {
			conditions = append(conditions, "r.root_path = ?")
			args = append(args, fp.RepoRoot)
		}
		if fp.Branch != "" {
			conditions = append(conditions, "r.git_current_branch = ?")
			args = append(args, fp.Branch)
		}
		if fp.User != "" {
			conditions = append(conditions, "r.scanned_by = ?")
			args = append(args, fp.User)
		}
	}

	if len(conditions) > 0 {
		sqlQuery += " WHERE " + strings.Join(conditions, " AND ")
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
		if err := rows.Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (db *DB) findArtifactsLIKE(query string, fp FilterParams) ([]ArtifactRow, error) {
	likePattern := "%" + query + "%"
	sqlQuery := `SELECT DISTINCT a.id, a.repo_id, COALESCE(a.short_id,''), a.kind, a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at
		FROM artifacts a
		LEFT JOIN sources src ON src.artifact_id = a.id
		LEFT JOIN artifact_revisions rv ON rv.id = a.current_revision_id`
	var conditions []string
	args := []any{likePattern, likePattern, likePattern}
	conditions = append(conditions, "(a.title LIKE ? OR src.path LIKE ? OR rv.body LIKE ?)")

	if fp.Kind != "" {
		conditions = append(conditions, "a.kind = ?")
		args = append(args, fp.Kind)
	}
	if fp.Tag != "" {
		sqlQuery += " JOIN artifact_tags at ON at.artifact_id = a.id"
		conditions = append(conditions, "at.tag = ?")
		args = append(args, fp.Tag)
	}
	if fp.RepoRoot != "" || fp.Branch != "" || fp.User != "" {
		sqlQuery += " JOIN repos r ON a.repo_id = r.id"
		if fp.RepoRoot != "" {
			conditions = append(conditions, "r.root_path = ?")
			args = append(args, fp.RepoRoot)
		}
		if fp.Branch != "" {
			conditions = append(conditions, "r.git_current_branch = ?")
			args = append(args, fp.Branch)
		}
		if fp.User != "" {
			conditions = append(conditions, "r.scanned_by = ?")
			args = append(args, fp.User)
		}
	}

	if len(conditions) > 0 {
		sqlQuery += " WHERE " + strings.Join(conditions, " AND ")
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
		if err := rows.Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
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
	ScannedBy      string
}

// GetRepoByRoot returns the repo row for a given root path, or nil if not found.
func (db *DB) GetRepoByRoot(rootPath string) *RepoMeta {
	var m RepoMeta
	err := db.QueryRow(
		"SELECT id, root_path, COALESCE(last_scan_commit,''), COALESCE(last_scan_at,''), COALESCE(scanned_by,'') FROM repos WHERE root_path = ?",
		rootPath,
	).Scan(&m.ID, &m.RootPath, &m.LastScanCommit, &m.LastScanAt, &m.ScannedBy)
	if err != nil {
		return nil
	}
	return &m
}

// UpdateScanMeta records the git commit, timestamp, and user of the last scan.
func (db *DB) UpdateScanMeta(repoID, commit, scannedBy, now string) {
	db.Exec("UPDATE repos SET last_scan_commit = ?, last_scan_at = ?, scanned_by = ? WHERE id = ?", commit, now, scannedBy, repoID)
}

// GetTagsForArtifact returns all tags for an artifact.
func (db *DB) GetTagsForArtifact(artifactID string) ([]TagRow, error) {
	rows, err := db.Query(
		"SELECT artifact_id, tag, source, created_at FROM artifact_tags WHERE artifact_id = ? ORDER BY tag",
		artifactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TagRow
	for rows.Next() {
		var r TagRow
		if err := rows.Scan(&r.ArtifactID, &r.Tag, &r.Source, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// InsertTag adds a tag for an artifact. It is a no-op on conflict.
func (db *DB) InsertTag(artifactID, tag, source, now string) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO artifact_tags (artifact_id, tag, source, created_at) VALUES (?, ?, ?, ?)",
		artifactID, tag, source, now,
	)
	return err
}

// DeleteTag removes a specific tag from an artifact.
func (db *DB) DeleteTag(artifactID, tag string) error {
	_, err := db.Exec("DELETE FROM artifact_tags WHERE artifact_id = ? AND tag = ?", artifactID, tag)
	return err
}

// DeleteAutoTags removes all frontmatter and inferred tags for an artifact (preserving manual).
func (db *DB) DeleteAutoTags(artifactID string) error {
	_, err := db.Exec("DELETE FROM artifact_tags WHERE artifact_id = ? AND source IN ('frontmatter', 'inferred')", artifactID)
	return err
}

// ResumeArtifacts returns all artifacts for a repo, with todo counts, sorted by last_observed_at DESC.
func (db *DB) ResumeArtifacts(repoRoot string, fp FilterParams) ([]ResumeRow, error) {
	query := `SELECT a.id, COALESCE(a.short_id,''), a.kind, a.title, a.status, a.last_observed_at,
		COALESCE(s.path, ''),
		(SELECT COUNT(*) FROM artifact_todos t WHERE t.artifact_id = a.id) as total_todos,
		(SELECT COUNT(*) FROM artifact_todos t WHERE t.artifact_id = a.id AND t.done = 0) as open_todos
	FROM artifacts a
	LEFT JOIN sources s ON s.artifact_id = a.id
	JOIN repos r ON a.repo_id = r.id`

	var conditions []string
	var args []any

	conditions = append(conditions, "r.root_path = ?")
	args = append(args, repoRoot)

	if fp.Tag != "" {
		query += " JOIN artifact_tags at ON at.artifact_id = a.id"
		conditions = append(conditions, "at.tag = ?")
		args = append(args, fp.Tag)
	}
	if fp.Branch != "" {
		conditions = append(conditions, "r.git_current_branch = ?")
		args = append(args, fp.Branch)
	}
	if fp.User != "" {
		conditions = append(conditions, "r.scanned_by = ?")
		args = append(args, fp.User)
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

	var result []ResumeRow
	for rows.Next() {
		var r ResumeRow
		if err := rows.Scan(&r.ID, &r.ShortID, &r.Kind, &r.Title, &r.Status, &r.LastObservedAt, &r.SourcePath, &r.TotalTodos, &r.OpenTodos); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ResumeRow holds the data needed for ds resume display.
type ResumeRow struct {
	ID             string
	ShortID        string
	Kind           string
	Title          string
	Status         string
	LastObservedAt string
	SourcePath     string
	TotalTodos     int
	OpenTodos      int
}

// UpdateArtifactShortID sets the short_id for an artifact.
func (db *DB) UpdateArtifactShortID(artifactID, shortID string) error {
	_, err := db.Exec("UPDATE artifacts SET short_id = ? WHERE id = ?", shortID, artifactID)
	return err
}
