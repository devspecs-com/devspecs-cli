package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/gitfacts"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

const (
	recentSourceAdmissionReason = "recent_git_source_context"
	recentSourceMaxFileBytes    = 256 * 1024
	recentSourceMaxExtra        = 500
	recentSourceMinExtra        = 100
)

type recentSourceCandidate struct {
	path  string
	score int
}

func buildRecentGitSourceContextCandidates(ctx context.Context, repoRoot string, existing []adapters.Candidate, opts RunOptions) []adapters.Candidate {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	maxCommits := opts.GitMaxCommits
	if maxCommits <= 0 {
		maxCommits = gitfacts.DefaultMaxCommits
	}
	maxFiles := opts.GitMaxFilesPerCommit
	if maxFiles <= 0 {
		maxFiles = 40
	}
	facts, err := gitfacts.Collect(ctx, repoRoot, gitfacts.Options{MaxCommits: maxCommits, MaxFilesPerCommit: maxFiles})
	if err != nil || len(facts.Files) == 0 {
		return nil
	}
	existingPaths := map[string]bool{}
	for _, candidate := range existing {
		if rel := normalizeRecentSourceRel(candidate.RelPath); rel != "" {
			existingPaths[rel] = true
		}
	}
	filesByCommit := map[string][]string{}
	for _, file := range facts.Files {
		rel := normalizeRecentSourceRel(file.FilePath)
		if rel == "" {
			continue
		}
		filesByCommit[file.CommitSHA] = append(filesByCommit[file.CommitSHA], rel)
	}
	scores := map[string]int{}
	for _, paths := range filesByCommit {
		hasTest := false
		for _, path := range paths {
			if recentSourceLooksTestPath(path) {
				hasTest = true
				break
			}
		}
		increment := 1
		if hasTest {
			increment = 3
		}
		for _, path := range paths {
			if existingPaths[path] || !recentSourceEligiblePath(ctx, repoRoot, path) {
				continue
			}
			scores[path] += increment
		}
	}
	if len(scores) == 0 {
		return nil
	}
	selected := make([]recentSourceCandidate, 0, len(scores))
	for path, score := range scores {
		selected = append(selected, recentSourceCandidate{path: path, score: score})
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].score == selected[j].score {
			return selected[i].path < selected[j].path
		}
		return selected[i].score > selected[j].score
	})
	limit := len(existing)
	if limit < recentSourceMinExtra {
		limit = recentSourceMinExtra
	}
	if limit > recentSourceMaxExtra {
		limit = recentSourceMaxExtra
	}
	if len(selected) > limit {
		selected = selected[:limit]
	}
	out := make([]adapters.Candidate, 0, len(selected))
	for _, candidate := range selected {
		out = append(out, adapters.Candidate{
			PrimaryPath:    filepath.Join(repoRoot, filepath.FromSlash(candidate.path)),
			RelPath:        candidate.path,
			AdapterName:    sourceCompanionAdapterName,
			DiscoveryScore: float64(candidate.score) / 100,
			DiscoveryReasons: []string{
				recentSourceAdmissionReason,
			},
			Metadata: map[string]any{
				"admission_reason": recentSourceAdmissionReason,
				"recent_git_score": fmt.Sprintf("%d", candidate.score),
				"source_path":      candidate.path,
			},
		})
	}
	return out
}

func recentSourceEligiblePath(ctx context.Context, repoRoot, rel string) bool {
	rel = normalizeRecentSourceRel(rel)
	if rel == "" || !recentSourceLooksSourcePath(rel) || recentSourceLooksTestPath(rel) || recentSourceLooksNoisePath(rel) {
		return false
	}
	if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
		return false
	}
	info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	if err != nil || info.IsDir() || info.Size() > recentSourceMaxFileBytes {
		return false
	}
	return true
}

func normalizeRecentSourceRel(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return ""
	}
	return path
}

func recentSourceLooksSourcePath(path string) bool {
	switch strings.ToLower(filepath.Ext(filepath.ToSlash(path))) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rb", ".rs", ".php", ".java", ".kt", ".kts", ".cs", ".vue", ".svelte", ".c", ".cc", ".cpp", ".h", ".hpp", ".mjs", ".cjs", ".mts", ".cts", ".sql":
		return true
	default:
		return false
	}
}

func recentSourceLooksTestPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	if hasRecentSourcePathSegment(path, "test", "tests", "__tests__", "spec", "e2e", "fixtures", "fixture", "testdata") {
		return true
	}
	switch {
	case ext == ".go" && strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")):
		return true
	case ext == ".java" && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "it")):
		return true
	case (ext == ".kt" || ext == ".kts") && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "spec")):
		return true
	case ext == ".php" && strings.HasSuffix(name, "test"):
		return true
	default:
		return false
	}
}

func recentSourceLooksNoisePath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".min.js") ||
		strings.HasSuffix(base, ".map") ||
		strings.HasSuffix(base, ".snap") ||
		strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, "_generated.go") ||
		strings.Contains(base, "generated") {
		return true
	}
	return hasRecentSourcePathSegment(path,
		".git", ".devspecs", "node_modules", "vendor", "dist", "build",
		"coverage", "target", "tmp", "temp", ".next", ".nuxt", ".venv",
		"venv", "__pycache__", "__snapshots__", "snapshots",
	)
}

func hasRecentSourcePathSegment(path string, segments ...string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(path)), "/")
	for _, part := range parts {
		for _, segment := range segments {
			if part == segment {
				return true
			}
		}
	}
	return false
}
