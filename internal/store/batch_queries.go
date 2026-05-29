package store

import (
	"database/sql"
	"strings"
)

const batchQueryChunkSize = 500

const artifactRowSelectColumns = `a.id, a.repo_id, COALESCE(a.short_id,''), a.kind, COALESCE(a.subtype,''), a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at`

type artifactFilterConfig struct {
	RepoJoined   bool
	SourceJoined bool
	TagJoined    bool
	SourceAlias  string
}

func appendArtifactFilterClauses(joins *[]string, conditions *[]string, args *[]any, fp FilterParams, cfg artifactFilterConfig) {
	sourceAlias := cfg.SourceAlias
	if sourceAlias == "" {
		sourceAlias = "s"
	}
	if (fp.RepoRoot != "" || fp.Branch != "" || fp.User != "") && !cfg.RepoJoined {
		*joins = append(*joins, "JOIN repos r ON a.repo_id = r.id")
	}
	if fp.SourceType != "" && !cfg.SourceJoined {
		*joins = append(*joins, "JOIN sources "+sourceAlias+" ON "+sourceAlias+".artifact_id = a.id")
	}
	if fp.Tag != "" && !cfg.TagJoined {
		*joins = append(*joins, "JOIN artifact_tags at ON at.artifact_id = a.id")
	}
	if fp.RepoRoot != "" {
		*conditions = append(*conditions, "r.root_path = ?")
		*args = append(*args, fp.RepoRoot)
	}
	if fp.Branch != "" {
		*conditions = append(*conditions, "r.git_current_branch = ?")
		*args = append(*args, fp.Branch)
	}
	if fp.User != "" {
		*conditions = append(*conditions, "r.scanned_by = ?")
		*args = append(*args, fp.User)
	}
	if fp.Kind != "" {
		*conditions = append(*conditions, "a.kind = ?")
		*args = append(*args, fp.Kind)
	}
	if fp.Subtype != "" {
		*conditions = append(*conditions, "a.subtype = ?")
		*args = append(*args, fp.Subtype)
	}
	if fp.Status != "" {
		*conditions = append(*conditions, "a.status = ?")
		*args = append(*args, fp.Status)
	}
	if fp.SourceType != "" {
		*conditions = append(*conditions, sourceAlias+".source_type = ?")
		*args = append(*args, fp.SourceType)
	}
	if fp.Tag != "" {
		*conditions = append(*conditions, "at.tag = ?")
		*args = append(*args, fp.Tag)
	}
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", n), ",")
}

