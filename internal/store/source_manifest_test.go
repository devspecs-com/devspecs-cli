package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceManifest_ReplaceRepoSourceManifestIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seedSourceManifestRepo(t, db, now)

	files := []SourceManifestFileInput{{
		FileID:          "src_1",
		RepoID:          "repo_src",
		Path:            "internal/auth/session.go",
		ContentHash:     "abc",
		SizeBytes:       123,
		Language:        "go",
		SourceRoot:      "internal",
		SourceRootKind:  "common_root",
		SourceRole:      "implementation",
		FirstPartyScore: 0.82,
	}}
	symbols := []SourceManifestSymbolInput{{FileID: "src_1", Symbol: "Session", Kind: "symbol"}}
	tests := []SourceManifestTestInput{{FileID: "src_1", TestName: "TestSession"}}
	imports := []SourceManifestImportInput{{FileID: "src_1", ImportRef: "context"}}
	fts := []SourceManifestFTSInput{{
		FileID:     "src_1",
		Path:       "internal/auth/session.go",
		PathTerms:  "internal auth session go",
		SourceRoot: "internal",
		Language:   "go",
		SourceRole: "implementation",
		Symbols:    "Session",
		TestNames:  "TestSession",
		Imports:    "context",
	}}
	seedSourceManifestRows(t, db, now)

	err := db.ReplaceRepoSourceManifest("repo_src", files, symbols, tests, imports, fts, now)
	require.NoError(t, err)

	counts, err := db.CountSourceManifest("repo_src")
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Files)
	assert.Equal(t, 1, counts.Symbols)
	assert.Equal(t, 1, counts.Tests)
	assert.Equal(t, 1, counts.Imports)
	assert.Equal(t, 1, counts.FTSRows)
}

func TestSourceManifest_ReplaceRepoSourceManifestBatchesLargeInputs(t *testing.T) {
	db := openTestDB(t)
	now := "2026-01-01T00:00:00Z"
	seedSourceManifestRepo(t, db, now)

	var files []SourceManifestFileInput
	var symbols []SourceManifestSymbolInput
	var tests []SourceManifestTestInput
	var imports []SourceManifestImportInput
	var fts []SourceManifestFTSInput
	for i := 0; i < sourceManifestInsertChunkSize+25; i++ {
		fileID := fmt.Sprintf("src_%03d", i)
		path := fmt.Sprintf("src/module/file_%03d.go", i)
		files = append(files, SourceManifestFileInput{
			FileID:          fileID,
			RepoID:          "repo_src",
			Path:            path,
			ContentHash:     fmt.Sprintf("hash_%03d", i),
			Language:        "go",
			SourceRoot:      "src",
			SourceRootKind:  "common_root",
			SourceRole:      "implementation",
			FirstPartyScore: 1,
		})
		symbols = append(symbols, SourceManifestSymbolInput{FileID: fileID, Symbol: fmt.Sprintf("Symbol%d", i), Kind: "symbol"})
		tests = append(tests, SourceManifestTestInput{FileID: fileID, TestName: fmt.Sprintf("TestSymbol%d", i)})
		imports = append(imports, SourceManifestImportInput{FileID: fileID, ImportRef: "context"})
		fts = append(fts, SourceManifestFTSInput{
			FileID:     fileID,
			Path:       path,
			PathTerms:  "src module file go",
			SourceRoot: "src",
			Language:   "go",
			SourceRole: "implementation",
			Symbols:    fmt.Sprintf("Symbol%d", i),
			TestNames:  fmt.Sprintf("TestSymbol%d", i),
			Imports:    "context",
		})
	}
	err := db.ReplaceRepoSourceManifest("repo_src", files, symbols, tests, imports, fts, now)
	require.NoError(t, err)

	counts, err := db.CountSourceManifest("repo_src")
	require.NoError(t, err)

	want := sourceManifestInsertChunkSize + 25
	assert.Equal(t, want, counts.Files)
	assert.Equal(t, want, counts.Symbols)
	assert.Equal(t, want, counts.Tests)
	assert.Equal(t, want, counts.Imports)
	assert.Equal(t, want, counts.FTSRows)
}

