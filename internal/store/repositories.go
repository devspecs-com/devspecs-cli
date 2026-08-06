package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// RepositoryIdentity describes one physical root and its optional stable Git identity.
type RepositoryIdentity struct {
	RootPath      string
	RemoteURL     string
	RootCommit    string
	GitIdentity   string
	CurrentBranch string
}

// FindRepoID resolves a physical root first and then its stable Git identity.
func (db *DB) FindRepoID(rootPath, gitIdentity string) (string, error) {
	var id string
	err := db.QueryRow(`
		SELECT r.id
		FROM repos r
		LEFT JOIN repo_roots rr ON rr.repo_id = r.id
		WHERE rr.root_path = ? OR r.root_path = ?
		ORDER BY CASE WHEN rr.root_path = ? THEN 0 ELSE 1 END
		LIMIT 1`, rootPath, rootPath, rootPath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	if strings.TrimSpace(gitIdentity) == "" {
		return "", sql.ErrNoRows
	}
	return db.findRepoIDByGitIdentity(gitIdentity)
}

// ResolveRepo creates or refreshes one logical repository and records its physical root alias.
func (db *DB) ResolveRepo(identity RepositoryIdentity, newID, now string) (string, error) {
	rootPath := strings.TrimSpace(identity.RootPath)
	if rootPath == "" {
		return "", fmt.Errorf("repository root path is required")
	}
	var id string
	var err error
	if strings.TrimSpace(identity.GitIdentity) != "" {
		id, err = db.findRepoIDByGitIdentity(identity.GitIdentity)
	}
	if strings.TrimSpace(identity.GitIdentity) == "" || err == sql.ErrNoRows {
		id, err = db.FindRepoID(rootPath, "")
	}
	switch err {
	case nil:
	case sql.ErrNoRows:
		id = newID
		_, err = db.Exec(`
			INSERT INTO repos (
				id, root_path, git_remote_url, git_root_commit, git_identity,
				git_current_branch, created_at, updated_at
			) VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			id, rootPath, identity.RemoteURL, identity.RootCommit, identity.GitIdentity,
			identity.CurrentBranch, now, now)
		if err != nil {
			// Another process may have inserted the same Git identity after our
			// lookup. Rebind to it instead of creating a duplicate repository.
			if strings.TrimSpace(identity.GitIdentity) == "" {
				return "", err
			}
			var lookupErr error
			id, lookupErr = db.findRepoIDByGitIdentity(identity.GitIdentity)
			if lookupErr != nil {
				return "", err
			}
		}
	default:
		return "", err
	}

	if _, err := db.Exec(`
		UPDATE repos SET
			root_path = ?,
			git_remote_url = COALESCE(NULLIF(?, ''), git_remote_url),
			git_root_commit = COALESCE(NULLIF(?, ''), git_root_commit),
			git_identity = COALESCE(NULLIF(?, ''), git_identity),
			git_current_branch = COALESCE(NULLIF(?, ''), git_current_branch),
			updated_at = ?
		WHERE id = ?`,
		rootPath, identity.RemoteURL, identity.RootCommit, identity.GitIdentity,
		identity.CurrentBranch, now, id); err != nil {
		return "", err
	}
	_, err = db.Exec(`
		INSERT INTO repo_roots (root_path, repo_id, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(root_path) DO UPDATE SET repo_id = excluded.repo_id, last_seen_at = excluded.last_seen_at`,
		rootPath, id, now, now)
	return id, err
}

func (db *DB) findRepoIDByGitIdentity(gitIdentity string) (string, error) {
	var id string
	err := db.QueryRow(`
		SELECT id FROM repos
		WHERE git_identity = ?
		ORDER BY COALESCE(last_scan_at, updated_at) DESC, id
		LIMIT 1`, gitIdentity).Scan(&id)
	return id, err
}

// RepoHasArtifacts reports whether a logical repository already has indexed artifacts.
func (db *DB) RepoHasArtifacts(repoID string) (bool, error) {
	var one int
	err := db.QueryRow("SELECT 1 FROM artifacts WHERE repo_id = ? LIMIT 1", repoID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// RepoRootCondition returns an alias-aware SQL predicate for a repos table alias.
func RepoRootCondition(repoAlias string) string {
	if strings.TrimSpace(repoAlias) == "" {
		repoAlias = "r"
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM repo_roots rr_filter WHERE rr_filter.repo_id = %s.id AND rr_filter.root_path = ?)", repoAlias)
}
