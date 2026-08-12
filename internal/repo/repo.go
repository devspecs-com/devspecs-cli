// Package repo detects git repository metadata without hard-depending on git.
package repo

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Info holds detected repository metadata.
type Info struct {
	RootPath      string
	RemoteURL     string
	RootCommit    string
	GitIdentity   string
	CurrentBranch string
	IsGit         bool
}

// DetectIdentity adds the stable remote-plus-root-commit identity used to
// recognize one logical repository across Git worktrees.
func DetectIdentity(dir string) Info {
	return DetectIdentityContext(context.Background(), dir)
}

// DetectIdentityContext is DetectIdentity with cancellation for Git metadata
// subprocesses.
func DetectIdentityContext(ctx context.Context, dir string) Info {
	return WithIdentityContext(ctx, DetectContext(ctx, dir))
}

// WithIdentity enriches already-detected Git metadata without repeating path,
// remote, and branch discovery.
func WithIdentity(info Info) Info {
	return WithIdentityContext(context.Background(), info)
}

// WithIdentityContext is WithIdentity with cancellation for root-history
// discovery.
func WithIdentityContext(ctx context.Context, info Info) Info {
	if !info.IsGit || strings.TrimSpace(info.RemoteURL) == "" {
		return info
	}
	info.RootCommit = RootCommitContext(ctx, info.RootPath)
	info.GitIdentity = StableGitIdentity(info.RemoteURL, info.RootCommit)
	return info
}

// RootCommit returns all root commits reachable from HEAD in stable order.
// Multiple roots are possible after merging unrelated histories.
func RootCommit(repoRoot string) string {
	return RootCommitContext(context.Background(), repoRoot)
}

// RootCommitContext is RootCommit with cancellation.
func RootCommitContext(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var roots []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if root := strings.TrimSpace(line); root != "" {
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)
	return strings.Join(roots, ",")
}

// StableGitIdentity combines a canonical remote with root history. Empty
// inputs stay path-scoped rather than risking an unrelated-repository match.
func StableGitIdentity(remoteURL, rootCommit string) string {
	remote := CanonicalRemoteURL(remoteURL)
	rootCommit = strings.TrimSpace(rootCommit)
	if remote == "" || rootCommit == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(remote + "\n" + rootCommit))
	return "git_" + hex.EncodeToString(sum[:])
}

// CanonicalRemoteURL normalizes common SSH and URL Git remote spellings.
func CanonicalRemoteURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if !strings.Contains(remote, "://") {
		at := strings.LastIndex(remote, "@")
		colon := strings.Index(remote, ":")
		if at >= 0 && colon > at {
			remote = "ssh://" + remote[:colon] + "/" + remote[colon+1:]
		}
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if port := parsed.Port(); port != "" && port != "22" && port != "443" {
		host += ":" + port
	}
	repoPath := strings.Trim(strings.ReplaceAll(parsed.Path, "\\", "/"), "/")
	repoPath = strings.TrimSuffix(repoPath, ".git")
	if repoPath == "" {
		return ""
	}
	return host + "/" + strings.ToLower(repoPath)
}

// Detect attempts to discover git repository info from the given directory.
// Returns non-git Info if not inside a git repo (no error).
func Detect(dir string) Info {
	return DetectContext(context.Background(), dir)
}

// DetectContext is Detect with cancellation for Git metadata subprocesses.
func DetectContext(ctx context.Context, dir string) Info {
	info := Info{RootPath: dir}

	gitDir := findGitDir(dir)
	if gitDir == "" {
		return info
	}
	info.IsGit = true
	info.RootPath = filepath.Dir(gitDir)

	if remote := gitConfigContext(ctx, info.RootPath, "remote.origin.url"); remote != "" {
		info.RemoteURL = remote
	}

	if branch := gitSymbolicRefContext(ctx, info.RootPath); branch != "" {
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

func gitConfigContext(ctx context.Context, repoRoot, key string) string {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitSymbolicRefContext(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// HeadCommit returns the current HEAD commit SHA, or "" if not in a git repo.
func HeadCommit(repoRoot string) string {
	return HeadCommitContext(context.Background(), repoRoot)
}

// HeadCommitContext is HeadCommit with cancellation.
func HeadCommitContext(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
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
	return ChangedFilesContext(context.Background(), repoRoot)
}

// ChangedFilesContext is ChangedFiles with cancellation.
func ChangedFilesContext(ctx context.Context, repoRoot string) []string {
	cmd := exec.CommandContext(ctx, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
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
	return FileFirstCommitDateContext(context.Background(), repoRoot, relPath)
}

// FileFirstCommitDateContext is FileFirstCommitDate with cancellation.
func FileFirstCommitDateContext(ctx context.Context, repoRoot, relPath string) string {
	if relPath == "" {
		return ""
	}
	cmd := exec.CommandContext(ctx, "git", "log", "--diff-filter=A", "--follow", "--format=%aI", "--", relPath)
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
	return FileFirstCommitDatesContext(context.Background(), repoRoot, relPaths)
}

// FileFirstCommitDatesContext is FileFirstCommitDates with cancellation for
// the potentially expensive full-history git traversal.
func FileFirstCommitDatesContext(ctx context.Context, repoRoot string, relPaths []string) map[string]string {
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
	cmd := exec.CommandContext(ctx, "git", "-c", "core.quotePath=false", "log", "--find-renames", "--format=%x00%aI", "--name-status")
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
