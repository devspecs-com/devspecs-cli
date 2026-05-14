// Package classify defines deterministic document-classification contracts.
//
// The package intentionally stays pure: it does not scan, index, or affect
// retrieval behavior until later integration phases opt into these contracts.
package classify

import "fmt"

type Scope string

const (
	ScopeContainer Scope = "container"
	ScopeDocument  Scope = "document"
)

func (s Scope) Valid() bool {
	return s == ScopeContainer || s == ScopeDocument
}

type Candidate struct {
	Path        string       `json:"path" yaml:"path"`
	Scope       Scope        `json:"scope" yaml:"scope"`
	Role        string       `json:"role,omitempty" yaml:"role,omitempty"`
	Ext         string       `json:"ext,omitempty" yaml:"ext,omitempty"`
	SizeBytes   int64        `json:"size_bytes,omitempty" yaml:"size_bytes,omitempty"`
	Body        string       `json:"-" yaml:"-"`
	Features    Features     `json:"features,omitempty" yaml:"features,omitempty"`
	SourceHints []SourceHint `json:"source_hints,omitempty" yaml:"source_hints,omitempty"`
}

type Features struct {
	PathTokens         []string          `json:"path_tokens,omitempty" yaml:"path_tokens,omitempty"`
	FilenameTokens     []string          `json:"filename_tokens,omitempty" yaml:"filename_tokens,omitempty"`
	DateTokens         []string          `json:"date_tokens,omitempty" yaml:"date_tokens,omitempty"`
	Frontmatter        map[string]string `json:"frontmatter,omitempty" yaml:"frontmatter,omitempty"`
	Title              string            `json:"title,omitempty" yaml:"title,omitempty"`
	Headings           []Heading         `json:"headings,omitempty" yaml:"headings,omitempty"`
	Sections           []Section         `json:"sections,omitempty" yaml:"sections,omitempty"`
	ChecklistItems     int               `json:"checklist_items,omitempty" yaml:"checklist_items,omitempty"`
	StatusPhrases      []string          `json:"status_phrases,omitempty" yaml:"status_phrases,omitempty"`
	LifecyclePhrases   []string          `json:"lifecycle_phrases,omitempty" yaml:"lifecycle_phrases,omitempty"`
	Identifiers        []string          `json:"identifiers,omitempty" yaml:"identifiers,omitempty"`
	PathReferences     []string          `json:"path_references,omitempty" yaml:"path_references,omitempty"`
	LinkTargets        []string          `json:"link_targets,omitempty" yaml:"link_targets,omitempty"`
	CodeFenceLanguages []string          `json:"code_fence_languages,omitempty" yaml:"code_fence_languages,omitempty"`
	LocalTerms         []string          `json:"local_terms,omitempty" yaml:"local_terms,omitempty"`
	Markers            []string          `json:"markers,omitempty" yaml:"markers,omitempty"`
}

type Heading struct {
	Level int    `json:"level" yaml:"level"`
	Text  string `json:"text" yaml:"text"`
	Line  int    `json:"line,omitempty" yaml:"line,omitempty"`
}

type Section struct {
	Heading   string `json:"heading,omitempty" yaml:"heading,omitempty"`
	Role      string `json:"role,omitempty" yaml:"role,omitempty"`
	StartLine int    `json:"start_line,omitempty" yaml:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty" yaml:"end_line,omitempty"`
}

