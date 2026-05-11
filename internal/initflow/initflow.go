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

	var confirm bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add default planning rules for these folders?").
				Description("Maps README.md, numbered *.md files, and nested steps to kind \"plan\" (editable later in .devspecs/config.yaml).").
				Value(&confirm),
		),
	)
	if err := confirmForm.Run(); err != nil {
		return nil, nil, err
	}
	if !confirm {
		return paths, nil, nil
	}

	for _, p := range paths {
		for _, pat := range DetectPatterns(repoRoot, p) {
			if pat.Match == "" || pat.SkipRule {
				continue
			}
			rules = append(rules, config.SourceRule{
				Match: pat.Match,
				Kind:  pat.DefaultKind,
			})
		}
	}
	return paths, rules, nil
}
