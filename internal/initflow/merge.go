package initflow

import (
	"sort"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/profiles"
)

// MergeSelectedProfiles merges built-in profile selections and optional custom markdown paths/rules
// into a copy of base. Empty selectedIDs leaves base unchanged (aside from custom additions).
func MergeSelectedProfiles(base *config.RepoConfig, selectedIDs []string, customPaths []string, customRules []config.SourceRule) (*config.RepoConfig, error) {
	cfg := config.CloneRepoConfig(base)
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if len(selectedIDs) == 0 && len(customPaths) == 0 && len(customRules) == 0 {
		return cfg, nil
	}

	have := map[string]struct{}{}
	for _, id := range selectedIDs {
		have[id] = struct{}{}
	}

	for id := range have {
		if id == profiles.IDCustom {
			continue
		}
		p, ok := profiles.ByID(id)
		if !ok {
			continue
		}
		switch p.SourceType {
		case "markdown":
			md := findOrCreateMarkdown(cfg)
			md.Paths = mergeSortedUniqueStrings(md.Paths, p.Paths)
			md.Rules = append(md.Rules, p.Rules...)
		case "openspec":
			found := false
			for i := range cfg.Sources {
				if cfg.Sources[i].Type == "openspec" {
					cfg.Sources[i].Path = "openspec"
					found = true
					break
				}
			}
			if !found {
				cfg.Sources = append(cfg.Sources, config.SourceConfig{Type: "openspec", Path: "openspec"})
			}
		case "adr":
			ad := findOrCreateADR(cfg)
			ad.Paths = mergeSortedUniqueStrings(ad.Paths, p.Paths)
		}
	}

	if len(customPaths) > 0 || len(customRules) > 0 {
		md := findOrCreateMarkdown(cfg)
		md.Paths = mergeSortedUniqueStrings(md.Paths, customPaths)
		md.Rules = append(md.Rules, customRules...)
	}

	if err := config.ValidateRepoConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func findOrCreateMarkdown(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "markdown" {
			return &cfg.Sources[i]
		}
	}
	cfg.Sources = append(cfg.Sources, config.SourceConfig{Type: "markdown"})
	return &cfg.Sources[len(cfg.Sources)-1]
}

func findOrCreateADR(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "adr" {
			return &cfg.Sources[i]
		}
	}
	cfg.Sources = append(cfg.Sources, config.SourceConfig{Type: "adr"})
	return &cfg.Sources[len(cfg.Sources)-1]
}

func mergeSortedUniqueStrings(base, extra []string) []string {
	seen := make(map[string]struct{})
	var acc []string
	for _, p := range base {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		acc = append(acc, p)
	}
	for _, p := range extra {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		acc = append(acc, p)
	}
	sort.Strings(acc)
	return acc
}
