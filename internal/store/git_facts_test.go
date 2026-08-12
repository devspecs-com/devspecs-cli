package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFacts_ReplaceRepoGitFactsIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

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
		BodyPreview:  "Fixes #42",
		CommittedAt:  now,
		FilesChanged: 2,
		HistoryShape: "single_commit",
	}}
	files := []GitCommitFileInput{
		{RepoID: "repo_git", CommitSHA: "abc123", FilePath: "docs/auth.md", ChangeType: "A"},
		{RepoID: "repo_git", CommitSHA: "abc123", FilePath: "docs/auth-tests.md", ChangeType: "A"},
	}
	seedGitFactRows(t, db, now)

	err = db.ReplaceRepoGitFacts("repo_git", commits, files, now)
	require.NoError(t, err)

	counts, err := db.CountGitFacts("repo_git")
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Commits)
	assert.Equal(t, 2, counts.Files)

	var bodyPreview string
	require.NoError(t, db.QueryRow("SELECT body_preview FROM git_commits WHERE repo_id = ? AND sha = ?", "repo_git", "abc123").Scan(&bodyPreview))
	assert.Equal(t, "Fixes #42", bodyPreview)
}

func TestGitFacts_DeleteRepoGitFacts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-05-26T00:00:00Z"
	mustNoErr(t, insertEvidenceRepo(db, "repo_git", now))
	seedGitFactRows(t, db, now)

	err = db.DeleteRepoGitFacts("repo_git")
	require.NoError(t, err)

	counts, err := db.CountGitFacts("repo_git")
	require.NoError(t, err)
	assert.Zero(t, counts.Commits)
	assert.Zero(t, counts.Files)
}

func seedGitFactRows(t *testing.T, db *DB, now string) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO git_commits (
		repo_id, sha, branch, author_name, author_email, message, body_preview,
		committed_at, files_changed, is_merge, history_shape, indexed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"repo_git", "abc123", "main", "Test User", "test@example.com", "touch auth docs", "old preview",
		now, 2, 0, "single_commit", now)
	require.NoError(t, err)

	_, err = db.Exec(`INSERT INTO git_commit_files (repo_id, commit_sha, file_path, change_type, old_path, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "repo_git", "abc123", "docs/old-auth.md", "A", "", now)
	require.NoError(t, err)
}
