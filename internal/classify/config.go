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
	Enabled             bool                      `json:"enabled" yaml:"enabled"`
	Scopes              []Scope                   `json:"scopes" yaml:"scopes"`
	Kind                string                    `json:"kind,omitempty" yaml:"kind,omitempty"`
	Subtype             string                    `json:"subtype,omitempty" yaml:"subtype,omitempty"`
	Authority           string                    `json:"authority,omitempty" yaml:"authority,omitempty"`
	FormatProfile       string                    `json:"format_profile,omitempty" yaml:"format_profile,omitempty"`
	PathHints           []string                  `json:"path_hints,omitempty" yaml:"path_hints,omitempty"`
	LayoutHints         map[string]string         `json:"layout_hints,omitempty" yaml:"layout_hints,omitempty"`
	EmitsEdges          []string                  `json:"emits_edges,omitempty" yaml:"emits_edges,omitempty"`
	EmitChildCandidates bool                      `json:"emit_child_candidates,omitempty" yaml:"emit_child_candidates,omitempty"`
	Evidence            []EvidenceRule            `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	NegativeEvidence    []EvidenceRule            `json:"negative_evidence,omitempty" yaml:"negative_evidence,omitempty"`
	Subformats          map[string]SubmodelConfig `json:"subformats,omitempty" yaml:"subformats,omitempty"`
	Families            map[string]SubmodelConfig `json:"families,omitempty" yaml:"families,omitempty"`
	NamedSubformats     []string                  `json:"named_subformats,omitempty" yaml:"named_subformats,omitempty"`
	Fallback            bool                      `json:"fallback,omitempty" yaml:"fallback,omitempty"`
}

type SubmodelConfig struct {
	Enabled          bool           `json:"enabled" yaml:"enabled"`
	Evidence         []EvidenceRule `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	NegativeEvidence []EvidenceRule `json:"negative_evidence,omitempty" yaml:"negative_evidence,omitempty"`
}

