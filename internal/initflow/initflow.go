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

	for _, dir := range paths {
		dirRules, err := configureDirRules(repoRoot, dir)
		if err != nil {
			return nil, nil, err
		}
		rules = append(rules, dirRules...)
	}

	return paths, rules, nil
}

// configureDirRules walks one directory: shows detected pattern proposals as a
// multi-select, then offers a manual rule-entry loop for anything the user
// wants to add beyond what was detected.
func configureDirRules(repoRoot, dir string) ([]config.SourceRule, error) {
	patterns := DetectPatterns(repoRoot, dir)
	var rules []config.SourceRule

	if len(patterns) > 0 {
		opts := make([]huh.Option[string], 0, len(patterns))
		kindForMatch := make(map[string]string)
		var preSelected []string

		for _, p := range patterns {
			label := fmt.Sprintf("%s \u2192 %s (%d file", p.Match, p.DefaultKind, p.FileCount)
			if p.FileCount != 1 {
				label += "s"
			}
			label += ")"
			opts = append(opts, huh.NewOption(label, p.Match))
			kindForMatch[p.Match] = p.DefaultKind
			preSelected = append(preSelected, p.Match)
		}

		selected := append([]string(nil), preSelected...)
		selectForm := huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title(fmt.Sprintf("Rules for %s", dir)).
					Description("Detected patterns. Space to toggle, enter to confirm.").
					Options(opts...).
					Value(&selected),
			),
		)
		if err := selectForm.Run(); err != nil {
			return nil, err
		}

		for _, match := range selected {
			rules = append(rules, config.SourceRule{
				Match: match,
				Kind:  kindForMatch[match],
			})
		}
	}

	// Manual rule loop
	for {
		var addCustom bool
		prompt := fmt.Sprintf("Add a custom rule for %s?", dir)
		if len(patterns) == 0 {
			prompt = fmt.Sprintf("Add a rule for %s?", dir)
		}
		customForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(prompt).
					Description("Enter a glob pattern and artifact kind.").
					Value(&addCustom),
			),
		)
		if err := customForm.Run(); err != nil {
			return nil, err
		}
		if !addCustom {
			break
		}

		var match, kindStr, subStr string
		ruleForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Path pattern").
					Description("Relative to "+dir+" (glob syntax: *, ?, []). Examples: ROADMAP.md, */README.md").
					Value(&match),
				huh.NewSelect[string]().
					Title("Artifact kind").
					Options(kindPickOptions()...).
					Value(&kindStr),
				huh.NewInput().
					Title("Subtype (optional)").
					Description("Usually blank. If needed: adr, openspec_change, or prd.").
					Value(&subStr),
			),
		)
		if err := ruleForm.Run(); err != nil {
			return nil, err
		}
		match = strings.TrimSpace(match)
		subStr = strings.TrimSpace(subStr)
		if match == "" {
			fmt.Fprintln(os.Stderr, "Skipping empty pattern.")
			continue
		}
		match = filepath.ToSlash(match)
		rule := config.SourceRule{Match: match, Kind: kindStr, Subtype: subStr}
		if err := config.ValidateSubtype(rule.Kind, rule.Subtype); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid rule: %v\n", err)
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func kindPickOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("plan \u2014 plans, roadmaps, checklists", config.KindPlan),
		huh.NewOption("spec \u2014 specifications, proposals", config.KindSpec),
		huh.NewOption("requirements \u2014 PRDs, needs", config.KindRequirements),
		huh.NewOption("design \u2014 design notes", config.KindDesign),
		huh.NewOption("contract \u2014 APIs, interfaces", config.KindContract),
		huh.NewOption("decision \u2014 ADRs, decisions", config.KindDecision),
		huh.NewOption("markdown_artifact \u2014 generic markdown", config.KindMarkdownArtifact),
	}
}
