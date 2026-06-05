package store

import (
	"fmt"
	"strings"
)

// SourceManifestFileInput is a compact first-party source/test file row.
type SourceManifestFileInput struct {
	FileID          string
	RepoID          string
	Path            string
	ContentHash     string
	SizeBytes       int64
	Language        string
	SourceRoot      string
	SourceRootKind  string
	SourceRole      string
	FirstPartyScore float64
	IgnoredReason   string
}

// SourceManifestSymbolInput is one extracted symbol for a source manifest file.
type SourceManifestSymbolInput struct {
	FileID string
	Symbol string
	Kind   string
	Line   int
}

// SourceManifestTestInput is one extracted test name for a source manifest file.
type SourceManifestTestInput struct {
	FileID   string
	TestName string
	Parent   string
	Line     int
}

// SourceManifestImportInput is one import-ish reference for a source manifest file.
type SourceManifestImportInput struct {
	FileID    string
	ImportRef string
	Line      int
}

// SourceManifestFTSInput is one searchable metadata row for source manifest FTS.
type SourceManifestFTSInput struct {
	FileID     string
	Path       string
	PathTerms  string
	SourceRoot string
	Language   string
	SourceRole string
	Symbols    string
	TestNames  string
	Imports    string
}

// SourceManifestCounts summarizes compact manifest rows for diagnostics/tests.
type SourceManifestCounts struct {
	Files   int `json:"files"`
	Tests   int `json:"tests"`
	Symbols int `json:"symbols"`
	Imports int `json:"imports"`
	FTSRows int `json:"fts_rows"`
}

// SourceManifestSearchRow is a compact source/test row returned from manifest FTS.
type SourceManifestSearchRow struct {
	FileID          string
	RepoID          string
	Path            string
	ContentHash     string
	SizeBytes       int64
	Language        string
	SourceRoot      string
	SourceRootKind  string
	SourceRole      string
	FirstPartyScore float64
	IndexedAt       string
	Symbols         string
	TestNames       string
	Imports         string
	Rank            float64
}

