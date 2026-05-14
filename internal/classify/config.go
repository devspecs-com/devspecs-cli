package classify

import (
	"fmt"
	"sort"
)

const (
	ProfileBuiltinIntentDocsV1 = "builtin_intent_docs_v1"

	ModelOpenSpec        = "openspec"
	ModelADR             = "adr"
	ModelRFC             = "rfc"
	ModelPRD             = "prd"
	ModelPlan            = "plan"
	ModelAgentNote       = "agent_note"
	ModelGenericMarkdown = "generic_markdown"

	SubmodelOpenSpecContainer  = "openspec.container"
	SubmodelOpenSpecDocument   = "openspec.document"
	SubmodelADRNygard          = "adr.nygard"
	SubmodelADRMADR            = "adr.madr"
	SubmodelADRYStatement      = "adr.y_statement"
	SubmodelRFCSectionPattern  = "rfc.section_pattern"
	SubmodelPRDProductIntent   = "prd.product_intent"
	SubmodelPlanImplementation = "plan.implementation_plan"
	SubmodelPlanMigration      = "plan.migration_plan"
	SubmodelPlanRollout        = "plan.rollout_plan"
	SubmodelAgentContinuation  = "agent_note.continuation_note"
	SubmodelAgentFollowup      = "agent_note.followup_note"
	SubmodelAgentBlocker       = "agent_note.blocker_note"
)

const (
	AuthorityHighCurrentIntent = "high_current_intent"
	AuthorityHighDecision      = "high_decision"
	AuthorityDesignProposal    = "design_proposal"
	AuthorityProductBackground = "product_background"
	AuthorityWorkingPlan       = "working_plan"
	AuthorityHandoffNote       = "handoff_note"
	AuthorityNeutral           = "neutral"
)

type PipelineConfig struct {
	Version     int                    `json:"version" yaml:"version"`
	Profile     string                 `json:"profile" yaml:"profile"`
	Discovery   DiscoveryConfig        `json:"discovery" yaml:"discovery"`
	Resolver    ResolverConfig         `json:"resolver" yaml:"resolver"`
	Models      map[string]ModelConfig `json:"models" yaml:"models"`
	LocalModels LocalModelsConfig      `json:"local_models,omitempty" yaml:"local_models,omitempty"`
}

type DiscoveryConfig struct {
	Mode                          string   `json:"mode" yaml:"mode"`
	IncludeConfiguredSources      bool     `json:"include_configured_sources" yaml:"include_configured_sources"`
	IncludeKnownIntentConventions bool     `json:"include_known_intent_conventions" yaml:"include_known_intent_conventions"`
	IncludeNestedDocsConventions  bool     `json:"include_nested_docs_conventions" yaml:"include_nested_docs_conventions"`
	BroadMarkdownDiscovery        bool     `json:"broad_markdown_discovery" yaml:"broad_markdown_discovery"`
	MaxFileSizeBytes              int64    `json:"max_file_size_bytes" yaml:"max_file_size_bytes"`
	MaxCandidates                 int      `json:"max_candidates" yaml:"max_candidates"`
	IgnoreGenerated               bool     `json:"ignore_generated" yaml:"ignore_generated"`
	IgnoreVendored                bool     `json:"ignore_vendored" yaml:"ignore_vendored"`
	ExtraIncludeGlobs             []string `json:"extra_include_globs,omitempty" yaml:"extra_include_globs,omitempty"`
	ExtraExcludeGlobs             []string `json:"extra_exclude_globs,omitempty" yaml:"extra_exclude_globs,omitempty"`
}

type ResolverConfig struct {
	StrongAccept           float64 `json:"strong_accept" yaml:"strong_accept"`
	WeakAccept             float64 `json:"weak_accept" yaml:"weak_accept"`
	AmbiguityGap           float64 `json:"ambiguity_gap" yaml:"ambiguity_gap"`
	RejectBelow            float64 `json:"reject_below" yaml:"reject_below"`
	Fallback               string  `json:"fallback" yaml:"fallback"`
	ConfiguredPathPrior    float64 `json:"configured_path_prior" yaml:"configured_path_prior"`
	ConfiguredPathCanForce bool    `json:"configured_path_can_force" yaml:"configured_path_can_force"`
}

type ModelConfig struct {
	Enabled         bool                      `json:"enabled" yaml:"enabled"`
	Scopes          []Scope                   `json:"scopes" yaml:"scopes"`
	Authority       string                    `json:"authority,omitempty" yaml:"authority,omitempty"`
	PathHints       []string                  `json:"path_hints,omitempty" yaml:"path_hints,omitempty"`
	LayoutHints     map[string]string         `json:"layout_hints,omitempty" yaml:"layout_hints,omitempty"`
	EmitsEdges      []string                  `json:"emits_edges,omitempty" yaml:"emits_edges,omitempty"`
	Subformats      map[string]SubmodelConfig `json:"subformats,omitempty" yaml:"subformats,omitempty"`
	Families        map[string]SubmodelConfig `json:"families,omitempty" yaml:"families,omitempty"`
	NamedSubformats []string                  `json:"named_subformats,omitempty" yaml:"named_subformats,omitempty"`
	Fallback        bool                      `json:"fallback,omitempty" yaml:"fallback,omitempty"`
}

type SubmodelConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type LocalModelsConfig struct {
	Enabled     bool                   `json:"enabled" yaml:"enabled"`
	Definitions []LocalModelDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
}

type LocalModelDefinition struct {
	ID               string   `json:"id" yaml:"id"`
	BaseModel        string   `json:"base_model" yaml:"base_model"`
	Authority        string   `json:"authority,omitempty" yaml:"authority,omitempty"`
	PathHints        []string `json:"path_hints,omitempty" yaml:"path_hints,omitempty"`
	RequiredHeadings []string `json:"required_headings,omitempty" yaml:"required_headings,omitempty"`
	PositiveTerms    []string `json:"positive_terms,omitempty" yaml:"positive_terms,omitempty"`
	NegativeTerms    []string `json:"negative_terms,omitempty" yaml:"negative_terms,omitempty"`
	Experimental     bool     `json:"experimental,omitempty" yaml:"experimental,omitempty"`
}

func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		Version: 1,
		Profile: ProfileBuiltinIntentDocsV1,
		Discovery: DiscoveryConfig{
			Mode:                          "conservative",
			IncludeConfiguredSources:      true,
			IncludeKnownIntentConventions: true,
			IncludeNestedDocsConventions:  true,
			BroadMarkdownDiscovery:        false,
			MaxFileSizeBytes:              262144,
			MaxCandidates:                 2000,
			IgnoreGenerated:               true,
			IgnoreVendored:                true,
		},
		Resolver: ResolverConfig{
			StrongAccept:           0.75,
			WeakAccept:             0.55,
			AmbiguityGap:           0.15,
			RejectBelow:            0.35,
			Fallback:               ModelGenericMarkdown,
			ConfiguredPathPrior:    0.10,
			ConfiguredPathCanForce: false,
		},
		Models: map[string]ModelConfig{
			ModelOpenSpec: {
				Enabled:   true,
				Scopes:    []Scope{ScopeContainer, ScopeDocument},
				Authority: AuthorityHighCurrentIntent,
				PathHints: []string{"openspec/changes/**"},
				LayoutHints: map[string]string{
					"proposal": "proposal.md",
					"design":   "design.md",
					"tasks":    "tasks.md",
					"specs":    "specs/**/spec.md",
				},
				EmitsEdges: []string{"openspec_companion"},
			},
			ModelADR: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityHighDecision,
				PathHints: []string{"docs/adr/**", "docs/adrs/**", "adr/**", "adrs/**"},
				Subformats: map[string]SubmodelConfig{
					"nygard":      {Enabled: true},
					"madr":        {Enabled: true},
					"y_statement": {Enabled: true},
				},
			},
			ModelRFC: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityDesignProposal,
				PathHints: []string{"rfcs/**", "docs/rfcs/**", "docs/proposals/**"},
				Families: map[string]SubmodelConfig{
					"section_pattern": {Enabled: true},
				},
			},
			ModelPRD: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityProductBackground,
				PathHints: []string{"docs/prd/**", "docs/prds/**", "prd/**", "prds/**"},
				Families: map[string]SubmodelConfig{
					"product_intent": {Enabled: true},
				},
			},
			ModelPlan: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityWorkingPlan,
				PathHints: []string{"plans/**", "docs/plans/**"},
				Families: map[string]SubmodelConfig{
					"implementation_plan": {Enabled: true},
					"migration_plan":      {Enabled: true},
					"rollout_plan":        {Enabled: true},
				},
			},
			ModelAgentNote: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityHandoffNote,
				PathHints: []string{".cursor/plans/**", ".claude/**", ".codex/**"},
				Families: map[string]SubmodelConfig{
					"continuation_note": {Enabled: true},
					"followup_note":     {Enabled: true},
					"blocker_note":      {Enabled: true},
				},
			},
			ModelGenericMarkdown: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Authority: AuthorityNeutral,
				Fallback:  true,
			},
		},
		LocalModels: LocalModelsConfig{Enabled: true},
	}
}

func DocumentedModelIDs() []string {
	return []string{
		ModelOpenSpec,
		SubmodelOpenSpecContainer,
		SubmodelOpenSpecDocument,
		ModelADR,
		SubmodelADRNygard,
		SubmodelADRMADR,
		SubmodelADRYStatement,
		ModelRFC,
		SubmodelRFCSectionPattern,
		ModelPRD,
		SubmodelPRDProductIntent,
		ModelPlan,
		SubmodelPlanImplementation,
		SubmodelPlanMigration,
		SubmodelPlanRollout,
		ModelAgentNote,
		SubmodelAgentContinuation,
		SubmodelAgentFollowup,
		SubmodelAgentBlocker,
		ModelGenericMarkdown,
	}
}

