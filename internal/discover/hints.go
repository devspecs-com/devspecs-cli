package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

// MaxScanHints is the maximum number of directory paths emitted for empty-scan recovery.
const MaxScanHints = 8

// maxHintsSpeckitDirs limits how many specs/* feature dirs are considered for hints.
const maxHintsSpeckitDirs = 5

// Tighter caps for the docs/ density probe inside ScanHintCandidates only (strict bounded work).
const maxHintsDocsWalkDirs = 256
const maxHintsDocsWalkFiles = 120

// HintCandidate is an on-disk path that may help recover from an empty configured scan.
type HintCandidate struct {
	RelPath    string // repo-relative, slash-separated, no trailing slash
	SourceType string // markdown | adr | openspec
}

// FormatSuggestCommand returns a concrete ds config add-source line for the candidate.
func FormatSuggestCommand(h HintCandidate) string {
	switch h.SourceType {
	case "openspec":
		return fmt.Sprintf("ds config add-source openspec %s", h.RelPath)
	case "adr":
		return fmt.Sprintf("ds config add-source adr %s", h.RelPath)
	default:
		return fmt.Sprintf("ds config add-source markdown %s", h.RelPath)
	}
}

// ScanHintCandidates returns bounded, stable-ordered directories that exist under repoRoot
// and are not ignored. Used when a configured scan finds zero artifacts.
func ScanHintCandidates(repoRoot string, m *ignore.Matcher) []HintCandidate {
	if m == nil {
		m, _ = ignore.NewMatcher(repoRoot)
	}
	var out []HintCandidate
	seen := map[string]bool{}

	add := func(rel, sourceType string) {
		if rel == "" || seen[rel] || len(out) >= MaxScanHints {
			return
		}
		if m.ShouldSkip(rel, true) {
			return
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
		st, err := os.Stat(abs)
		if err != nil || !st.IsDir() {
			return
		}
		seen[rel] = true
		out = append(out, HintCandidate{RelPath: rel, SourceType: sourceType})
	}

	if openspecChangesPresent(repoRoot, m) {
		add("openspec", "openspec")
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
		add(rel, "markdown")
		if len(out) >= MaxScanHints {
			return out
		}
	}

	base := filepath.Join(repoRoot, "specs")
	if ents, err := os.ReadDir(base); err == nil {
		n := 0
		for _, e := range ents {
			if len(out) >= MaxScanHints {
				break
			}
			if !e.IsDir() {
				continue
			}
			if n >= maxHintsSpeckitDirs {
				break
			}
			n++
			rel := "specs/" + e.Name()
			specRel := rel + "/spec.md"
			if m.ShouldSkip(specRel, false) {
				continue
			}
			if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(specRel))); err != nil {
				continue
			}
			add(rel, "markdown")
		}
	}

	if plainDocsWorthIndexing(repoRoot, m, &Result{}, maxHintsDocsWalkDirs, maxHintsDocsWalkFiles) {
		add("docs", "markdown")
	}

	for _, rel := range []string{"docs/adr", "docs/adrs", "adr", "adrs", "architecture/decisions"} {
		add(rel, "adr")
		if len(out) >= MaxScanHints {
			return out
		}
	}

	return out
}

// HintDisplayPath returns a slash path with trailing slash for directory display lines.
func HintDisplayPath(rel string) string {
	rel = filepath.ToSlash(rel)
	if rel == "" {
		return ""
	}
	if strings.HasSuffix(rel, "/") {
		return rel
	}
	return rel + "/"
}
