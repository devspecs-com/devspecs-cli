package store

import (
	"path/filepath"
	"testing"
)

func TestGitFacts_ReplaceRepoGitFactsIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_git", now))
	commits := []GitCommitInput{{
		RepoID:       "repo_git",
		SHA:          "abc123",
		Branch:       "main",
		AuthorName:   "Test User",
		AuthorEmail:  "test@example.com",
		Message:      "touch auth docs",
		CommittedAt:  now,
		FilesChanged: 2,
		HistoryShape: "single_commit",
	}}
	files := []GitCommitFileInput{
		{RepoID: "repo_git", CommitSHA: "abc123", FilePath: "docs/auth.md", ChangeType: "A"},
		{RepoID: "repo_git", CommitSHA: "abc123", FilePath: "docs/auth-tests.md", ChangeType: "A"},
	}
	for i := 0; i < 2; i++ {
		if err := db.ReplaceRepoGitFacts("repo_git", commits, files, now); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := db.CountGitFacts("repo_git")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Commits != 1 || counts.Files != 2 {
		t.Fatalf("unexpected git fact counts: %#v", counts)
	}
}

func TestGitFacts_DeleteRepoGitFacts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_git", now))
	mustNoErr(t, db.ReplaceRepoGitFacts("repo_git",
		[]GitCommitInput{{RepoID: "repo_git", SHA: "abc123", CommittedAt: now}},
		[]GitCommitFileInput{{RepoID: "repo_git", CommitSHA: "abc123", FilePath: "docs/auth.md"}},
		now,
	))
	if err := db.DeleteRepoGitFacts("repo_git"); err != nil {
		t.Fatal(err)
	}
	counts, err := db.CountGitFacts("repo_git")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Commits != 0 || counts.Files != 0 {
		t.Fatalf("expected git facts deleted, got %#v", counts)
	}
}
