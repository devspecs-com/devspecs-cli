package initflow

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

// DetectedPattern describes a filesystem shape match relative to a markdown source directory.
type DetectedPattern struct {
	Label       string
	Match       string // glob relative to sourcePath
	DefaultKind string
	SkipRule    bool // if true, do not emit a config rule
}

// DetectPatterns scans one markdown source directory (repo-relative) up to two levels
// and suggests glob rules for common numbered-plan layouts.
func DetectPatterns(repoRoot, sourcePath string) []DetectedPattern {
	sourcePath = filepath.ToSlash(strings.TrimSpace(sourcePath))
	base := filepath.Join(repoRoot, filepath.FromSlash(sourcePath))
	var out []DetectedPattern

	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		return nil
	}

	ents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	var hasRootREADME, hasNumUnderscore, hasNumDash bool
	var subdirREADME, subdirNum bool
	for _, e := range ents {
		name := e.Name()
		if !e.IsDir() {
			if strings.EqualFold(name, "readme.md") {
				hasRootREADME = true
			}
			if matchedNumRoot(name, "_") {
				hasNumUnderscore = true
			}
			if matchedNumRoot(name, "-") {
				hasNumDash = true
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
			if strings.EqualFold(sn, "readme.md") {
				subdirREADME = true
			}
			if matchedNumNested(sn) {
				subdirNum = true
			}
		}
	}

	if hasRootREADME {
		out = append(out, DetectedPattern{Label: "Root README.md", Match: "README.md", DefaultKind: config.KindPlan})
	}
	if hasNumUnderscore {
		out = append(out, DetectedPattern{Label: "Numbered root files (NN_TITLE.md)", Match: "[0-9][0-9]_*.md", DefaultKind: config.KindPlan})
	}
	if hasNumDash {
		out = append(out, DetectedPattern{Label: "Numbered root files (NN-title.md)", Match: "[0-9][0-9]-*.md", DefaultKind: config.KindPlan})
	}
	if subdirREADME {
		out = append(out, DetectedPattern{Label: "Subfolder README.md", Match: "*/README.md", DefaultKind: config.KindPlan})
	}
	if subdirNum {
		out = append(out, DetectedPattern{Label: "Nested numbered steps", Match: "*/[0-9][0-9]-*.md", DefaultKind: config.KindPlan})
	}

	return out
}

func matchedNumRoot(name, sep string) bool {
	base := strings.TrimSuffix(strings.ToLower(name), ".md")
	if len(base) < 4 {
		return false
	}
	if base[0] < '0' || base[0] > '9' || base[1] < '0' || base[1] > '9' {
		return false
	}
	return strings.Contains(base[2:], sep)
}

func matchedNumNested(name string) bool {
	base := strings.TrimSuffix(strings.ToLower(name), ".md")
	if len(base) < 4 {
		return false
	}
	if base[0] < '0' || base[0] > '9' || base[1] < '0' || base[1] > '9' {
		return false
	}
	return base[2] == '-'
}
