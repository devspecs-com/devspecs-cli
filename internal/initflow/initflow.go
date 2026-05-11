package initflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/profiles"
)

// RunProfilePick runs the interactive profile multi-select and optional custom markdown wizard.
// It returns selected profile IDs (excluding custom), plus paths and rules contributed by the custom wizard.
func RunProfilePick(repoRoot string) (selected []string, customPaths []string, customRules []config.SourceRule, err error) {
	detected := profiles.Detect(repoRoot, nil)
	opts := make([]huh.Option[string], 0, len(profiles.All())+1)
	for _, p := range profiles.All() {
		opts = append(opts, huh.NewOption(p.Label, p.ID))
	}
	cp := profiles.CustomProfile()
	opts = append(opts, huh.NewOption(cp.Label, profiles.IDCustom))

	selected = append([]string(nil), detected...)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select workflow profiles to index").
				Description("Space toggles, enter confirms. Detected layouts are pre-selected.").
				Options(opts...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return nil, nil, nil, err
	}

	haveCustom := false
	filtered := selected[:0]
	for _, id := range selected {
		if id == profiles.IDCustom {
			haveCustom = true
			continue
		}
		filtered = append(filtered, id)
	}
	selected = filtered

	if haveCustom {
		cp, cr, err := runCustomMarkdownWizard(repoRoot)
		if err != nil {
			return nil, nil, nil, err
		}
		customPaths = cp
		customRules = cr
	}

	return selected, customPaths, customRules, nil
}

func runCustomMarkdownWizard(repoRoot string) (paths []string, rules []config.SourceRule, err error) {
	var raw string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Custom markdown paths").
				Description("Comma-separated directories relative to the repo root (e.g. v2/plans, decisions).").
				Value(&raw),
		),
	)
	if err := form.Run(); err != nil {
		return nil, nil, err
	}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		slash := filepath.ToSlash(p)
		abs := filepath.Join(repoRoot, filepath.FromSlash(slash))
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return nil, nil, fmt.Errorf("not a directory: %s", slash)
		}
		paths = append(paths, slash)
	}
	if len(paths) == 0 {
		return nil, nil, nil
	}

	var configureRules bool
	introForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Configure kind rules for these folders?").
				Description("Map path patterns to artifact kinds. Each pattern is matched relative to every folder you listed (not from the repo root). Uses glob syntax: *, ?, and []. Example: ROADMAP.md or */README.md or [0-9][0-9]-*.md. You can skip and edit .devspecs/config.yaml later.").
				Value(&configureRules),
		),
	)
	if err := introForm.Run(); err != nil {
		return nil, nil, err
	}
	if !configureRules {
		return paths, nil, nil
	}

	for {
		var match, kindStr, subStr string
		ruleForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Path pattern").
					Description("Relative to each custom folder (use /). Examples: ROADMAP.md, */README.md, */[0-9][0-9]-*.md").
					Value(&match),
				huh.NewSelect[string]().
					Title("Artifact kind").
					Options(kindPickOptions()...).
					Value(&kindStr),
				huh.NewInput().
					Title("Subtype (optional)").
					Description("Usually leave blank. If needed: adr (with decision), openspec_change (with spec), prd (with requirements).").
					Value(&subStr),
			),
		)
		if err := ruleForm.Run(); err != nil {
			return nil, nil, err
		}
		match = strings.TrimSpace(match)
		subStr = strings.TrimSpace(subStr)
		if match == "" {
			fmt.Fprintln(os.Stderr, "Skipping empty pattern.")
		} else {
			match = filepath.ToSlash(match)
			rule := config.SourceRule{Match: match, Kind: kindStr, Subtype: subStr}
			if err := config.ValidateSubtype(rule.Kind, rule.Subtype); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid rule (%s → %s / %s): %v\n", match, kindStr, subStr, err)
			} else {
				rules = append(rules, rule)
			}
		}

		var more bool
		moreForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Add another rule?").
					Value(&more),
			),
		)
		if err := moreForm.Run(); err != nil {
			return nil, nil, err
		}
		if !more {
			break
		}
	}

	return paths, rules, nil
}

func kindPickOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("plan — plans, roadmaps, checklists", config.KindPlan),
		huh.NewOption("spec — specifications, proposals", config.KindSpec),
		huh.NewOption("requirements — PRDs, needs", config.KindRequirements),
		huh.NewOption("design — design notes", config.KindDesign),
		huh.NewOption("contract — APIs, interfaces", config.KindContract),
		huh.NewOption("decision — ADRs, decisions", config.KindDecision),
		huh.NewOption("markdown_artifact — generic markdown", config.KindMarkdownArtifact),
	}
}