func TestSourceManifest_DeleteRepoSourceManifest(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seedSourceManifestRepo(t, db, now)
	require.NoError(t, db.ReplaceRepoSourceManifest("repo_src",
		[]SourceManifestFileInput{{FileID: "src_1", RepoID: "repo_src", Path: "src/app.ts", ContentHash: "abc", Language: "typescript", SourceRoot: "src", SourceRootKind: "common_root", SourceRole: "implementation"}},
		nil, nil, nil,
		[]SourceManifestFTSInput{{FileID: "src_1", Path: "src/app.ts", PathTerms: "src app ts", SourceRoot: "src", Language: "typescript", SourceRole: "implementation"}},
		now,
	))

	err := db.DeleteRepoSourceManifest("repo_src")
	require.NoError(t, err)

	counts, err := db.CountSourceManifest("repo_src")
	require.NoError(t, err)
	assert.Zero(t, counts.Files)
	assert.Zero(t, counts.Symbols)
	assert.Zero(t, counts.Tests)
	assert.Zero(t, counts.Imports)
	assert.Zero(t, counts.FTSRows)
}

func TestSourceManifest_SearchSourceManifestFTS(t *testing.T) {
	tmp := t.TempDir()
	db, err := Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	_, err = db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now)
	require.NoError(t, err)
	require.NoError(t, db.ReplaceRepoSourceManifest("repo_src",
		[]SourceManifestFileInput{{
			FileID: "src_1", RepoID: "repo_src", Path: "src/auth/session.go", ContentHash: "abc",
			Language: "go", SourceRoot: "src/auth", SourceRootKind: "module_root", SourceRole: "implementation", FirstPartyScore: 0.9,
		}},
		[]SourceManifestSymbolInput{{FileID: "src_1", Symbol: "RefreshSession", Kind: "symbol"}},
		[]SourceManifestTestInput{},
		[]SourceManifestImportInput{{FileID: "src_1", ImportRef: "context"}},
		[]SourceManifestFTSInput{{
			FileID: "src_1", Path: "src/auth/session.go", PathTerms: "src auth session go",
			SourceRoot: "src/auth", Language: "go", SourceRole: "implementation",
			Symbols: "RefreshSession", Imports: "context",
		}},
		now))

	rows, err := db.SearchSourceManifestFTS(`"refresh" OR "session"`, FilterParams{RepoRoot: tmp}, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "src/auth/session.go", rows[0].Path)
	assert.Equal(t, "RefreshSession", rows[0].Symbols)
}

func seedSourceManifestRepo(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_src', '/tmp/repo-src', ?, ?)", now, now)
	require.NoError(t, err)
}

func seedSourceManifestRows(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO source_manifest (
		file_id, repo_id, path, content_hash, size_bytes, language, source_root,
		source_root_kind, source_role, first_party_score, ignored_reason, indexed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"src_1", "repo_src", "internal/auth/session.go", "old", 1, "go", "internal",
		"common_root", "implementation", 0.5, "", now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO source_manifest_symbols (file_id, symbol, kind, line) VALUES (?, ?, ?, ?)`, "src_1", "OldSession", "symbol", 1)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO source_manifest_tests (file_id, test_name, parent, line) VALUES (?, ?, ?, ?)`, "src_1", "TestOldSession", "", 1)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO source_manifest_imports (file_id, import_ref, line) VALUES (?, ?, ?)`, "src_1", "errors", 1)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO source_manifest_fts (
		file_id, path, path_terms, source_root, language, source_role, symbols, test_names, imports
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"src_1", "internal/auth/session.go", "internal auth session go", "internal", "go", "implementation", "OldSession", "TestOldSession", "errors")
	require.NoError(t, err)

}
