package indexquery

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestLoadSourceManifestCandidatesForQueryMaterializesTestCandidate(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_src"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRepoSourceManifest(repoID,
		[]store.SourceManifestFileInput{{
			FileID: "src_1", RepoID: repoID, Path: "tests/session_test.go", ContentHash: "abc",
			Language: "go", SourceRoot: "tests", SourceRootKind: "test_root", SourceRole: "test", FirstPartyScore: 0.8,
		}},
		[]store.SourceManifestSymbolInput{{FileID: "src_1", Symbol: "TestRefreshSession", Kind: "test"}},
		[]store.SourceManifestTestInput{{FileID: "src_1", TestName: "TestRefreshSession"}},
		[]store.SourceManifestImportInput{},
		[]store.SourceManifestFTSInput{{
			FileID: "src_1", Path: "tests/session_test.go", PathTerms: "tests session test go",
			SourceRoot: "tests", Language: "go", SourceRole: "test",
			Symbols: "TestRefreshSession", TestNames: "TestRefreshSession",
		}},
		now); err != nil {
		t.Fatal(err)
	}

	candidates, report, err := LoadSourceManifestCandidatesForQuery(
		db,
		store.FilterParams{RepoRoot: tmp},
		"what tests cover refresh session",
		SourceManifestCandidateOptions{Mode: SourceManifestCandidateModeMetadata, Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.SelectedCount != 1 || len(candidates) != 1 {
		t.Fatalf("unexpected report/candidates: %#v len=%d", report, len(candidates))
	}
	got := candidates[0]
	if got.Kind != "source_context" || got.Subtype != "test_case" {
		t.Fatalf("unexpected candidate role fields: %#v", got)
	}
	if got.Metadata["retrieval_candidate"] != "source_manifest" || got.Metadata["test_name"] != "TestRefreshSession" {
		t.Fatalf("unexpected metadata: %#v", got.Metadata)
	}
	if !strings.Contains(got.Body, "Test names:") || !strings.Contains(got.Body, "TestRefreshSession") {
		t.Fatalf("body missing manifest evidence:\n%s", got.Body)
	}
}