type EvidenceRule struct {
	ID      string        `json:"id" yaml:"id"`
	Weight  float64       `json:"weight" yaml:"weight"`
	Reason  ReasonCode    `json:"reason" yaml:"reason"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Match   EvidenceMatch `json:"match" yaml:"match"`
}

type EvidenceMatch struct {
	Always            bool              `json:"always,omitempty" yaml:"always,omitempty"`
	Scope             Scope             `json:"scope,omitempty" yaml:"scope,omitempty"`
	PathHints         bool              `json:"path_hints,omitempty" yaml:"path_hints,omitempty"`
	PathGlobs         []string          `json:"path_globs,omitempty" yaml:"path_globs,omitempty"`
	PathContainsAny   []string          `json:"path_contains_any,omitempty" yaml:"path_contains_any,omitempty"`
	PathSuffixesAny   []string          `json:"path_suffixes_any,omitempty" yaml:"path_suffixes_any,omitempty"`
	FilenameAny       []string          `json:"filename_any,omitempty" yaml:"filename_any,omitempty"`
	TitleAny          []string          `json:"title_any,omitempty" yaml:"title_any,omitempty"`
	TitleAll          []string          `json:"title_all,omitempty" yaml:"title_all,omitempty"`
	FrontmatterExists []string          `json:"frontmatter_exists,omitempty" yaml:"frontmatter_exists,omitempty"`
	FrontmatterEquals map[string]string `json:"frontmatter_equals,omitempty" yaml:"frontmatter_equals,omitempty"`
	HeadingsAny       []string          `json:"headings_any,omitempty" yaml:"headings_any,omitempty"`
	HeadingsAll       []string          `json:"headings_all,omitempty" yaml:"headings_all,omitempty"`
	SectionRolesAny   []string          `json:"section_roles_any,omitempty" yaml:"section_roles_any,omitempty"`
	SectionRolesAll   []string          `json:"section_roles_all,omitempty" yaml:"section_roles_all,omitempty"`
	ChecklistMin      int               `json:"checklist_min,omitempty" yaml:"checklist_min,omitempty"`
	DateTokensMin     int               `json:"date_tokens_min,omitempty" yaml:"date_tokens_min,omitempty"`
	MarkersAny        []string          `json:"markers_any,omitempty" yaml:"markers_any,omitempty"`
	IdentifiersAny    []string          `json:"identifiers_any,omitempty" yaml:"identifiers_any,omitempty"`
	LocalTermsAny     []string          `json:"local_terms_any,omitempty" yaml:"local_terms_any,omitempty"`
	BodyContainsAny   []string          `json:"body_contains_any,omitempty" yaml:"body_contains_any,omitempty"`
	BodyContainsAll   []string          `json:"body_contains_all,omitempty" yaml:"body_contains_all,omitempty"`
	BodyRegexAny      []string          `json:"body_regex_any,omitempty" yaml:"body_regex_any,omitempty"`
	ChildRolesAny     []string          `json:"child_roles_any,omitempty" yaml:"child_roles_any,omitempty"`
	ChildRolesAll     []string          `json:"child_roles_all,omitempty" yaml:"child_roles_all,omitempty"`
}

type LocalModelsConfig struct {
	Enabled     bool                   `json:"enabled" yaml:"enabled"`
	Definitions []LocalModelDefinition `json:"definitions,omitempty" yaml:"definitions,omitempty"`
}

type LocalModelDefinition struct {
	ID               string         `json:"id" yaml:"id"`
	BaseModel        string         `json:"base_model" yaml:"base_model"`
	Authority        string         `json:"authority,omitempty" yaml:"authority,omitempty"`
	PathHints        []string       `json:"path_hints,omitempty" yaml:"path_hints,omitempty"`
	RequiredHeadings []string       `json:"required_headings,omitempty" yaml:"required_headings,omitempty"`
	PositiveTerms    []string       `json:"positive_terms,omitempty" yaml:"positive_terms,omitempty"`
	NegativeTerms    []string       `json:"negative_terms,omitempty" yaml:"negative_terms,omitempty"`
	Evidence         []EvidenceRule `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	NegativeEvidence []EvidenceRule `json:"negative_evidence,omitempty" yaml:"negative_evidence,omitempty"`
	Experimental     bool           `json:"experimental,omitempty" yaml:"experimental,omitempty"`
}

