// Package repo detects git repository metadata without hard-depending on git.
package repo

import (
	"bufio"
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
		if fi, err := os.Stat(candidate); err == nil && (fi.IsDir() || fi.Mode().IsRegular()) {
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

// FileFirstCommitDate returns the author date (RFC3339) of the oldest commit
// that added path, following renames. Empty string if git is unavailable or
// the path has no history in the repo.
func FileFirstCommitDate(repoRoot, relPath string) string {
	if relPath == "" {
		return ""
	}
	cmd := exec.Command("git", "log", "--diff-filter=A", "--follow", "--format=%aI", "--", relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return ""
	}
	lines := strings.Split(raw, "\n")
	last := lines[len(lines)-1]
	if last == "" {
		return ""
	}
	return last
}

// FileFirstCommitDates returns FileFirstCommitDate-compatible add dates for
// multiple paths by walking repo history once and following simple renames
// backwards through name-status records. Paths that cannot be resolved are
// omitted so callers can fall back to the exact per-file --follow path.
func FileFirstCommitDates(repoRoot string, relPaths []string) map[string]string {
	result := map[string]string{}
	tracked := map[string][]string{}
	for _, rel := range relPaths {
		rel = strings.TrimSpace(filepath.ToSlash(rel))
		if rel == "" {
			continue
		}
		if _, ok := tracked[rel]; ok {
			continue
		}
		tracked[rel] = []string{rel}
	}
	if len(tracked) == 0 {
		return result
	}
	cmd := exec.Command("git", "-c", "core.quotePath=false", "log", "--find-renames", "--format=%x00%aI", "--name-status")
	cmd.Dir = repoRoot
	out, err := cmd.StdoutPipe()
	if err != nil {
		return result
	}
	if err := cmd.Start(); err != nil {
		return result
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	currentDate := ""
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.HasPrefix(line, "\x00") {
			currentDate = strings.TrimSpace(strings.TrimPrefix(line, "\x00"))
			continue
		}
		if currentDate == "" || line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		switch {
		case status == "A":
			path := filepath.ToSlash(fields[1])
			for _, target := range tracked[path] {
				result[target] = currentDate
			}
		case strings.HasPrefix(status, "R") && len(fields) >= 3:
			oldPath := filepath.ToSlash(fields[1])
			newPath := filepath.ToSlash(fields[2])
			targets := tracked[newPath]
			if len(targets) == 0 {
				continue
			}
			delete(tracked, newPath)
			tracked[oldPath] = appendUniqueStrings(tracked[oldPath], targets...)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil || waitErr != nil {
		return map[string]string{}
	}
	return result
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := map[string]bool{}
	for _, value := range dst {
		seen[value] = true
	}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		dst = append(dst, value)
	}
	return dst
}
