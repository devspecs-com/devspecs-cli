package config

import (
	"fmt"
	"strings"
)

// Canonical artifact kinds (closed vocabulary).
const (
	KindPlan             = "plan"
	KindSpec             = "spec"
	KindRequirements     = "requirements"
	KindDesign           = "design"
	KindContract         = "contract"
	KindDecision         = "decision"
	KindMarkdownArtifact = "markdown_artifact"
)

// Known subtypes per kind (empty subtype is always allowed).
const (
	SubtypeADR            = "adr"
	SubtypeOpenspecChange = "openspec_change"
	SubtypePRD            = "prd"
)

var validKinds = map[string]struct{}{
	KindPlan:             {},
	KindSpec:             {},
	KindRequirements:     {},
	KindDesign:           {},
	KindContract:         {},
	KindDecision:         {},
	KindMarkdownArtifact: {},
}

// allowedSubtypes maps kind -> set of allowed non-empty subtype strings.
var allowedSubtypes = map[string]map[string]struct{}{
	KindDecision: {
		SubtypeADR: {},
	},
	KindSpec: {
		SubtypeOpenspecChange: {},
	},
	KindRequirements: {
		SubtypePRD: {},
	},
}

// ValidateKind returns an error if kind is not in the closed vocabulary.
func ValidateKind(kind string) error {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return fmt.Errorf("artifact kind is empty")
	}
	if _, ok := validKinds[kind]; !ok {
		return fmt.Errorf("unknown artifact kind %q", kind)
	}
	return nil
}

// ValidateSubtype returns an error if subtype is non-empty but not allowed for the given kind.
func ValidateSubtype(kind, subtype string) error {
	if err := ValidateKind(kind); err != nil {
		return err
	}
	subtype = strings.TrimSpace(subtype)
	if subtype == "" {
		return nil
	}
	set, hasKind := allowedSubtypes[kind]
	if !hasKind || len(set) == 0 {
		return fmt.Errorf("kind %q does not allow subtype %q", kind, subtype)
	}
	if _, ok := set[subtype]; !ok {
		return fmt.Errorf("unknown subtype %q for kind %q", subtype, kind)
	}
	return nil
}

// ValidateSourceRules checks source rule kinds and subtypes.
func ValidateSourceRules(rules []SourceRule) error {
	for i, r := range rules {
		if strings.TrimSpace(r.Match) == "" {
			return fmt.Errorf("sources.rules[%d]: match is empty", i)
		}
		if err := ValidateSubtype(r.Kind, r.Subtype); err != nil {
			return fmt.Errorf("sources.rules[%d] (match %q): %w", i, r.Match, err)
		}
	}
	return nil
}

// ValidateRepoConfig checks kinds in markdown source rules.
func ValidateRepoConfig(cfg *RepoConfig) error {
	if cfg == nil {
		return nil
	}
	for si, src := range cfg.Sources {
		if src.Type != "markdown" {
			continue
		}
		if err := ValidateSourceRules(src.Rules); err != nil {
			return fmt.Errorf("sources[%d]: %w", si, err)
		}
	}
	return nil
}
