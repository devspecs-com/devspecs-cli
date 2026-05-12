package initflow

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

// DetectedPattern describes a filesystem-derived glob→kind proposal for one source directory.
type DetectedPattern struct {
	Label       string
	Match       string
	DefaultKind string
	FileCount   int
}

// DetectPatterns scans a markdown source directory (repo-relative) up to two levels
// and returns proposed glob→kind mappings based on common naming conventions.
// The default kind is inferred from the directory name (e.g. "decisions" → decision).
func DetectPatterns(repoRoot, sourcePath string) []DetectedPattern {
	sourcePath = filepath.ToSlash(strings.TrimSpace(sourcePath))
	base := filepath.Join(repoRoot, filepath.FromSlash(sourcePath))

	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		return nil
	}

	files := collectMDFiles(base)
	if len(files) == 0 {
		return nil
	}

	dirKind := inferDirKind(sourcePath)

	// Candidate globs in priority order. All use the directory-inferred kind.
	candidateGlobs := []string{
		"README.md",
		"[0-9][0-9][0-9]-*.md",
		"[0-9][0-9][0-9]_*.md",
		"[0-9][0-9]_*.md",
		"[0-9][0-9]-*.md",
		"*/README.md",
		"*/[0-9][0-9][0-9]-*.md",
		"*/[0-9][0-9]-*.md",
		"*/[0-9][0-9]_*.md",
	}

	var out []DetectedPattern
	matched := make(map[string]bool)

	for _, glob := range candidateGlobs {
		count := 0
		for _, f := range files {
			if patternMatch(glob, f) {
				count++
				matched[f] = true
			}
		}
		if count > 0 {
			out = append(out, DetectedPattern{
				Label:       glob,
				Match:       glob,
				DefaultKind: dirKind,
				FileCount:   count,
			})
		}
	}

	// Propose individual root-level .md files not matched by any pattern above.
	for _, f := range files {
		if matched[f] || strings.Contains(f, "/") {
			continue
		}
		kind := inferSingleFileKind(f)
		out = append(out, DetectedPattern{
			Label:       f,
			Match:       f,
			DefaultKind: kind,
			FileCount:   1,
		})
	}

	return out
}

// collectMDFiles returns .md file paths relative to base, up to two directory levels.
func collectMDFiles(base string) []string {
	var files []string
	ents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	for _, e := range ents {
		name := e.Name()
		if !e.IsDir() {
			if strings.HasSuffix(strings.ToLower(name), ".md") {
				files = append(files, name)
			}
			continue
		}
		sub := filepath.Join(base, name)
		subEnts, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, se := range subEnts {
			if se.IsDir() {
				continue
			}
			sn := se.Name()
			if strings.HasSuffix(strings.ToLower(sn), ".md") {
				files = append(files, name+"/"+sn)
			}
		}
	}
	return files
}

func patternMatch(pattern, path string) bool {
	ok, err := filepath.Match(pattern, path)
	return err == nil && ok
}

// inferDirKind guesses the dominant artifact kind from the directory basename.
func inferDirKind(dirPath string) string {
	lower := strings.ToLower(filepath.Base(filepath.ToSlash(dirPath)))
	switch {
	case strings.Contains(lower, "decision"), strings.Contains(lower, "adr"):
		return config.KindDecision
	case strings.Contains(lower, "spec"):
		return config.KindSpec
	case strings.Contains(lower, "design"):
		return config.KindDesign
	case strings.Contains(lower, "requirement"), strings.Contains(lower, "prd"):
		return config.KindRequirements
	default:
		return config.KindPlan
	}
}

// inferSingleFileKind guesses a kind from a standalone filename.
func inferSingleFileKind(name string) string {
	lower := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	switch {
	case strings.HasPrefix(lower, "roadmap"), strings.HasPrefix(lower, "plan"):
		return config.KindPlan
	case strings.HasPrefix(lower, "spec"):
		return config.KindSpec
	case strings.HasPrefix(lower, "design"):
		return config.KindDesign
	case strings.HasPrefix(lower, "decision"), strings.HasPrefix(lower, "adr"):
		return config.KindDecision
	case strings.HasPrefix(lower, "requirement"), strings.HasPrefix(lower, "prd"):
		return config.KindRequirements
	default:
		return config.KindMarkdownArtifact
	}
}
