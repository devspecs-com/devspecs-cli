package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSourceManifest_ReplaceRepoSourceManifestIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_src', '/tmp/repo-src', ?, ?)", now, now); err != nil {
		t.Fatal(err)
	}

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

	for i := 0; i < 2; i++ {
		if err := db.ReplaceRepoSourceManifest("repo_src", files, symbols, tests, imports, fts, now); err != nil {
			t.Fatal(err)
		}
		counts, err := db.CountSourceManifest("repo_src")
		if err != nil {
			t.Fatal(err)
		}
		if counts.Files != 1 || counts.Symbols != 1 || counts.Tests != 1 || counts.Imports != 1 || counts.FTSRows != 1 {
			t.Fatalf("unexpected counts after replace %d: %#v", i, counts)
		}
	}
}

func TestSourceManifest_DeleteRepoSourceManifest(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_src', '/tmp/repo-src', ?, ?)", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRepoSourceManifest("repo_src",
		[]SourceManifestFileInput{{FileID: "src_1", RepoID: "repo_src", Path: "src/app.ts", ContentHash: "abc", Language: "typescript", SourceRoot: "src", SourceRootKind: "common_root", SourceRole: "implementation"}},
		nil, nil, nil,
		[]SourceManifestFTSInput{{FileID: "src_1", Path: "src/app.ts", PathTerms: "src app ts", SourceRoot: "src", Language: "typescript", SourceRole: "implementation"}},
		now,
	); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRepoSourceManifest("repo_src"); err != nil {
		t.Fatal(err)
	}
	counts, err := db.CountSourceManifest("repo_src")
	if err != nil {
		t.Fatal(err)
	}
	if counts != (SourceManifestCounts{}) {
		t.Fatalf("expected empty manifest counts after delete, got %#v", counts)
	}
}

func TestSourceManifest_SearchSourceManifestFTS(t *testing.T) {
	tmp := t.TempDir()
	db, err := Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRepoSourceManifest("repo_src",
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
		now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.SearchSourceManifestFTS(`"refresh" OR "session"`, FilterParams{RepoRoot: tmp}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Path != "src/auth/session.go" || rows[0].Symbols != "RefreshSession" {
		t.Fatalf("unexpected row: %#v", rows[0])
	}
}