// ReplaceRepoSourceManifest replaces all compact source manifest rows for a repo.
func (db *DB) ReplaceRepoSourceManifest(repoID string, files []SourceManifestFileInput, symbols []SourceManifestSymbolInput, tests []SourceManifestTestInput, imports []SourceManifestImportInput, ftsRows []SourceManifestFTSInput, now string) error {
	const savepoint = "source_manifest_replace"
	if _, err := db.Exec("SAVEPOINT " + savepoint); err != nil {
		return err
	}
	rollback := func(err error) error {
		_, _ = db.Exec("ROLLBACK TO SAVEPOINT " + savepoint)
		_, _ = db.Exec("RELEASE SAVEPOINT " + savepoint)
		return err
	}
	if _, err := db.Exec("DELETE FROM source_manifest_fts WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM source_manifest_symbols WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM source_manifest_tests WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM source_manifest_imports WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM source_manifest WHERE repo_id = ?", repoID); err != nil {
		return rollback(err)
	}

	fileStmt, err := db.Prepare(
		`INSERT INTO source_manifest
			(file_id, repo_id, path, content_hash, size_bytes, language, source_root, source_root_kind, source_role, first_party_score, ignored_reason, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer fileStmt.Close()
	for _, f := range files {
		if _, err := fileStmt.Exec(f.FileID, f.RepoID, f.Path, f.ContentHash, f.SizeBytes, f.Language, f.SourceRoot, f.SourceRootKind, f.SourceRole, f.FirstPartyScore, f.IgnoredReason, now); err != nil {
			return rollback(err)
		}
	}

	symbolStmt, err := db.Prepare(
		`INSERT INTO source_manifest_symbols (file_id, symbol, kind, line) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer symbolStmt.Close()
	for _, s := range symbols {
		if _, err := symbolStmt.Exec(s.FileID, s.Symbol, s.Kind, s.Line); err != nil {
			return rollback(err)
		}
	}

	testStmt, err := db.Prepare(
		`INSERT INTO source_manifest_tests (file_id, test_name, parent, line) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer testStmt.Close()
	for _, t := range tests {
		if _, err := testStmt.Exec(t.FileID, t.TestName, t.Parent, t.Line); err != nil {
			return rollback(err)
		}
	}

	importStmt, err := db.Prepare(
		`INSERT INTO source_manifest_imports (file_id, import_ref, line) VALUES (?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer importStmt.Close()
	for _, i := range imports {
		if _, err := importStmt.Exec(i.FileID, i.ImportRef, i.Line); err != nil {
			return rollback(err)
		}
	}

	ftsStmt, err := db.Prepare(
		`INSERT INTO source_manifest_fts
			(file_id, path, path_terms, source_root, language, source_role, symbols, test_names, imports)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer ftsStmt.Close()
	for _, row := range ftsRows {
		if _, err := ftsStmt.Exec(row.FileID, row.Path, row.PathTerms, row.SourceRoot, row.Language, row.SourceRole, row.Symbols, row.TestNames, row.Imports); err != nil {
			return rollback(err)
		}
	}

	if _, err := db.Exec("RELEASE SAVEPOINT " + savepoint); err != nil {
		return rollback(err)
	}
	return nil
}

// DeleteRepoSourceManifest removes compact source manifest rows for a repo.
func (db *DB) DeleteRepoSourceManifest(repoID string) error {
	if _, err := db.Exec("DELETE FROM source_manifest_fts WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM source_manifest_symbols WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM source_manifest_tests WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM source_manifest_imports WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)", repoID); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM source_manifest WHERE repo_id = ?", repoID)
	return err
}

// CountSourceManifest returns compact manifest row counts for a repo.
func (db *DB) CountSourceManifest(repoID string) (SourceManifestCounts, error) {
	var out SourceManifestCounts
	if err := db.QueryRow("SELECT COUNT(*) FROM source_manifest WHERE repo_id = ?", repoID).Scan(&out.Files); err != nil {
		return out, fmt.Errorf("count source_manifest: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM source_manifest_tests smt
		JOIN source_manifest sm ON sm.file_id = smt.file_id
		WHERE sm.repo_id = ?`, repoID).Scan(&out.Tests); err != nil {
		return out, fmt.Errorf("count source_manifest_tests: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM source_manifest_symbols sms
		JOIN source_manifest sm ON sm.file_id = sms.file_id
		WHERE sm.repo_id = ?`, repoID).Scan(&out.Symbols); err != nil {
		return out, fmt.Errorf("count source_manifest_symbols: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM source_manifest_imports smi
		JOIN source_manifest sm ON sm.file_id = smi.file_id
		WHERE sm.repo_id = ?`, repoID).Scan(&out.Imports); err != nil {
		return out, fmt.Errorf("count source_manifest_imports: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM source_manifest_fts
		WHERE file_id IN (SELECT file_id FROM source_manifest WHERE repo_id = ?)`, repoID).Scan(&out.FTSRows); err != nil {
		return out, fmt.Errorf("count source_manifest_fts: %w", err)
	}
	return out, nil
}

// SearchSourceManifestFTS returns compact source/test rows matching manifest FTS.
func (db *DB) SearchSourceManifestFTS(match string, fp FilterParams, limit int) ([]SourceManifestSearchRow, error) {
	if strings.TrimSpace(match) == "" || limit <= 0 {
		return nil, nil
	}
	query := `SELECT sm.file_id, sm.repo_id, sm.path, sm.content_hash, sm.size_bytes,
			sm.language, sm.source_root, sm.source_root_kind, sm.source_role,
			sm.first_party_score, sm.indexed_at,
			source_manifest_fts.symbols, source_manifest_fts.test_names, source_manifest_fts.imports,
			bm25(source_manifest_fts) AS rank
		FROM source_manifest_fts
		JOIN source_manifest sm ON sm.file_id = source_manifest_fts.file_id`
	var conditions []string
	args := []any{match}
	if fp.RepoRoot != "" || fp.Branch != "" || fp.User != "" {
		query += " JOIN repos r ON r.id = sm.repo_id"
	}
	conditions = append(conditions, "source_manifest_fts MATCH ?")
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
	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY rank, sm.first_party_score DESC, sm.path LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search source manifest fts: %w", err)
	}
	defer rows.Close()
	var out []SourceManifestSearchRow
	for rows.Next() {
		var row SourceManifestSearchRow
		if err := rows.Scan(
			&row.FileID,
			&row.RepoID,
			&row.Path,
			&row.ContentHash,
			&row.SizeBytes,
			&row.Language,
			&row.SourceRoot,
			&row.SourceRootKind,
			&row.SourceRole,
			&row.FirstPartyScore,
			&row.IndexedAt,
			&row.Symbols,
			&row.TestNames,
			&row.Imports,
			&row.Rank,
		); err != nil {
			return nil, fmt.Errorf("scan source manifest row: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source manifest rows: %w", err)
	}
	return out, nil
}
