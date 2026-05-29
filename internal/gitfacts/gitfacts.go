// Package gitfacts collects bounded local git history facts for diagnostic evidence.
package gitfacts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ShapeNonGit       = "non_git"
	ShapeUnavailable  = "unavailable"
	ShapeEmpty        = "empty"
	ShapeSingleCommit = "single_commit"
	ShapeShallow      = "shallow"
	ShapeFull         = "full"

	DefaultMaxCommits        = 200
	DefaultMaxFilesPerCommit = 32
	DefaultMaxBodyBytes      = 4096
	DefaultMaxBodyLines      = 40
)

// Options controls bounded git history collection.
type Options struct {
	MaxCommits        int
	MaxFilesPerCommit int
}

// Facts is the collected local git history slice.
type Facts struct {
	Commits     []Commit
	Files       []FileChange
	Diagnostics Diagnostics
}

// Commit is one local git commit fact.
type Commit struct {
	SHA          string
	Branch       string
	Parents      []string
	AuthorName   string
	AuthorEmail  string
	Message      string
	BodyPreview  string
	CommittedAt  string
	FilesChanged int
	IsMerge      bool
	HistoryShape string
}

// FileChange is one file changed by a commit.
type FileChange struct {
	CommitSHA  string
	FilePath   string
	ChangeType string
	OldPath    string
}

// Diagnostics summarizes collection and history shape.
type Diagnostics struct {
	Enabled             bool   `json:"enabled"`
	HistoryShape        string `json:"history_shape"`
	IsShallow           bool   `json:"is_shallow,omitempty"`
	Branch              string `json:"branch,omitempty"`
	MaxCommits          int    `json:"max_commits"`
	MaxFilesPerCommit   int    `json:"max_files_per_commit"`
	TotalCommits        int    `json:"total_commits,omitempty"`
	CommitsRead         int    `json:"commits_read,omitempty"`
	CommitsStored       int    `json:"commits_stored,omitempty"`
	FilesStored         int    `json:"files_stored,omitempty"`
	SkippedLargeCommits int    `json:"skipped_large_commits,omitempty"`
	BodiesTruncated     int    `json:"commit_bodies_truncated,omitempty"`
	GitError            string `json:"git_error,omitempty"`
}

// Collect gathers a bounded set of local git facts. Non-git or unavailable git
// states are reported as diagnostics rather than hard failures.
func Collect(ctx context.Context, repoRoot string, opts Options) (Facts, error) {
	opts = normalizeOptions(opts)
	out := Facts{Diagnostics: Diagnostics{
		Enabled:           true,
		HistoryShape:      ShapeUnavailable,
		MaxCommits:        opts.MaxCommits,
		MaxFilesPerCommit: opts.MaxFilesPerCommit,
	}}
	if repoRoot == "" {
		out.Diagnostics.GitError = "empty repo root"
		return out, nil
	}
	inside, err := gitOutput(ctx, repoRoot, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			out.Diagnostics.HistoryShape = ShapeUnavailable
			out.Diagnostics.GitError = "git executable not found"
			return out, nil
		}
		if gitErrorMeansNonGit(err) {
			out.Diagnostics.HistoryShape = ShapeNonGit
			return out, nil
		}
		out.Diagnostics.HistoryShape = ShapeUnavailable
		out.Diagnostics.GitError = shortGitError(err)
		return out, nil
	}
	if strings.TrimSpace(inside) != "true" {
		out.Diagnostics.HistoryShape = ShapeNonGit
		return out, nil
	}
	branch, _ := gitOutput(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	out.Diagnostics.Branch = strings.TrimSpace(branch)

	shallow, err := gitOutput(ctx, repoRoot, "rev-parse", "--is-shallow-repository")
	if err == nil && strings.TrimSpace(shallow) == "true" {
		out.Diagnostics.IsShallow = true
	}
	if _, err := gitOutput(ctx, repoRoot, "rev-parse", "--verify", "HEAD"); err != nil {
		out.Diagnostics.HistoryShape = ShapeEmpty
		return out, nil
	}
	if countRaw, err := gitOutput(ctx, repoRoot, "rev-list", "--count", "HEAD"); err == nil {
		out.Diagnostics.TotalCommits, _ = strconv.Atoi(strings.TrimSpace(countRaw))
	}
	out.Diagnostics.HistoryShape = historyShape(out.Diagnostics.TotalCommits, out.Diagnostics.IsShallow)

	logRaw, err := gitOutput(ctx, repoRoot,
		"log",
		"--date=iso-strict",
		"--pretty=format:__DEV_SPECS_COMMIT__%x1f%H%x1f%P%x1f%an%x1f%ae%x1f%aI%x1f%s%n__DEV_SPECS_BODY__%n%b%n__DEV_SPECS_FILES__",
		"--name-status",
		"-n", strconv.Itoa(opts.MaxCommits),
	)
	if err != nil {
		out.Diagnostics.GitError = shortGitError(err)
		return out, nil
	}
	out.Commits, out.Files, out.Diagnostics.SkippedLargeCommits, out.Diagnostics.BodiesTruncated = parseLog(logRaw, out.Diagnostics.Branch, out.Diagnostics.HistoryShape, opts.MaxFilesPerCommit)
	out.Diagnostics.CommitsRead = len(out.Commits)
	out.Diagnostics.CommitsStored = len(out.Commits)
	out.Diagnostics.FilesStored = len(out.Files)
	return out, nil
}

