package commands

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const (
	findGitReceiptMaxPaths   = 8
	findGitReceiptMaxCommits = 80
	findGitReceiptMaxDisplay = 3
	findGitReceiptTimeout    = 2 * time.Second
	findGitCommitMarker      = "__DEV_SPECS_GIT_RECEIPT_COMMIT__"
	findGitBodyMarker        = "__DEV_SPECS_GIT_RECEIPT_BODY__"
	findGitFilesMarker       = "__DEV_SPECS_GIT_RECEIPT_FILES__"
)

var findGitWorkRefPattern = regexp.MustCompile(`(?i)(merge pull request\s+#\d+|\(\s*#\d+\s*\)|\b(?:fixes|closes|refs)\s+#\d+\b|\b[A-Z][A-Z0-9]+-\d+\b|\bPR\s*#?\d+\b|\bGH-\d+\b)`)

type FindGitTrustContext struct {
	Mode        string           `json:"mode"`
	PathCount   int              `json:"path_count"`
	CommitsRead int              `json:"commits_read"`
	Receipts    []FindGitReceipt `json:"receipts,omitempty"`
}

type FindGitReceipt struct {
	SHA          string   `json:"sha"`
	ShortSHA     string   `json:"short_sha"`
	CommittedAt  string   `json:"committed_at,omitempty"`
	Subject      string   `json:"subject"`
	Detail       string   `json:"detail,omitempty"`
	MatchedPaths []string `json:"matched_paths,omitempty"`
	MatchedTerms []string `json:"matched_terms,omitempty"`
	Signals      []string `json:"signals,omitempty"`
	Score        int      `json:"score,omitempty"`
}

type parsedFindGitCommit struct {
	sha         string
	committedAt string
	subject     string
	body        string
	paths       []string
	order       int
}

func buildFindGitTrustContext(ctx context.Context, repoRoot, query string, rolePack retrieval.RoleGroupedPack) *FindGitTrustContext {
	paths := packGitReceiptPaths(rolePack)
	if repoRoot == "" || len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(ctx, repoRoot) {
		return nil
	}
	commits, ok := findGitLogForPaths(ctx, repoRoot, paths)
	if !ok || len(commits) == 0 {
		return nil
	}
	receipts := scoreFindGitReceipts(commits, paths, query)
	if len(receipts) == 0 {
		return nil
	}
	return &FindGitTrustContext{
		Mode:        "bounded_git_path_receipts_v0",
		PathCount:   len(paths),
		CommitsRead: len(commits),
		Receipts:    receipts,
	}
}

func packGitReceiptPaths(rolePack retrieval.RoleGroupedPack) []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range rolePack.Groups {
		for _, item := range group.Items {
			if len(out) >= findGitReceiptMaxPaths {
				return out
			}
			path := normalizeFindGitReceiptPath(item.Path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

func normalizeFindGitReceiptPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "#L"); idx >= 0 {
		path = path[:idx]
	}
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.Trim(path, "/")
	if path == "." || path == "" {
		return ""
	}
	return path
}

func findGitRepoAvailable(ctx context.Context, repoRoot string) bool {
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Clean(repoRoot), "rev-parse", "--is-inside-work-tree").CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func findGitLogForPaths(ctx context.Context, repoRoot string, paths []string) ([]parsedFindGitCommit, bool) {
	args := []string{
		"-C", filepath.Clean(repoRoot),
		"log",
		"--date=short",
		"--pretty=format:" + findGitCommitMarker + "%x1f%H%x1f%ad%x1f%s%n" + findGitBodyMarker + "%n%b%n" + findGitFilesMarker,
		"--name-only",
		"-n", fmt.Sprintf("%d", findGitReceiptMaxCommits),
		"--",
	}
	args = append(args, paths...)
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return nil, false
	}
	return parseFindGitLog(string(out)), true
}

func findGitLogRecent(ctx context.Context, repoRoot string, limit int) ([]parsedFindGitCommit, bool) {
	if limit <= 0 {
		limit = findGitReceiptMaxDisplay
	}
	args := []string{
		"-C", filepath.Clean(repoRoot),
		"log",
		"--date=short",
		"--pretty=format:" + findGitCommitMarker + "%x1f%H%x1f%ad%x1f%s%n" + findGitBodyMarker + "%n%b%n" + findGitFilesMarker,
		"--name-only",
		"-n", fmt.Sprintf("%d", limit),
	}
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return nil, false
	}
	return parseFindGitLog(string(out)), true
}

func parseFindGitLog(raw string) []parsedFindGitCommit {
	var commits []parsedFindGitCommit
	var current *parsedFindGitCommit
	var bodyLines []string
	state := ""
	flush := func() {
		if current == nil {
			return
		}
		current.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
		commits = append(commits, *current)
		current = nil
		bodyLines = nil
		state = ""
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, findGitCommitMarker) {
			flush()
			fields := strings.SplitN(strings.TrimPrefix(line, findGitCommitMarker), "\x1f", 4)
			if len(fields) < 4 {
				continue
			}
			current = &parsedFindGitCommit{
				sha:         fields[1],
				committedAt: fields[2],
				subject:     strings.TrimSpace(fields[3]),
				order:       len(commits),
			}
			state = "commit"
			continue
		}
		if current == nil {
			continue
		}
		switch line {
		case findGitBodyMarker:
			state = "body"
			continue
		case findGitFilesMarker:
			state = "files"
			continue
		}
		if state == "body" {
			bodyLines = append(bodyLines, line)
			continue
		}
		if state == "files" {
			path := normalizeFindGitReceiptPath(line)
			if path != "" {
				current.paths = appendUniqueString(current.paths, path)
			}
		}
	}
	flush()
	return commits
}