func stringArgs(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

func chunks(values []string, size int) [][]string {
	if size <= 0 {
		size = batchQueryChunkSize
	}
	out := make([][]string, 0, (len(values)+size-1)/size)
	for start := 0; start < len(values); start += size {
		end := start + size
		if end > len(values) {
			end = len(values)
		}
		out = append(out, values[start:end])
	}
	return out
}

// CountArtifacts returns the number of distinct artifacts that match the filters.
func (db *DB) CountArtifacts(fp FilterParams) (int, error) {
	query := "SELECT COUNT(DISTINCT a.id) FROM artifacts a"
	var joins []string
	var conditions []string
	var args []any
	appendArtifactFilterClauses(&joins, &conditions, &args, fp, artifactFilterConfig{})
	if len(joins) > 0 {
		query += " " + strings.Join(joins, " ")
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// ListArtifactsByIDs returns filtered artifact rows for selected IDs in input order.
func (db *DB) ListArtifactsByIDs(ids []string, fp FilterParams) ([]ArtifactRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	byID := make(map[string]ArtifactRow, len(ids))
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		query := "SELECT DISTINCT " + artifactRowSelectColumns + " FROM artifacts a"
		var joins []string
		conditions := []string{"a.id IN (" + placeholders(len(chunk)) + ")"}
		args := stringArgs(chunk)
		appendArtifactFilterClauses(&joins, &conditions, &args, fp, artifactFilterConfig{})
		if len(joins) > 0 {
			query += " " + strings.Join(joins, " ")
		}
		query += " WHERE " + strings.Join(conditions, " AND ")
		rows, err := db.Query(query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r ArtifactRow
			if err := rows.Scan(&r.ID, &r.RepoID, &r.ShortID, &r.Kind, &r.Subtype, &r.Title, &r.Status, &r.CurrentRevID, &r.CreatedAt, &r.UpdatedAt, &r.LastObservedAt); err != nil {
				rows.Close()
				return nil, err
			}
			byID[r.ID] = r
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	out := make([]ArtifactRow, 0, len(byID))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		if row, ok := byID[id]; ok {
			out = append(out, row)
			seen[id] = true
		}
	}
	return out, nil
}

// GetSourcesForArtifacts returns sources grouped by artifact ID.
func (db *DB) GetSourcesForArtifacts(ids []string) (map[string][]SourceRow, error) {
	out := map[string][]SourceRow{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		rows, err := db.Query(
			"SELECT id, artifact_id, source_type, COALESCE(path,''), source_identity, COALESCE(format_profile,''), COALESCE(layout_group,'') FROM sources WHERE artifact_id IN ("+placeholders(len(chunk))+") ORDER BY artifact_id, path, id",
			stringArgs(chunk)...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r SourceRow
			if err := rows.Scan(&r.ID, &r.ArtifactID, &r.SourceType, &r.Path, &r.SourceIdentity, &r.FormatProfile, &r.LayoutGroup); err != nil {
				rows.Close()
				return nil, err
			}
			out[r.ArtifactID] = append(out[r.ArtifactID], r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// GetLinksForArtifacts returns links grouped by artifact ID.
func (db *DB) GetLinksForArtifacts(ids []string) (map[string][]LinkRow, error) {
	out := map[string][]LinkRow{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		rows, err := db.Query(
			"SELECT id, artifact_id, link_type, target, created_at FROM links WHERE artifact_id IN ("+placeholders(len(chunk))+") ORDER BY artifact_id, link_type, target",
			stringArgs(chunk)...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r LinkRow
			if err := rows.Scan(&r.ID, &r.ArtifactID, &r.LinkType, &r.Target, &r.CreatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			out[r.ArtifactID] = append(out[r.ArtifactID], r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// GetTodosForArtifacts returns todos grouped by artifact ID.
func (db *DB) GetTodosForArtifacts(ids []string) (map[string][]TodoRow, error) {
	out := map[string][]TodoRow{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		rows, err := db.Query(
			"SELECT id, artifact_id, revision_id, ordinal, text, done, source_file, source_line FROM artifact_todos WHERE artifact_id IN ("+placeholders(len(chunk))+") ORDER BY artifact_id, ordinal",
			stringArgs(chunk)...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r TodoRow
			if err := rows.Scan(&r.ID, &r.ArtifactID, &r.RevisionID, &r.Ordinal, &r.Text, &r.Done, &r.SourceFile, &r.SourceLine); err != nil {
				rows.Close()
				return nil, err
			}
			out[r.ArtifactID] = append(out[r.ArtifactID], r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// GetSectionsForArtifacts returns persisted sections grouped by artifact ID.
func (db *DB) GetSectionsForArtifacts(ids []string) (map[string][]SectionRow, error) {
	out := map[string][]SectionRow{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		rows, err := db.Query(
			`SELECT id, artifact_id, revision_id, source_path, heading_path, heading_depth, start_line, end_line, title, body, token_estimate, section_kind, metadata_json
			 FROM artifact_sections
			 WHERE artifact_id IN (`+placeholders(len(chunk))+`)
			 ORDER BY artifact_id, start_line, heading_path`,
			stringArgs(chunk)...,
		)
		if err != nil {
			return nil, err
		}
		sections, err := scanSectionRows(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		for _, row := range sections {
			out[row.ArtifactID] = append(out[row.ArtifactID], row)
		}
	}
	return out, nil
}

// GetRevisionsByIDs returns revisions grouped by revision ID.
func (db *DB) GetRevisionsByIDs(ids []string) (map[string]RevisionRow, error) {
	out := map[string]RevisionRow{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, chunk := range chunks(ids, batchQueryChunkSize) {
		rows, err := db.Query(
			"SELECT id, artifact_id, content_hash, body, COALESCE(extracted_json,''), observed_at FROM artifact_revisions WHERE id IN ("+placeholders(len(chunk))+")",
			stringArgs(chunk)...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r RevisionRow
			if err := rows.Scan(&r.ID, &r.ArtifactID, &r.ContentHash, &r.Body, &r.ExtractedJSON, &r.ObservedAt); err != nil {
				rows.Close()
				return nil, err
			}
			out[r.ID] = r
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// FindArtifactIDsFTS returns artifact IDs matching artifact-level FTS.
func (db *DB) FindArtifactIDsFTS(match string, fp FilterParams, limit int) ([]string, error) {
	if strings.TrimSpace(match) == "" || limit <= 0 {
		return nil, nil
	}
	query := `SELECT DISTINCT a.id
		FROM artifacts_fts f
		JOIN artifacts a ON a.id = f.artifact_id`
	var joins []string
	conditions := []string{"artifacts_fts MATCH ?"}
	args := []any{match}
	appendArtifactFilterClauses(&joins, &conditions, &args, fp, artifactFilterConfig{})
	if len(joins) > 0 {
		query += " " + strings.Join(joins, " ")
	}
	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY bm25(artifacts_fts), a.last_observed_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactIDs(rows)
}

// FindArtifactIDsBySectionFTS returns artifact IDs matching section-level FTS.
func (db *DB) FindArtifactIDsBySectionFTS(match string, fp FilterParams, limit int) ([]string, error) {
	if strings.TrimSpace(match) == "" || limit <= 0 {
		return nil, nil
	}
	query := `SELECT DISTINCT s.artifact_id
		FROM artifact_sections_fts
		JOIN artifact_sections s ON s.id = artifact_sections_fts.section_id
		JOIN artifacts a ON a.id = s.artifact_id`
	var joins []string
	conditions := []string{"artifact_sections_fts MATCH ?"}
	args := []any{match}
	appendArtifactFilterClauses(&joins, &conditions, &args, fp, artifactFilterConfig{})
	if len(joins) > 0 {
		query += " " + strings.Join(joins, " ")
	}
	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY bm25(artifact_sections_fts), s.start_line LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactIDs(rows)
}

// FindArtifactIDsByTitleOrPathTerms returns artifact IDs with title/path LIKE matches.
func (db *DB) FindArtifactIDsByTitleOrPathTerms(terms []string, fp FilterParams, limit int) ([]string, error) {
	if len(terms) == 0 || limit <= 0 {
		return nil, nil
	}
	query := `SELECT DISTINCT a.id
		FROM artifacts a
		LEFT JOIN sources src ON src.artifact_id = a.id`
	var joins []string
	var conditions []string
	var args []any
	var termClauses []string
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		termClauses = append(termClauses, "(LOWER(a.title) LIKE ? OR LOWER(src.path) LIKE ?)")
		pattern := "%" + term + "%"
		args = append(args, pattern, pattern)
	}
	if len(termClauses) == 0 {
		return nil, nil
	}
	conditions = append(conditions, "("+strings.Join(termClauses, " OR ")+")")
	appendArtifactFilterClauses(&joins, &conditions, &args, fp, artifactFilterConfig{SourceJoined: true, SourceAlias: "src"})
	if len(joins) > 0 {
		query += " " + strings.Join(joins, " ")
	}
	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY a.last_observed_at DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArtifactIDs(rows)
}

func scanArtifactIDs(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
