package store

import "fmt"

// GitCommitInput is a local git commit fact captured during an opt-in scan.
type GitCommitInput struct {
	RepoID       string
	SHA          string
	Branch       string
	AuthorName   string
	AuthorEmail  string
	Message      string
	BodyPreview  string
	CommittedAt  string
	FilesChanged int
	IsMerge      bool
	HistoryShape string
}

// GitCommitFileInput is one changed file entry for a captured git commit.
type GitCommitFileInput struct {
	RepoID     string
	CommitSHA  string
	FilePath   string
	ChangeType string
	OldPath    string
}

// GitFactCounts summarizes stored git fact rows for a repo.
type GitFactCounts struct {
	Commits int
	Files   int
}

// ArtifactSourcePathRow maps a stored artifact to a source path that can be matched to git file changes.
type ArtifactSourcePathRow struct {
	ArtifactID     string
	Kind           string
	Subtype        string
	Title          string
	Path           string
	SourceIdentity string
}

// ReplaceRepoGitFacts replaces all git fact rows for a repo.
func (db *DB) ReplaceRepoGitFacts(repoID string, commits []GitCommitInput, files []GitCommitFileInput, now string) error {
	const savepoint = "git_facts_replace"
	if _, err := db.Exec("SAVEPOINT " + savepoint); err != nil {
		return err
	}
	rollback := func(err error) error {
		_, _ = db.Exec("ROLLBACK TO SAVEPOINT " + savepoint)
		_, _ = db.Exec("RELEASE SAVEPOINT " + savepoint)
		return err
	}
	if _, err := db.Exec("DELETE FROM git_commit_files WHERE repo_id = ?", repoID); err != nil {
		return rollback(err)
	}
	if _, err := db.Exec("DELETE FROM git_commits WHERE repo_id = ?", repoID); err != nil {
		return rollback(err)
	}
	commitStmt, err := db.Prepare(
		`INSERT INTO git_commits
			(repo_id, sha, branch, author_name, author_email, message, body_preview, committed_at, files_changed, is_merge, history_shape, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer commitStmt.Close()
	for _, c := range commits {
		isMerge := 0
		if c.IsMerge {
			isMerge = 1
		}
		if _, err := commitStmt.Exec(c.RepoID, c.SHA, c.Branch, c.AuthorName, c.AuthorEmail, c.Message, c.BodyPreview, c.CommittedAt, c.FilesChanged, isMerge, c.HistoryShape, now); err != nil {
			return rollback(err)
		}
	}
	fileStmt, err := db.Prepare(
		`INSERT INTO git_commit_files
			(repo_id, commit_sha, file_path, change_type, old_path, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return rollback(err)
	}
	defer fileStmt.Close()
	for _, f := range files {
		if _, err := fileStmt.Exec(f.RepoID, f.CommitSHA, f.FilePath, f.ChangeType, f.OldPath, now); err != nil {
			return rollback(err)
		}
	}
	if _, err := db.Exec("RELEASE SAVEPOINT " + savepoint); err != nil {
		return rollback(err)
	}
	return nil
}

// DeleteRepoGitFacts removes git fact rows for a repo.
func (db *DB) DeleteRepoGitFacts(repoID string) error {
	if _, err := db.Exec("DELETE FROM git_commit_files WHERE repo_id = ?", repoID); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM git_commits WHERE repo_id = ?", repoID)
	return err
}

// CountGitFacts returns commit/file fact counts for a repo.
func (db *DB) CountGitFacts(repoID string) (GitFactCounts, error) {
	var out GitFactCounts
	if err := db.QueryRow("SELECT COUNT(*) FROM git_commits WHERE repo_id = ?", repoID).Scan(&out.Commits); err != nil {
		return out, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM git_commit_files WHERE repo_id = ?", repoID).Scan(&out.Files); err != nil {
		return out, err
	}
	return out, nil
}

// GetArtifactSourcePaths returns artifact source paths for git file-change mapping.
func (db *DB) GetArtifactSourcePaths(repoID string) ([]ArtifactSourcePathRow, error) {
	rows, err := db.Query(
		`SELECT DISTINCT a.id, a.kind, COALESCE(a.subtype,''), a.title, COALESCE(s.path,''), COALESCE(s.source_identity,'')
		 FROM artifacts a
		 JOIN sources s ON s.artifact_id = a.id
		 WHERE a.repo_id = ?
		 ORDER BY s.path, s.source_identity, a.id`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactSourcePathRow
	for rows.Next() {
		var r ArtifactSourcePathRow
		if err := rows.Scan(&r.ArtifactID, &r.Kind, &r.Subtype, &r.Title, &r.Path, &r.SourceIdentity); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("artifact source paths: %w", err)
	}
	return out, nil
}
