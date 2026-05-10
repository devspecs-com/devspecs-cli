// Package repo detects git repository metadata without hard-depending on git.
package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Info holds detected repository metadata.
type Info struct {
	RootPath      string
	RemoteURL     string
	CurrentBranch string
	IsGit         bool
}

// Detect attempts to discover git repository info from the given directory.
// Returns non-git Info if not inside a git repo (no error).
func Detect(dir string) Info {
	info := Info{RootPath: dir}

	gitDir := findGitDir(dir)
	if gitDir == "" {
		return info
	}
	info.IsGit = true
	info.RootPath = filepath.Dir(gitDir)

	if remote := gitConfig(info.RootPath, "remote.origin.url"); remote != "" {
		info.RemoteURL = remote
	}

	if branch := gitSymbolicRef(info.RootPath); branch != "" {
		info.CurrentBranch = branch
	}

	return info
}

func findGitDir(dir string) string {
	for {
		candidate := filepath.Join(dir, ".git")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func gitConfig(repoRoot, key string) string {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitSymbolicRef(repoRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// HeadCommit returns the current HEAD commit SHA, or "" if not in a git repo.
func HeadCommit(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ChangedFiles returns the list of files changed in the most recent commit.
// Uses diff-tree which works on initial commits and merge commits correctly.
func ChangedFiles(repoRoot string) []string {
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}