func DefaultPipelineConfig() PipelineConfig {
	evidence := func(id string, weight float64, reason ReasonCode, message string, match EvidenceMatch) EvidenceRule {
		return EvidenceRule{
			ID:      id,
			Weight:  weight,
			Reason:  reason,
			Message: message,
			Match:   match,
		}
	}

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
				Enabled:             true,
				Scopes:              []Scope{ScopeContainer, ScopeDocument},
				Kind:                "spec",
				Authority:           AuthorityHighCurrentIntent,
				FormatProfile:       "openspec",
				PathHints:           []string{"openspec/changes/**"},
				EmitChildCandidates: true,
				LayoutHints: map[string]string{
					"proposal": "proposal.md",
					"design":   "design.md",
					"tasks":    "tasks.md",
					"specs":    "specs/**/spec.md",
				},
				EmitsEdges: []string{"openspec_companion"},
				Evidence: []EvidenceRule{
					evidence("openspec_container_core_layout", 0.55, ReasonLayoutMatch, "OpenSpec change container includes proposal, design, and tasks roles.", EvidenceMatch{
						Scope:         ScopeContainer,
						ChildRolesAll: []string{"proposal", "design", "tasks"},
					}),
					evidence("openspec_container_spec_delta", 0.12, ReasonContainerChild, "OpenSpec container includes a spec delta child.", EvidenceMatch{
						Scope:         ScopeContainer,
						ChildRolesAny: []string{"spec_delta"},
					}),
					evidence("openspec_document_layout", 0.42, ReasonLayoutMatch, "OpenSpec document path matches a known child artifact layout.", EvidenceMatch{
						Scope: ScopeDocument,
						PathGlobs: []string{
							"openspec/changes/*/proposal.md",
							"openspec/changes/*/design.md",
							"openspec/changes/*/tasks.md",
							"openspec/changes/*/specs/*/spec.md",
						},
					}),
					evidence("openspec_requirements_language", 0.12, ReasonHeadingMatch, "OpenSpec document includes requirements, tasks, or impact structure.", EvidenceMatch{
						Scope:       ScopeDocument,
						HeadingsAny: []string{"proposal", "motivation", "impact", "requirements", "tasks"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("openspec_generated_marker", 0.20, ReasonGeneratedMarker, "Generated marker reduces OpenSpec confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
					evidence("openspec_changelog_marker", 0.14, ReasonChangelogMarker, "Changelog marker reduces OpenSpec confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
				},
			},
			ModelADR: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "decision",
				Authority: AuthorityHighDecision,
				PathHints: []string{"docs/adr/**", "docs/adrs/**", "adr/**", "adrs/**"},
				Evidence: []EvidenceRule{
					evidence("adr_title_signal", 0.12, ReasonHeadingMatch, "ADR title contains decision-record language.", EvidenceMatch{
						Scope:    ScopeDocument,
						TitleAny: []string{"adr", "architecture decision", "decision record"},
					}),
					evidence("adr_path_signal", 0.08, ReasonPathHint, "ADR path contains decision-record language.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{"adr", "adrs", "decision-record"},
					}),
					evidence("adr_status_frontmatter", 0.16, ReasonFrontmatter, "ADR status is declared in frontmatter.", EvidenceMatch{
						Scope:             ScopeDocument,
						FrontmatterExists: []string{"status"},
					}),
					evidence("adr_decision_heading", 0.17, ReasonHeadingMatch, "ADR includes a decision heading.", EvidenceMatch{
						Scope:       ScopeDocument,
						HeadingsAny: []string{"decision", "decision outcome"},
					}),
					evidence("adr_context_decision_structure", 0.16, ReasonHeadingMatch, "ADR includes context and decision structure.", EvidenceMatch{
						Scope:       ScopeDocument,
						HeadingsAll: []string{"context", "decision"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("adr_changelog_marker", 0.20, ReasonChangelogMarker, "Changelog marker reduces ADR confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
					evidence("adr_generated_marker", 0.18, ReasonGeneratedMarker, "Generated marker reduces ADR confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
				},
				Subformats: map[string]SubmodelConfig{
					"nygard": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("adr_nygard_sections", 0.26, ReasonSubformatEvidence, "Nygard ADR sections are present.", EvidenceMatch{
								Scope:       ScopeDocument,
								HeadingsAll: []string{"context", "decision", "consequences"},
							}),
						},
					},
					"madr": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("adr_madr_sections", 0.26, ReasonSubformatEvidence, "MADR-style problem, options, and outcome sections are present.", EvidenceMatch{
								Scope:       ScopeDocument,
								HeadingsAll: []string{"context and problem statement", "decision outcome"},
								HeadingsAny: []string{"decision drivers", "considered options"},
							}),
						},
					},
					"y_statement": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("adr_y_statement_sentence", 0.28, ReasonSubformatEvidence, "Y-Statement decision sentence is present.", EvidenceMatch{
								Scope: ScopeDocument,
								BodyRegexAny: []string{
									`(?is)\bin the context of\b.+\bfacing\b.+\bwe decided\b.+\bto achieve\b.+\baccepting\b`,
								},
							}),
						},
					},
				},
			},
			ModelRFC: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "design",
				Authority: AuthorityDesignProposal,
				PathHints: []string{"rfcs/**", "docs/rfcs/**", "docs/proposals/**", "enhancements/**", "keps/**", "teps/**", "oseps/**"},
				Evidence: []EvidenceRule{
					evidence("rfc_title_signal", 0.12, ReasonHeadingMatch, "RFC/proposal title signal is present.", EvidenceMatch{
						Scope:    ScopeDocument,
						TitleAny: []string{"rfc", "request for comments", "proposal"},
					}),
					evidence("rfc_path_signal", 0.08, ReasonPathHint, "RFC/proposal path signal is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{"rfc", "rfcs", "proposal", "proposals"},
					}),
					evidence("rfc_enhancement_path_signal", 0.34, ReasonPathHint, "Enhancement/KEP/TEP/SIP/SHIP path signal is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{"enhancements/", "/enhancements/", "keps/", "/keps/", "teps/", "/teps/", "oseps/", "/oseps/", "ships/", "/ships/", "sips/", "/sips/", "docs/design/", "docs/proposals/"},
					}),
					evidence("rfc_governance_frontmatter_status", 0.10, ReasonStatusSignal, "Proposal-style governance frontmatter declares a status.", EvidenceMatch{
						Scope:             ScopeDocument,
						FrontmatterExists: []string{"status"},
					}),
					evidence("rfc_design_sections", 0.23, ReasonHeadingMatch, "RFC/proposal design sections are present.", EvidenceMatch{
						Scope:       ScopeDocument,
						HeadingsAny: []string{"summary", "abstract", "motivation", "proposal", "detailed design", "alternatives", "risks", "rollout", "open questions"},
					}),
					evidence("rfc_engineering_shape", 0.18, ReasonHeadingMatch, "RFC/proposal has problem and proposal structure.", EvidenceMatch{
						Scope:       ScopeDocument,
						HeadingsAll: []string{"problem", "proposal"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("rfc_changelog_marker", 0.22, ReasonChangelogMarker, "Changelog marker reduces RFC confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
					evidence("rfc_generated_marker", 0.18, ReasonGeneratedMarker, "Generated marker reduces RFC confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
				},
				Families: map[string]SubmodelConfig{
					"section_pattern": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("rfc_section_pattern_family", 0.24, ReasonFamilyEvidence, "RFC section-pattern family evidence is present.", EvidenceMatch{
								Scope:       ScopeDocument,
								HeadingsAny: []string{"summary", "abstract", "motivation", "detailed design", "alternatives", "drawbacks", "risks", "unresolved questions", "open questions"},
							}),
						},
					},
				},
			},
			ModelPRD: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "requirements",
				Subtype:   "prd",
				Authority: AuthorityProductBackground,
				PathHints: []string{"docs/prd/**", "docs/prds/**", "prd/**", "prds/**"},
				Evidence: []EvidenceRule{
					evidence("prd_frontmatter_kind", 0.18, ReasonFrontmatter, "PRD frontmatter declares requirements kind.", EvidenceMatch{
						Scope:             ScopeDocument,
						FrontmatterEquals: map[string]string{"kind": "requirements"},
					}),
					evidence("prd_frontmatter_subtype", 0.16, ReasonFrontmatter, "PRD frontmatter declares PRD subtype.", EvidenceMatch{
						Scope:             ScopeDocument,
						FrontmatterEquals: map[string]string{"subtype": "prd"},
					}),
					evidence("prd_title_signal", 0.12, ReasonHeadingMatch, "PRD title signal is present.", EvidenceMatch{
						Scope:    ScopeDocument,
						TitleAny: []string{"prd", "product requirements", "requirements"},
					}),
					evidence("prd_filename_signal", 0.22, ReasonPathHint, "PRD filename signal is present.", EvidenceMatch{
						Scope:       ScopeDocument,
						FilenameAny: []string{"prd", "product_requirements", "product-requirements", "product requirements"},
					}),
					evidence("prd_path_signal", 0.08, ReasonPathHint, "PRD path signal is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{"prd", "prds", "requirements"},
					}),
					evidence("prd_body_phrase_signal", 0.26, ReasonHeadingMatch, "PRD body includes product requirements language.", EvidenceMatch{
						Scope:           ScopeDocument,
						BodyContainsAny: []string{"product requirements document", "functional requirements", "product scope", "user personas", "success metrics"},
					}),
					evidence("prd_product_sections", 0.18, ReasonHeadingMatch, "Product requirement sections are present.", EvidenceMatch{
						Scope:           ScopeDocument,
						SectionRolesAny: []string{"product", "requirements"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("prd_changelog_marker", 0.22, ReasonChangelogMarker, "Changelog marker reduces PRD confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
					evidence("prd_generated_marker", 0.18, ReasonGeneratedMarker, "Generated marker reduces PRD confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
				},
				Families: map[string]SubmodelConfig{
					"product_intent": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("prd_product_intent_family", 0.24, ReasonFamilyEvidence, "Product-intent family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								SectionRolesAny: []string{"product", "requirements"},
								HeadingsAny:     []string{"goals", "non-goals", "user outcomes", "success metrics", "requirements", "acceptance criteria"},
							}),
						},
					},
				},
			},
			ModelPlan: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "plan",
				Authority: AuthorityWorkingPlan,
				PathHints: []string{"plans/**", "docs/plans/**"},
				Evidence: []EvidenceRule{
					evidence("plan_dated_filename", 0.12, ReasonLifecycleSignal, "Plan has dated filename or path token.", EvidenceMatch{
						Scope:         ScopeDocument,
						DateTokensMin: 1,
					}),
					evidence("plan_title_signal", 0.12, ReasonHeadingMatch, "Plan title signal is present.", EvidenceMatch{
						Scope:    ScopeDocument,
						TitleAny: []string{"plan", "implementation", "migration", "rollout"},
					}),
					evidence("plan_path_signal", 0.08, ReasonPathHint, "Plan path signal is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{"plan", "plans"},
					}),
					evidence("plan_checklist", 0.15, ReasonHeadingMatch, "Plan includes checklist/task items.", EvidenceMatch{
						Scope:        ScopeDocument,
						ChecklistMin: 1,
					}),
					evidence("plan_work_sections", 0.16, ReasonHeadingMatch, "Plan includes implementation, task, risk, or open-question sections.", EvidenceMatch{
						Scope:           ScopeDocument,
						SectionRolesAny: []string{"tasks", "risk", "open_questions"},
						HeadingsAny:     []string{"implementation", "phases", "tasks", "risks", "open questions", "deferred"},
					}),
					evidence("plan_work_language", 0.12, ReasonHeadingMatch, "Plan includes step, sequence, phase, or task language.", EvidenceMatch{
						Scope:           ScopeDocument,
						BodyContainsAny: []string{"step", "sequence", "phase", "task", "todo", "next"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("plan_generated_marker", 0.24, ReasonGeneratedMarker, "Generated marker reduces plan confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
					evidence("plan_changelog_marker", 0.22, ReasonChangelogMarker, "Changelog marker reduces plan confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
				},
				Families: map[string]SubmodelConfig{
					"implementation_plan": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("plan_implementation_family", 0.24, ReasonFamilyEvidence, "Implementation-plan family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								HeadingsAny:     []string{"implementation", "phases", "tasks", "next steps", "plan"},
								SectionRolesAny: []string{"tasks"},
							}),
						},
					},
					"migration_plan": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("plan_migration_family", 0.22, ReasonFamilyEvidence, "Migration-plan family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								TitleAny:        []string{"migration"},
								HeadingsAny:     []string{"migration", "rollout", "backfill", "compatibility"},
								BodyContainsAny: []string{"migration", "backfill"},
							}),
						},
					},
					"rollout_plan": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("plan_rollout_family", 0.22, ReasonFamilyEvidence, "Rollout-plan family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								TitleAny:        []string{"rollout"},
								HeadingsAny:     []string{"rollout", "launch", "monitoring"},
								BodyContainsAny: []string{"rollout", "launch"},
							}),
						},
					},
				},
			},
			ModelAgentNote: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "plan",
				Authority: AuthorityHandoffNote,
				PathHints: []string{".cursor/plans/**", ".claude/**", ".codex/**"},
				Evidence: []EvidenceRule{
					evidence("agent_note_path_signal", 0.18, ReasonPathHint, "Agent-note path signal is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						PathContainsAny: []string{".cursor", ".claude", ".codex"},
					}),
					evidence("agent_note_frontmatter_signal", 0.12, ReasonFrontmatter, "Agent-note frontmatter signal is present.", EvidenceMatch{
						Scope:             ScopeDocument,
						FrontmatterExists: []string{"agent", "tool", "source", "generator"},
					}),
					evidence("agent_note_handoff_terms", 0.20, ReasonHeadingMatch, "Agent handoff or continuation language is present.", EvidenceMatch{
						Scope:           ScopeDocument,
						BodyContainsAny: []string{"follow-up", "follow up", "handoff", "resume", "next step", "blocked", "blocker", "stopped after"},
					}),
				},
				NegativeEvidence: []EvidenceRule{
					evidence("agent_note_generated_marker", 0.18, ReasonGeneratedMarker, "Generated marker reduces agent-note confidence.", EvidenceMatch{MarkersAny: []string{MarkerGenerated}}),
					evidence("agent_note_changelog_marker", 0.16, ReasonChangelogMarker, "Changelog marker reduces agent-note confidence.", EvidenceMatch{MarkersAny: []string{MarkerChangelog}}),
				},
				Families: map[string]SubmodelConfig{
					"continuation_note": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("agent_continuation_family", 0.24, ReasonFamilyEvidence, "Continuation-note family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								BodyContainsAny: []string{"resume", "continue", "handoff", "stopped after", "next step"},
							}),
						},
					},
					"followup_note": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("agent_followup_family", 0.30, ReasonFamilyEvidence, "Follow-up-note family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								TitleAny:        []string{"followup", "follow-up", "follow up"},
								BodyContainsAny: []string{"follow-up", "follow up", "followup"},
							}),
						},
					},
					"blocker_note": {
						Enabled: true,
						Evidence: []EvidenceRule{
							evidence("agent_blocker_family", 0.24, ReasonFamilyEvidence, "Blocker-note family evidence is present.", EvidenceMatch{
								Scope:           ScopeDocument,
								TitleAny:        []string{"blocker", "blocked"},
								BodyContainsAny: []string{"blocked", "blocker", "cannot proceed"},
							}),
						},
					},
				},
			},
			ModelGenericMarkdown: {
				Enabled:   true,
				Scopes:    []Scope{ScopeDocument},
				Kind:      "markdown_artifact",
				Authority: AuthorityNeutral,
				Fallback:  true,
				Evidence: []EvidenceRule{
					evidence("generic_markdown_fallback", 0.40, ReasonFallback, "Generic markdown fallback for useful text without a stronger model match.", EvidenceMatch{
						Scope:  ScopeDocument,
						Always: true,
					}),
				},
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
		if err := validateRuleSet(id, "evidence", model.Evidence); err != nil {
			return err
		}
		if err := validateRuleSet(id, "negative_evidence", model.NegativeEvidence); err != nil {
			return err
		}
		for subID, sub := range model.Subformats {
			if err := validateRuleSet(id+"."+subID, "evidence", sub.Evidence); err != nil {
				return err
			}
			if err := validateRuleSet(id+"."+subID, "negative_evidence", sub.NegativeEvidence); err != nil {
				return err
			}
		}
		for familyID, family := range model.Families {
			if err := validateRuleSet(id+"."+familyID, "evidence", family.Evidence); err != nil {
				return err
			}
			if err := validateRuleSet(id+"."+familyID, "negative_evidence", family.NegativeEvidence); err != nil {
				return err
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
		if err := validateRuleSet(local.ID, "evidence", local.Evidence); err != nil {
			return err
		}
		if err := validateRuleSet(local.ID, "negative_evidence", local.NegativeEvidence); err != nil {
			return err
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

func validateRuleSet(owner, field string, rules []EvidenceRule) error {
	for _, rule := range rules {
		if rule.ID == "" {
			return fmt.Errorf("classifier model %q %s rule id is required", owner, field)
		}
		if rule.Weight < 0 || rule.Weight > 1 {
			return fmt.Errorf("classifier model %q %s rule %q weight must be between 0 and 1", owner, field, rule.ID)
		}
		if rule.Match.Scope != "" {
			if err := ValidateScope(rule.Match.Scope); err != nil {
				return fmt.Errorf("classifier model %q %s rule %q: %w", owner, field, rule.ID, err)
			}
		}
	}
	return nil
}
