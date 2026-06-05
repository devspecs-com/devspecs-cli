package store

import "fmt"

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