func normalizeOptions(opts Options) Options {
	if opts.MaxCommits <= 0 {
		opts.MaxCommits = DefaultMaxCommits
	}
	if opts.MaxFilesPerCommit <= 0 {
		opts.MaxFilesPerCommit = DefaultMaxFilesPerCommit
	}
	return opts
}

func historyShape(total int, shallow bool) string {
	switch {
	case total <= 0:
		return ShapeFull
	case total == 1:
		return ShapeSingleCommit
	case shallow:
		return ShapeShallow
	default:
		return ShapeFull
	}
}

func gitOutput(ctx context.Context, repoRoot string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", filepath.Clean(repoRoot)}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(b)))
	}
	return string(b), nil
}

func gitErrorMeansNonGit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not a git repository") ||
		strings.Contains(msg, "not a git work tree") ||
		strings.Contains(msg, "not inside a git")
}

func parseLog(raw, branch, shape string, maxFilesPerCommit int) ([]Commit, []FileChange, int, int) {
	var commits []Commit
	var files []FileChange
	var current *Commit
	var currentFiles []FileChange
	var bodyLines []string
	skippedLarge := 0
	bodiesTruncated := 0
	state := ""
	flush := func() {
		if current == nil {
			return
		}
		if len(bodyLines) > 0 {
			body, truncated := boundedCommitBody(bodyLines)
			current.BodyPreview = body
			if truncated {
				bodiesTruncated++
			}
		}
		current.FilesChanged = len(currentFiles)
		commits = append(commits, *current)
		if len(currentFiles) > maxFilesPerCommit {
			skippedLarge++
		} else {
			files = append(files, currentFiles...)
		}
		current = nil
		currentFiles = nil
		bodyLines = nil
		state = ""
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "__DEV_SPECS_COMMIT__") {
			flush()
			fields := strings.SplitN(strings.TrimPrefix(line, "__DEV_SPECS_COMMIT__"), "\x1f", 7)
			if len(fields) < 7 {
				continue
			}
			parents := strings.Fields(fields[2])
			current = &Commit{
				SHA:          fields[1],
				Branch:       branch,
				Parents:      parents,
				AuthorName:   fields[3],
				AuthorEmail:  fields[4],
				CommittedAt:  fields[5],
				Message:      fields[6],
				IsMerge:      len(parents) > 1,
				HistoryShape: shape,
			}
			state = "commit"
			continue
		}
		if current == nil {
			continue
		}
		switch line {
		case "__DEV_SPECS_BODY__":
			state = "body"
			continue
		case "__DEV_SPECS_FILES__":
			state = "files"
			continue
		}
		if state == "body" {
			bodyLines = append(bodyLines, line)
			continue
		}
		if state != "files" || strings.TrimSpace(line) == "" {
			continue
		}
		if change, ok := parseNameStatus(current.SHA, line); ok {
			currentFiles = append(currentFiles, change)
		}
	}
	flush()
	return commits, files, skippedLarge, bodiesTruncated
}

func boundedCommitBody(lines []string) (string, bool) {
	truncated := false
	if len(lines) > DefaultMaxBodyLines {
		lines = lines[:DefaultMaxBodyLines]
		truncated = true
	}
	body := strings.TrimSpace(strings.Join(lines, "\n"))
	if len(body) > DefaultMaxBodyBytes {
		body = body[:DefaultMaxBodyBytes]
		truncated = true
	}
	return body, truncated
}

func parseNameStatus(commitSHA, line string) (FileChange, bool) {
	fields := strings.Split(line, "\t")
	if len(fields) < 2 {
		return FileChange{}, false
	}
	status := strings.TrimSpace(fields[0])
	changeType := status
	if len(changeType) > 1 {
		changeType = changeType[:1]
	}
	change := FileChange{CommitSHA: commitSHA, ChangeType: changeType}
	switch changeType {
	case "R", "C":
		if len(fields) < 3 {
			return FileChange{}, false
		}
		change.OldPath = cleanGitPath(fields[1])
		change.FilePath = cleanGitPath(fields[2])
	default:
		change.FilePath = cleanGitPath(fields[1])
	}
	if change.FilePath == "" {
		return FileChange{}, false
	}
	return change, true
}

func cleanGitPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return path
}

func shortGitError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 160 {
		msg = msg[:157] + "..."
	}
	return msg
}