func MissingDocumentedModels(cfg PipelineConfig) []string {
	var missing []string
	needModel := func(id string) {
		if _, ok := cfg.Models[id]; !ok {
			missing = append(missing, id)
		}
	}
	needModel(ModelOpenSpec)
	needModel(ModelADR)
	needModel(ModelRFC)
	needModel(ModelPRD)
	needModel(ModelPlan)
	needModel(ModelAgentNote)
	needModel(ModelGenericMarkdown)

	if !hasScope(cfg.Models[ModelOpenSpec], ScopeContainer) {
		missing = append(missing, SubmodelOpenSpecContainer)
	}
	if !hasScope(cfg.Models[ModelOpenSpec], ScopeDocument) {
		missing = append(missing, SubmodelOpenSpecDocument)
	}
	for _, tc := range []struct {
		model string
		key   string
		id    string
	}{
		{ModelADR, "nygard", SubmodelADRNygard},
		{ModelADR, "madr", SubmodelADRMADR},
		{ModelADR, "y_statement", SubmodelADRYStatement},
		{ModelRFC, "section_pattern", SubmodelRFCSectionPattern},
		{ModelPRD, "product_intent", SubmodelPRDProductIntent},
		{ModelPlan, "implementation_plan", SubmodelPlanImplementation},
		{ModelPlan, "migration_plan", SubmodelPlanMigration},
		{ModelPlan, "rollout_plan", SubmodelPlanRollout},
		{ModelAgentNote, "continuation_note", SubmodelAgentContinuation},
		{ModelAgentNote, "followup_note", SubmodelAgentFollowup},
		{ModelAgentNote, "blocker_note", SubmodelAgentBlocker},
	} {
		model, ok := cfg.Models[tc.model]
		if !ok {
			continue
		}
		if _, ok := model.Subformats[tc.key]; ok {
			continue
		}
		if _, ok := model.Families[tc.key]; ok {
			continue
		}
		missing = append(missing, tc.id)
	}
	sort.Strings(missing)
	return missing
}

func ValidateConfig(cfg PipelineConfig) error {
	if cfg.Version != 1 {
		return fmt.Errorf("classifier_pipeline.version must be 1, got %d", cfg.Version)
	}
	if cfg.Profile == "" {
		return fmt.Errorf("classifier_pipeline.profile is required")
	}
	if cfg.Discovery.MaxFileSizeBytes < 0 {
		return fmt.Errorf("classifier_pipeline.discovery.max_file_size_bytes must be non-negative")
	}
	if cfg.Discovery.MaxCandidates < 0 {
		return fmt.Errorf("classifier_pipeline.discovery.max_candidates must be non-negative")
	}
	if err := validateResolver(cfg.Resolver); err != nil {
		return err
	}
	for id, model := range cfg.Models {
		if len(model.Scopes) == 0 {
			return fmt.Errorf("classifier model %q must declare at least one scope", id)
		}
		for _, scope := range model.Scopes {
			if err := ValidateScope(scope); err != nil {
				return fmt.Errorf("classifier model %q: %w", id, err)
			}
		}
	}
	if missing := MissingDocumentedModels(cfg); len(missing) > 0 {
		return fmt.Errorf("classifier_pipeline missing documented models: %v", missing)
	}
	for _, local := range cfg.LocalModels.Definitions {
		if local.ID == "" {
			return fmt.Errorf("local model id is required")
		}
		if local.BaseModel == "" {
			return fmt.Errorf("local model %q base_model is required", local.ID)
		}
		if _, ok := cfg.Models[local.BaseModel]; !ok {
			return fmt.Errorf("local model %q references unknown base_model %q", local.ID, local.BaseModel)
		}
	}
	return nil
}

func validateResolver(r ResolverConfig) error {
	for name, value := range map[string]float64{
		"strong_accept":         r.StrongAccept,
		"weak_accept":           r.WeakAccept,
		"ambiguity_gap":         r.AmbiguityGap,
		"reject_below":          r.RejectBelow,
		"configured_path_prior": r.ConfiguredPathPrior,
	} {
		if value < 0 || value > 1 {
			return fmt.Errorf("classifier_pipeline.resolver.%s must be between 0 and 1", name)
		}
	}
	if r.StrongAccept < r.WeakAccept {
		return fmt.Errorf("classifier_pipeline.resolver.strong_accept must be >= weak_accept")
	}
	if r.WeakAccept < r.RejectBelow {
		return fmt.Errorf("classifier_pipeline.resolver.weak_accept must be >= reject_below")
	}
	if r.Fallback == "" {
		return fmt.Errorf("classifier_pipeline.resolver.fallback is required")
	}
	return nil
}

func hasScope(model ModelConfig, scope Scope) bool {
	for _, got := range model.Scopes {
		if got == scope {
			return true
		}
	}
	return false
}
