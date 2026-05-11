// Package discover performs bounded repository layout detection for ds init.
package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

// Limits keep discovery bounded on large trees.
const (
	MaxWalkDirs  = 4096
	MaxDocsFiles = 400
)

// Result holds paths to merge into config (high confidence) and suggestion lines.
type Result struct {
	MergeMarkdown []string
	MergeADR      []string
	Suggestions   []string
	// SkippedIgnored counts candidates skipped due to the ignore stack (approximate during walks).
	SkippedIgnored int
}

// Run detects layout under repoRoot using m for ignore rules. If m is nil, a matcher is loaded from repoRoot.
func Run(repoRoot string, m *ignore.Matcher) *Result {
	if m == nil {
		m, _ = ignore.NewMatcher(repoRoot)
	}
	out := &Result{}
	seenM := map[string]bool{}
	seenA := map[string]bool{}

	tryMarkdown := func(rel string) {
		if rel == "" || seenM[rel] {
			return
		}
		if m.ShouldSkip(rel, true) {
			out.SkippedIgnored++
			return
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return
		}
		seenM[rel] = true
		out.MergeMarkdown = append(out.MergeMarkdown, rel)
	}

	tryADR := func(rel string) {
		if rel == "" || seenA[rel] {
			return
		}
		if m.ShouldSkip(rel, true) {
			out.SkippedIgnored++
			return
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return
		}
		seenA[rel] = true
		out.MergeADR = append(out.MergeADR, rel)
	}

	for _, rel := range []string{
		".cursor/plans",
		"_bmad-output",
		".specify/memory",
		"plans",
		"docs/specs",
		"docs/plans",
		"docs/design",
		"docs/technical",
	} {
		tryMarkdown(rel)
	}

	if hasSpeckitSpec(repoRoot, m) {
		tryMarkdown("specs")
	}

	if plainDocsWorthIndexing(repoRoot, m, out) {
		tryMarkdown("docs")
	} else if dirExists(repoRoot, "docs") && !m.ShouldSkip("docs", true) {
		out.Suggestions = append(out.Suggestions,
			"docs/ looks sparse for specs/plans — add paths manually if needed (see README), e.g. edit .devspecs/config.yaml markdown paths.")
	}

	for _, rel := range []string{"docs/adr", "docs/adrs", "adr", "adrs", "architecture/decisions"} {
		tryADR(rel)
	}

	if !openspecChangesPresent(repoRoot, m) && dirExists(repoRoot, "openspec") && !m.ShouldSkip("openspec", true) {
		out.Suggestions = append(out.Suggestions,
			"openspec/ exists but no change proposals found under openspec/changes/*/proposal.md — confirm OpenSpec layout or adjust the openspec path in config.")
	}

	_ = MaxWalkDirs
	return out
}

func dirExists(repoRoot, rel string) bool {
	st, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel)))
	return err == nil && st.IsDir()
}

func hasSpeckitSpec(repoRoot string, m *ignore.Matcher) bool {
	base := filepath.Join(repoRoot, "specs")
	ents, err := os.ReadDir(base)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rel := "specs/" + e.Name() + "/spec.md"
		if m != nil && m.ShouldSkip(rel, false) {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}

func openspecChangesPresent(repoRoot string, m *ignore.Matcher) bool {
	if m != nil && (m.ShouldSkip("openspec", true) || m.ShouldSkip("openspec/changes", true)) {
		return false
	}
	dir := filepath.Join(repoRoot, "openspec", "changes")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		rel := "openspec/changes/" + e.Name() + "/proposal.md"
		if m != nil && m.ShouldSkip(rel, false) {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}

func plainDocsWorthIndexing(repoRoot string, m *ignore.Matcher, out *Result) bool {
	base := filepath.Join(repoRoot, "docs")
	st, err := os.Stat(base)
	if err != nil || !st.IsDir() {
		return false
	}
	if m != nil && m.ShouldSkip("docs", true) {
		out.SkippedIgnored++
		return false
	}

	hits := 0
	n := 0
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, ok := relPath(repoRoot, path)
		if !ok {
			return nil
		}
		if m != nil && m.ShouldSkip(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			out.SkippedIgnored++
			return nil
		}
		if d.IsDir() {
			return nil
		}
		n++
		if n > MaxDocsFiles {
			return filepath.SkipAll
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".spec.md") || strings.HasSuffix(name, ".plan.md") || strings.HasSuffix(name, ".prd.md") {
			hits++
		}
		return nil
	})
	return hits >= 2
}

func relPath(repoRoot, abs string) (string, bool) {
	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