type SourceHint struct {
	Kind       string  `json:"kind" yaml:"kind"`
	Value      string  `json:"value" yaml:"value"`
	Confidence float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`
}

type Classifier interface {
	Name() string
	Classify(Candidate) Classification
}

type Classification struct {
	Classifier      string         `json:"classifier" yaml:"classifier"`
	Scope           Scope          `json:"scope" yaml:"scope"`
	Subformat       string         `json:"subformat,omitempty" yaml:"subformat,omitempty"`
	Family          string         `json:"family,omitempty" yaml:"family,omitempty"`
	Accepted        bool           `json:"accepted" yaml:"accepted"`
	Confidence      float64        `json:"confidence" yaml:"confidence"`
	Kind            string         `json:"kind,omitempty" yaml:"kind,omitempty"`
	Subtype         string         `json:"subtype,omitempty" yaml:"subtype,omitempty"`
	Status          string         `json:"status,omitempty" yaml:"status,omitempty"`
	Lifecycle       string         `json:"lifecycle,omitempty" yaml:"lifecycle,omitempty"`
	Authority       string         `json:"authority,omitempty" yaml:"authority,omitempty"`
	FormatProfile   string         `json:"format_profile,omitempty" yaml:"format_profile,omitempty"`
	LayoutGroup     string         `json:"layout_group,omitempty" yaml:"layout_group,omitempty"`
	PositiveReasons []Reason       `json:"positive_reasons,omitempty" yaml:"positive_reasons,omitempty"`
	NegativeReasons []Reason       `json:"negative_reasons,omitempty" yaml:"negative_reasons,omitempty"`
	ChildCandidates []Candidate    `json:"child_candidates,omitempty" yaml:"child_candidates,omitempty"`
	Extracted       map[string]any `json:"extracted,omitempty" yaml:"extracted,omitempty"`
}

type Resolution struct {
	Winner          Classification   `json:"winner" yaml:"winner"`
	Alternatives    []Classification `json:"alternatives,omitempty" yaml:"alternatives,omitempty"`
	Ambiguous       bool             `json:"ambiguous" yaml:"ambiguous"`
	FallbackGeneric bool             `json:"fallback_generic" yaml:"fallback_generic"`
}

type ReasonPolarity string

const (
	ReasonPositive ReasonPolarity = "positive"
	ReasonNegative ReasonPolarity = "negative"
)

type ReasonCode string

const (
	ReasonPathHint          ReasonCode = "path_hint"
	ReasonLayoutMatch       ReasonCode = "layout_match"
	ReasonHeadingMatch      ReasonCode = "heading_match"
	ReasonFrontmatter       ReasonCode = "frontmatter"
	ReasonStatusSignal      ReasonCode = "status_signal"
	ReasonLifecycleSignal   ReasonCode = "lifecycle_signal"
	ReasonSubformatEvidence ReasonCode = "subformat_evidence"
	ReasonFamilyEvidence    ReasonCode = "family_evidence"
	ReasonIdentifierSignal  ReasonCode = "identifier_signal"
	ReasonContainerChild    ReasonCode = "container_child"
	ReasonLocalOverride     ReasonCode = "local_override"
	ReasonGeneratedMarker   ReasonCode = "generated_marker"
	ReasonChangelogMarker   ReasonCode = "changelog_marker"
	ReasonVendoredMarker    ReasonCode = "vendored_marker"
	ReasonAmbiguous         ReasonCode = "ambiguous"
	ReasonFallback          ReasonCode = "fallback"
)

type Reason struct {
	Code     ReasonCode     `json:"code" yaml:"code"`
	Polarity ReasonPolarity `json:"polarity" yaml:"polarity"`
	Message  string         `json:"message" yaml:"message"`
	Evidence string         `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

func ReasonVocabulary() []ReasonCode {
	return []ReasonCode{
		ReasonPathHint,
		ReasonLayoutMatch,
		ReasonHeadingMatch,
		ReasonFrontmatter,
		ReasonStatusSignal,
		ReasonLifecycleSignal,
		ReasonSubformatEvidence,
		ReasonFamilyEvidence,
		ReasonIdentifierSignal,
		ReasonContainerChild,
		ReasonLocalOverride,
		ReasonGeneratedMarker,
		ReasonChangelogMarker,
		ReasonVendoredMarker,
		ReasonAmbiguous,
		ReasonFallback,
	}
}

func ValidateScope(scope Scope) error {
	if !scope.Valid() {
		return fmt.Errorf("invalid classifier scope %q", scope)
	}
	return nil
}
