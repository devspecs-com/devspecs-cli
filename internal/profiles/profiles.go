// Package profiles defines built-in workflow presets for ds init.
package profiles

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

// Sentinel ID for the custom markdown wizard (not a filesystem detectable preset).
const IDCustom = "custom"

// WorkflowProfile is a selectable preset that contributes paths and optional markdown rules.
type WorkflowProfile struct {
	ID          string
	Label       string
	Description string
	// SourceType is "markdown", "openspec", or "adr".
	SourceType string
	Paths      []string
	DetectDirs []string
	Rules      []config.SourceRule
}

// All returns the registry of built-in profiles (excluding Custom).
func All() []WorkflowProfile {
	return []WorkflowProfile{
		{
			ID: "cursor", Label: "Cursor (.cursor/plans/)", Description: "Cursor saved plans",
			SourceType: "markdown", Paths: []string{".cursor/plans"}, DetectDirs: []string{".cursor/plans"},
			Rules: []config.SourceRule{{Match: "*.plan.md", Kind: config.KindPlan}},
		},
		{
			ID: "claude", Label: "Claude (plans/)", Description: "Markdown plans under plans/",
			SourceType: "markdown", Paths: []string{"plans"}, DetectDirs: []string{"plans"},
		},
		{
			ID: "codex", Label: "Codex (plans/)", Description: "Codex PLAN.md style",
			SourceType: "markdown", Paths: []string{"plans"}, DetectDirs: []string{"plans"},
			Rules: []config.SourceRule{{Match: "PLAN.md", Kind: config.KindPlan}},
		},
		{
			ID: "openspec", Label: "OpenSpec (openspec/)", Description: "OpenSpec change proposals",
			SourceType: "openspec", Paths: nil, DetectDirs: []string{"openspec"},
		},
		{
			ID: "bmad", Label: "BMAD (_bmad-output/)", Description: "BMAD planning artifacts",
			SourceType: "markdown", Paths: []string{"_bmad-output"}, DetectDirs: []string{"_bmad-output"},
			Rules: []config.SourceRule{{Match: "planning-artifacts/*.md", Kind: config.KindRequirements, Subtype: config.SubtypePRD}},
		},
		{
			ID: "speckit", Label: "Spec Kit (.specify/, specs/)", Description: "GitHub Spec Kit layouts",
			SourceType: "markdown", Paths: []string{".specify/memory", "specs"}, DetectDirs: []string{".specify", "specs"},
			Rules: []config.SourceRule{
				{Match: "*/spec.md", Kind: config.KindSpec},
				{Match: "*/tasks.md", Kind: config.KindPlan},
				{Match: "*/plan.md", Kind: config.KindPlan},
			},
		},
		{
			ID: "adr", Label: "ADR (docs/adr/, adr/)", Description: "Architecture decision records",
			SourceType: "adr", Paths: []string{"docs/adr", "docs/adrs", "adr", "adrs"}, DetectDirs: []string{"docs/adr", "docs/adrs", "adr", "adrs"},
		},
		{
			ID: "docs", Label: "Docs (docs/specs/, docs/plans/, docs/prd/)", Description: "Specs, plans, PRDs, and design notes under docs/",
			SourceType: "markdown", Paths: []string{"docs/specs", "docs/plans", "docs/prd", "docs/design", "docs/technical"},
			DetectDirs: []string{"docs/specs", "docs/plans", "docs/prd"},
		},
	}
}

// CustomProfile returns the synthetic custom profile entry for multi-select UIs.
func CustomProfile() WorkflowProfile {
	return WorkflowProfile{
		ID: IDCustom, Label: "Custom markdown paths…", Description: "Wizard for your folder patterns",
		SourceType: "markdown",
	}
}

// Detect returns profile IDs whose DetectDirs match existing (non-ignored) directories.
func Detect(repoRoot string, m *ignore.Matcher) []string {
	if m == nil {
		var err error
		m, err = ignore.NewMatcher(repoRoot)
		if err != nil {
			m = nil
		}
	}
	var found []string
	for _, p := range All() {
		for _, d := range p.DetectDirs {
			d = filepath.ToSlash(d)
			if m != nil && m.ShouldSkip(d, true) {
				continue
			}
			abs := filepath.Join(repoRoot, filepath.FromSlash(d))
			if st, err := os.Stat(abs); err == nil && st.IsDir() {
				found = append(found, p.ID)
				break
			}
		}
	}
	sort.Strings(found)
	return dedupeStrings(found)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ByID returns a built-in profile or ok=false.
func ByID(id string) (WorkflowProfile, bool) {
	for _, p := range All() {
		if p.ID == id {
			return p, true
		}
	}
	return WorkflowProfile{}, false
}