func scoreFindGitReceipts(commits []parsedFindGitCommit, packedPaths []string, query string) []FindGitReceipt {
	pathSet := map[string]bool{}
	for _, path := range packedPaths {
		pathSet[path] = true
	}
	anchors := findGitReceiptQueryAnchors(query)
	specificAnchors := findGitReceiptSpecificAnchors(query)
	var scored []FindGitReceipt
	for _, commit := range commits {
		if findGitReceiptCommitNoisy(commit) {
			continue
		}
		matchedPaths := commitMatchedPackPaths(commit.paths, pathSet)
		if len(matchedPaths) == 0 {
			continue
		}
		combined := strings.ToLower(commit.subject + "\n" + commit.body)
		var matchedTerms []string
		for _, anchor := range anchors {
			if strings.Contains(combined, anchor) {
				matchedTerms = appendUniqueString(matchedTerms, anchor)
			}
		}
		for _, anchor := range specificAnchors {
			for _, path := range commit.paths {
				if strings.Contains(strings.ToLower(filepath.ToSlash(path)), anchor) {
					matchedTerms = appendUniqueString(matchedTerms, anchor)
					break
				}
			}
		}
		if len(specificAnchors) > 0 && len(matchedTerms) == 0 && findGitReceiptPathOnlyNoise(commit, matchedPaths) {
			continue
		}
		signals := []string{fmt.Sprintf("touched %d packed path(s)", len(matchedPaths))}
		score := 20 + len(matchedPaths)*5
		if len(matchedTerms) > 0 {
			score += len(matchedTerms) * 4
			signals = append(signals, "matched query anchors")
		}
		if findGitWorkRefPattern.MatchString(commit.subject + "\n" + commit.body) {
			score += 3
			signals = append(signals, "PR/issue reference")
		}
		if len(matchedPaths) > 1 {
			score += 2
			signals = append(signals, "cross-file change")
		}
		if commit.order < 5 {
			score += 5 - commit.order
		}
		scored = append(scored, FindGitReceipt{
			SHA:          commit.sha,
			ShortSHA:     shortFindGitSHA(commit.sha),
			CommittedAt:  commit.committedAt,
			Subject:      limitRunes(commit.subject, 120),
			Detail:       limitRunes(firstNonEmptyGitBodyLine(commit.body), 120),
			MatchedPaths: firstStrings(matchedPaths, 3),
			MatchedTerms: firstStrings(matchedTerms, 5),
			Signals:      signals,
			Score:        score,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].CommittedAt > scored[j].CommittedAt
	})
	if len(scored) > findGitReceiptMaxDisplay {
		scored = scored[:findGitReceiptMaxDisplay]
	}
	return scored
}

func findGitReceiptCommitNoisy(commit parsedFindGitCommit) bool {
	subject := strings.ToLower(strings.TrimSpace(commit.subject))
	combined := subject + "\n" + strings.ToLower(commit.body)
	switch {
	case strings.Contains(combined, "dependabot") ||
		strings.Contains(combined, "renovate[bot]") ||
		strings.Contains(combined, "github-actions[bot]"):
		return true
	case strings.HasPrefix(subject, "update dependency ") ||
		strings.HasPrefix(subject, "bump ") ||
		strings.Contains(subject, "lockfile"):
		return true
	case strings.Contains(subject, "update translations") ||
		strings.Contains(subject, "translations for "):
		return true
	default:
		return false
	}
}

func findGitReceiptQueryAnchors(query string) []string {
	profile := retrieval.BuildAnchorProfile(query)
	var out []string
	for _, anchor := range profile.Anchors {
		switch anchor.Kind {
		case retrieval.AnchorGenericTaskWord, retrieval.AnchorArtifactRoleTerm:
			continue
		default:
			out = appendUniqueString(out, strings.ToLower(anchor.Term))
		}
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func findGitReceiptSpecificAnchors(query string) []string {
	generic := map[string]bool{
		"engine": true, "format": true, "formatting": true, "header": true, "headers": true,
		"helper": true, "helpers": true, "parser": true, "parsers": true, "regex": true,
		"release": true, "source": true, "test": true, "tests": true,
	}
	var out []string
	for _, token := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r == '_' || r == '-' || r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		token = strings.Trim(token, "_-.")
		if token == "" || generic[token] {
			continue
		}
		if len(token) <= 6 || strings.ContainsAny(token, "0123456789_-.") {
			out = appendUniqueString(out, token)
		}
	}
	return out
}

func findGitReceiptPathOnlyNoise(commit parsedFindGitCommit, matchedPaths []string) bool {
	subject := strings.ToLower(strings.TrimSpace(commit.subject))
	if len(matchedPaths) > 3 {
		return true
	}
	return strings.Contains(subject, "prettier") ||
		strings.Contains(subject, "format") ||
		strings.Contains(subject, "rustfmt") ||
		strings.Contains(subject, "migration guide") ||
		strings.Contains(subject, "style:") ||
		strings.Contains(subject, "changelog")
}

func commitMatchedPackPaths(paths []string, packed map[string]bool) []string {
	var out []string
	for _, path := range paths {
		if packed[path] {
			out = appendUniqueString(out, path)
		}
	}
	return out
}

func shortFindGitSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

func firstNonEmptyGitBodyLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != findGitFilesMarker {
			return line
		}
	}
	return ""
}
