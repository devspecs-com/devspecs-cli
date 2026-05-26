package evalharness

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/openspecmetrics"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	CorpusSourceFilesystemFixture = "filesystem_fixture"
	CorpusSourceSQLiteIndex       = "sqlite_index"
	ProductPathLabOnly            = "lab_only"
	ProductPathIndexedHarness     = "indexed_harness"
	ProductPathLiveCLICommand     = "live_cli_command"
)

type TokenCounter interface {
	Count(text string) int
	Name() string
}

type PricingProfile struct {
	Name              string  `json:"name"`
	InputUSDPer1MTok  float64 `json:"input_usd_per_1m_tokens,omitempty"`
	OutputUSDPer1MTok float64 `json:"output_usd_per_1m_tokens,omitempty"`
}

type TokenizerProfile struct {
	Name          string         `json:"name"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model,omitempty"`
	Approximation string         `json:"approximation,omitempty"`
	Pricing       PricingProfile `json:"pricing,omitempty"`
}

type ProfiledTokenCounter interface {
	TokenCounter
	Profile() TokenizerProfile
}

type ApproxTokenCounter struct{}

func (ApproxTokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func (ApproxTokenCounter) Name() string { return "approx_chars_div_4" }

func (ApproxTokenCounter) Profile() TokenizerProfile {
	return TokenizerProfile{
		Name:          "approx_chars_div_4",
		Provider:      "deterministic",
		Approximation: "ceil(chars / 4.0)",
		Pricing: PricingProfile{
			Name: "none",
		},
	}
}

type Options struct {
	JSON                            bool
	MinRecall                       *float64
	MinMeanRecall                   *float64
	MinMustRecall                   *float64
	MinSufficiency                  *float64
	MinReductionFull                *float64
	CorpusSource                    string
	CommandUnderTest                string
	FindRuntime                     string
	CommandRunner                   CommandRunner
	TokenCounter                    TokenCounter
	Retriever                       retrieval.Retriever
	TestCaseArtifacts               bool
	CodeCommentArtifacts            bool
	DisableSectionAwareRetrieval    bool
	ExperimentalBalancedEvidence    bool
	ExperimentalBudgetedPacking     bool
	ExperimentalConceptBackfill     bool
	ExperimentalGlossaryConcepts    bool
	ExperimentalTieredConceptOutput bool
	ExperimentalAnchorFirstRanking  bool
	ExperimentalAnchorFirstMode     string
	PackDiagnostics                 bool
	ContextTokenBudget              int
	IndexCacheDir                   string
	RefreshIndexCache               bool
	MaxCorpusFiles                  int
	MaxSourceFiles                  int
	MaxTestCaseArtifacts            int
	MaxCodeComments                 int
	MaxCaseSeconds                  int
	ProgressWriter                  io.Writer
	ProgressInterval                time.Duration
}

type CommandRunner func(fixtureAbs string, cases []CaseSpec) (map[string]CommandCaseOutput, error)

type CommandCaseOutput struct {
	Artifacts       []retrieval.Candidate
	Context         string
	ArtifactReasons []ArtifactReason
}

type CaseFile struct {
	FixtureVersion string     `yaml:"fixture_version"`
	EvalStage      string     `yaml:"eval_stage"`
	Cases          []CaseSpec `yaml:"cases"`
}

type CaseSpec struct {
	ID               string             `yaml:"id" json:"id"`
	Query            string             `yaml:"query" json:"query"`
	ExpectedRelevant []ExpectedArtifact `yaml:"expected_relevant" json:"expected_relevant"`
	ExpectedExcluded []string           `yaml:"expected_excluded" json:"expected_excluded"`
	ExpectedStatus   map[string]string  `yaml:"expected_status" json:"expected_status,omitempty"`
	SuccessCriteria  SuccessCriteria    `yaml:"success_criteria" json:"success_criteria,omitempty"`
}

type ExpectedArtifact struct {
	Path       string `yaml:"path" json:"path"`
	Importance string `yaml:"importance" json:"importance"`
}

func (a *ExpectedArtifact) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		a.Path = filepath.ToSlash(strings.TrimSpace(value.Value))
		a.Importance = "must"
		return nil
	case yaml.MappingNode:
		var aux struct {
			Path       string `yaml:"path"`
			Importance string `yaml:"importance"`
		}
		if err := value.Decode(&aux); err != nil {
			return err
		}
		a.Path = filepath.ToSlash(strings.TrimSpace(aux.Path))
		a.Importance = defaultString(aux.Importance, "must")
		return nil
	default:
		return fmt.Errorf("expected_relevant entry must be a path string or mapping")
	}
}

type SuccessCriteria struct {
	MustContainTerms        []string `yaml:"must_contain_terms" json:"must_contain_terms,omitempty"`
	MustContainArtifacts    []string `yaml:"must_contain_artifacts" json:"must_contain_artifacts,omitempty"`
	MustNotContainTerms     []string `yaml:"must_not_contain_terms" json:"must_not_contain_terms,omitempty"`
	MustNotContainArtifacts []string `yaml:"must_not_contain_artifacts" json:"must_not_contain_artifacts,omitempty"`
	LegacyMustNotContain    []string `yaml:"must_not_contain" json:"must_not_contain,omitempty"`
}

func (c SuccessCriteria) Configured() bool {
	return len(c.MustContainTerms) > 0 ||
		len(c.MustContainArtifacts) > 0 ||
		len(c.MustNotContainTerms) > 0 ||
		len(c.MustNotContainArtifacts) > 0 ||
		len(c.LegacyMustNotContain) > 0
}

type BaselineMetrics struct {
	Name                     string   `json:"name"`
	FileScope                string   `json:"file_scope"`
	IncludesSourceCandidates bool     `json:"includes_source_candidates"`
	Tokens                   int      `json:"tokens"`
	ArtifactCount            int      `json:"artifact_count"`
	Artifacts                []string `json:"artifacts"`
	RelevantIncluded         int      `json:"relevant_included"`
	IrrelevantCount          int      `json:"irrelevant_count"`
}

type Diagnostics struct {
	ExpectedRelevantCount          int                           `json:"expected_relevant_count"`
	ExpectedAvailableCount         int                           `json:"expected_available_count"`
	ExpectedMissingFromCorpusCount int                           `json:"expected_missing_from_corpus_count"`
	MissedAfterDiscoveryCount      int                           `json:"missed_after_discovery_count"`
	DiscoveryCoverage              float64                       `json:"discovery_coverage"`
	RetrievalCoverageOfDiscovered  float64                       `json:"retrieval_coverage_of_discovered"`
	ExpectedMissingFromCorpus      []string                      `json:"expected_missing_from_corpus,omitempty"`
	MissedAfterDiscovery           []string                      `json:"missed_after_discovery,omitempty"`
	RoleSummaries                  []RoleDiagnostic              `json:"role_summaries,omitempty"`
	MissClassSummaries             []MissClassDiagnostic         `json:"miss_class_summaries,omitempty"`
	FalsePositiveSummaries         []FalsePositiveDiagnostic     `json:"false_positive_summaries,omitempty"`
	ExtensionSummaries             []ExtensionDiagnostic         `json:"extension_summaries,omitempty"`
	UnindexedDocumentSummaries     []UnindexedDocumentDiagnostic `json:"unindexed_document_summaries,omitempty"`
	OpenSpec                       *openspecmetrics.Metrics      `json:"openspec,omitempty"`
}

type RoleDiagnostic struct {
	Role                          string  `json:"role"`
	Expected                      int     `json:"expected"`
	ExpectedAvailable             int     `json:"expected_available"`
	Retrieved                     int     `json:"retrieved"`
	IrrelevantRetrieved           int     `json:"irrelevant_retrieved"`
	MissingFromCorpus             int     `json:"missing_from_corpus"`
	MissedAfterDiscovery          int     `json:"missed_after_discovery"`
	DiscoveryCoverage             float64 `json:"discovery_coverage"`
	RetrievalCoverageOfDiscovered float64 `json:"retrieval_coverage_of_discovered"`
}

type MissClassDiagnostic struct {
	Class    string   `json:"class"`
	Count    int      `json:"count"`
	Examples []string `json:"examples,omitempty"`
}

type FalsePositiveDiagnostic struct {
	Class       string                 `json:"class"`
	QueryType   string                 `json:"query_type"`
	Lane        string                 `json:"lane"`
	Role        string                 `json:"role"`
	ReasonClass string                 `json:"reason_class"`
	GradeCounts GradeCounts            `json:"grade_counts"`
	Count       int                    `json:"count"`
	Examples    []FalsePositiveExample `json:"examples,omitempty"`
}

type FalsePositiveExample struct {
	CaseID      string   `json:"case_id"`
	QueryType   string   `json:"query_type"`
	Path        string   `json:"path"`
	Position    int      `json:"position"`
	Lane        string   `json:"lane"`
	Role        string   `json:"role"`
	Grade       string   `json:"grade"`
	Weight      float64  `json:"weight"`
	ReasonClass string   `json:"reason_class"`
	Reasons     []string `json:"reasons,omitempty"`
}

type ExtensionDiagnostic struct {
	Extension                  string      `json:"extension"`
	Role                       string      `json:"role"`
	Expected                   int         `json:"expected"`
	ExactRetrieved             int         `json:"exact_retrieved"`
	MissingFromCorpus          int         `json:"missing_from_corpus"`
	MissedAfterDiscovery       int         `json:"missed_after_discovery"`
	PrimaryFalsePositive       int         `json:"primary_false_positive"`
	PrimaryFalsePositiveGrades GradeCounts `json:"primary_false_positive_grades,omitempty"`
	Examples                   []string    `json:"examples,omitempty"`
}

type UnindexedDocumentDiagnostic struct {
	Extension string   `json:"extension"`
	Role      string   `json:"role"`
	Count     int      `json:"count"`
	Examples  []string `json:"examples,omitempty"`
}

type CaseResult struct {
	ID                               string                     `json:"id"`
	Query                            string                     `json:"query"`
	CaseDurationMS                   int64                      `json:"case_duration_ms,omitempty"`
	CaseBudgetExceeded               bool                       `json:"case_budget_exceeded,omitempty"`
	CaseBudgetSeconds                int                        `json:"case_budget_seconds,omitempty"`
	DevSpecsTokens                   int                        `json:"devspecs_tokens"`
	FullPlanningTokens               int                        `json:"full_planning_tokens"`
	AllMarkdownTokens                int                        `json:"all_markdown_tokens"`
	FullCandidateCorpusTokens        int                        `json:"full_candidate_corpus_tokens"`
	QueryFileBaselineTokens          int                        `json:"query_file_baseline_tokens"`
	PreBudgetDevSpecsTokens          int                        `json:"pre_budget_devspecs_tokens,omitempty"`
	ContextTokenBudget               int                        `json:"context_token_budget,omitempty"`
	ContextBudgetDroppedCount        int                        `json:"context_budget_dropped_count,omitempty"`
	ContextBudgetDroppedArtifacts    []string                   `json:"context_budget_dropped_artifacts,omitempty"`
	TokenReductionVsFullPlanning     float64                    `json:"token_reduction_vs_full_planning"`
	TokenReductionVsAllMarkdown      float64                    `json:"token_reduction_vs_all_markdown"`
	TokenReductionVsFullCandidate    float64                    `json:"token_reduction_vs_full_candidate_corpus"`
	TokenReductionVsQueryFile        float64                    `json:"token_reduction_vs_query_file_baseline"`
	ExpectedRelevantCount            int                        `json:"expected_relevant_count"`
	RelevantRetrieved                int                        `json:"relevant_retrieved"`
	ArtifactRecall                   float64                    `json:"artifact_recall"`
	MustExpectedCount                int                        `json:"must_expected_count"`
	MustRelevantRetrieved            int                        `json:"must_relevant_retrieved"`
	MustHaveRecall                   float64                    `json:"must_have_recall"`
	HelpfulExpectedCount             int                        `json:"helpful_expected_count"`
	HelpfulRelevantRetrieved         int                        `json:"helpful_relevant_retrieved"`
	HelpfulRecall                    float64                    `json:"helpful_recall"`
	BackgroundExpectedCount          int                        `json:"background_expected_count"`
	BackgroundRelevantRetrieved      int                        `json:"background_relevant_retrieved"`
	BackgroundRecall                 float64                    `json:"background_recall"`
	ArtifactsIncluded                []string                   `json:"artifacts_included"`
	ArtifactReasons                  []ArtifactReason           `json:"artifact_reasons"`
	PackDiagnostics                  *retrieval.RoleGroupedPack `json:"pack_diagnostics,omitempty"`
	RelatedArtifacts                 []string                   `json:"related_artifacts,omitempty"`
	RelatedArtifactReasons           []ArtifactReason           `json:"related_artifact_reasons,omitempty"`
	RelatedDevSpecsTokens            int                        `json:"related_devspecs_tokens,omitempty"`
	RelatedRelevantIncluded          []string                   `json:"related_relevant_included,omitempty"`
	RelatedIrrelevantIncluded        []string                   `json:"related_irrelevant_included,omitempty"`
	RelatedArtifactPrecision         float64                    `json:"related_artifact_precision,omitempty"`
	RelatedAgentMetrics              CaseAgentMetrics           `json:"related_agent_metrics,omitempty"`
	RelatedArtifactGrades            []ArtifactGrade            `json:"related_artifact_grades,omitempty"`
	CombinedTieredArtifacts          []string                   `json:"combined_tiered_artifacts,omitempty"`
	CombinedTieredDevSpecsTokens     int                        `json:"combined_tiered_devspecs_tokens,omitempty"`
	CombinedTieredContextSufficiency SufficiencyResult          `json:"combined_tiered_context_sufficiency,omitempty"`
	PackedSectionArtifacts           []string                   `json:"packed_section_artifacts,omitempty"`
	PackedSectionCount               int                        `json:"packed_section_count,omitempty"`
	SectionSelectedArtifacts         []string                   `json:"section_selected_artifacts,omitempty"`
	SectionSelectedCount             int                        `json:"section_selected_count,omitempty"`
	FullFileArtifactCount            int                        `json:"full_file_artifact_count,omitempty"`
	TestCaseArtifactCount            int                        `json:"test_case_artifact_count,omitempty"`
	CodeCommentArtifactCount         int                        `json:"code_comment_artifact_count,omitempty"`
	RelevantIncluded                 []string                   `json:"relevant_included"`
	IrrelevantIncluded               []string                   `json:"irrelevant_included"`
	ArtifactPrecision                float64                    `json:"artifact_precision"`
	MissedExpectedRelevant           []string                   `json:"missed_expected_relevant"`
	MissedMustConceptDiagnostics     []ConceptMissDiagnostic    `json:"missed_must_concept_diagnostics,omitempty"`
	PrimaryFalsePositiveDiagnostics  []FalsePositiveExample     `json:"primary_false_positive_diagnostics,omitempty"`
	UnexpectedExcludedHits           []string                   `json:"unexpected_excluded_hits"`
	ExpectedAvailableCount           int                        `json:"expected_available_count"`
	ExpectedMissingFromCorpus        []string                   `json:"expected_missing_from_corpus,omitempty"`
	MissedAfterDiscovery             []string                   `json:"missed_after_discovery,omitempty"`
	DiscoveryCoverage                float64                    `json:"discovery_coverage"`
	RetrievalCoverageOfDiscovered    float64                    `json:"retrieval_coverage_of_discovered"`
	ContextSufficiency               SufficiencyResult          `json:"context_sufficiency"`
	AgentMetrics                     CaseAgentMetrics           `json:"agent_metrics"`
	ArtifactGrades                   []ArtifactGrade            `json:"artifact_grades,omitempty"`
	Baselines                        []BaselineMetrics          `json:"baselines"`
	ThresholdFailures                []string                   `json:"threshold_failures,omitempty"`
}

type ConceptMissDiagnostic struct {
	ExpectedPath     string   `json:"expected_path"`
	InCandidatePool  bool     `json:"in_candidate_pool"`
	ConceptRank      int      `json:"concept_rank,omitempty"`
	ConceptScore     float64  `json:"concept_score,omitempty"`
	MatchedCompacts  []string `json:"matched_compacts,omitempty"`
	MatchedPhrases   []string `json:"matched_phrases,omitempty"`
	MatchedPathTerms []string `json:"matched_path_terms,omitempty"`
	GlossaryMatches  []string `json:"glossary_matches,omitempty"`
	GlossaryEvidence []string `json:"glossary_evidence,omitempty"`
}

type Summary struct {
	Cases                                    int           `json:"cases"`
	MedianTokenReductionVsFullPlanning       float64       `json:"median_token_reduction_vs_full_planning"`
	MeanTokenReductionVsFullPlanning         float64       `json:"mean_token_reduction_vs_full_planning"`
	MedianTokenReductionVsQueryFileBaseline  float64       `json:"median_token_reduction_vs_query_file_baseline"`
	MeanTokenReductionVsQueryFileBaseline    float64       `json:"mean_token_reduction_vs_query_file_baseline"`
	MeanArtifactRecall                       float64       `json:"mean_artifact_recall"`
	MeanMustHaveRecall                       float64       `json:"mean_must_have_recall"`
	MeanHelpfulRecall                        float64       `json:"mean_helpful_recall"`
	MeanBackgroundRecall                     float64       `json:"mean_background_recall"`
	MeanArtifactPrecision                    float64       `json:"mean_artifact_precision"`
	MeanGradedPrecision                      float64       `json:"mean_graded_precision"`
	MeanPenalizedUtilityPrecision            float64       `json:"mean_penalized_utility_precision"`
	GradeCounts                              GradeCounts   `json:"grade_counts"`
	RelatedCases                             int           `json:"related_cases,omitempty"`
	RelatedArtifactCount                     int           `json:"related_artifact_count,omitempty"`
	RelatedRelevantCount                     int           `json:"related_relevant_count,omitempty"`
	MeanRelatedArtifactPrecision             float64       `json:"mean_related_artifact_precision,omitempty"`
	MeanRelatedGradedPrecision               float64       `json:"mean_related_graded_precision,omitempty"`
	RelatedGradeCounts                       GradeCounts   `json:"related_grade_counts,omitempty"`
	CombinedTieredContextSufficiencyCases    int           `json:"combined_tiered_context_sufficiency_cases,omitempty"`
	CombinedTieredContextSufficiencyPassed   int           `json:"combined_tiered_context_sufficiency_passed,omitempty"`
	CombinedTieredContextSufficiencyPassRate float64       `json:"combined_tiered_context_sufficiency_pass_rate,omitempty"`
	ContextSufficiencyCases                  int           `json:"context_sufficiency_cases"`
	ContextSufficiencyPassed                 int           `json:"context_sufficiency_passed"`
	ContextSufficiencyPassRate               float64       `json:"context_sufficiency_pass_rate"`
	AgentMetrics                             AgentMetrics  `json:"agent_metrics"`
	Pareto                                   ParetoSummary `json:"pareto"`
	WorstRecallCase                          string        `json:"worst_recall_case"`
	LargestTokenContextCase                  string        `json:"largest_token_context_case"`
	FailedThresholdCount                     int           `json:"failed_threshold_count,omitempty"`
}

type ParetoSummary struct {
	MeanTokenReductionVsFullPlanning      float64 `json:"mean_token_reduction_vs_full_planning"`
	MeanTokenReductionVsQueryFileBaseline float64 `json:"mean_token_reduction_vs_query_file_baseline"`
	MeanArtifactRecall                    float64 `json:"mean_artifact_recall"`
	MeanMustHaveRecall                    float64 `json:"mean_must_have_recall"`
	MeanArtifactPrecision                 float64 `json:"mean_artifact_precision"`
	MeanGradedPrecision                   float64 `json:"mean_graded_precision"`
	MeanPenalizedUtilityPrecision         float64 `json:"mean_penalized_utility_precision"`
	ContextSufficiencyPassRate            float64 `json:"context_sufficiency_pass_rate"`
}

type ArtifactReason = retrieval.Reason

type SufficiencyResult struct {
	Configured                bool     `json:"configured"`
	Passed                    bool     `json:"passed"`
	MissingTerms              []string `json:"missing_terms"`
	MissingArtifacts          []string `json:"missing_artifacts"`
	ForbiddenTermsPresent     []string `json:"forbidden_terms_present"`
	ForbiddenArtifactsPresent []string `json:"forbidden_artifacts_present"`
	Failures                  []string `json:"failures"`
}

type Result struct {
	Fixture          string            `json:"fixture"`
	FixtureVersion   string            `json:"fixture_version"`
	EvalStage        string            `json:"eval_stage"`
	CorpusSource     string            `json:"corpus_source"`
	ProductPath      string            `json:"product_path"`
	CommandUnderTest string            `json:"command_under_test,omitempty"`
	FindRuntime      string            `json:"find_runtime,omitempty"`
	Retriever        string            `json:"retriever"`
	TokenCounter     string            `json:"token_counter"`
	TokenizerProfile TokenizerProfile  `json:"tokenizer_profile"`
	PricingProfile   PricingProfile    `json:"pricing_profile"`
	ResultsFile      string            `json:"results_file,omitempty"`
	Corpus           CorpusSummary     `json:"corpus"`
	Summary          Summary           `json:"summary"`
	Diagnostics      Diagnostics       `json:"diagnostics"`
	AgentMetrics     AgentMetrics      `json:"agent_metrics"`
	LaneMetrics      []LaneMetric      `json:"lane_metrics"`
	MetricNotes      map[string]string `json:"metric_notes,omitempty"`
	PhaseTelemetry   []PhaseTelemetry  `json:"phase_telemetry,omitempty"`
	IndexCache       *IndexCacheReport `json:"index_cache,omitempty"`
	Budgets          BudgetReport      `json:"budgets,omitempty"`
	Cases            []CaseResult      `json:"cases"`
}

type PhaseTelemetry struct {
	Name       string            `json:"name"`
	StartedAt  string            `json:"started_at"`
	EndedAt    string            `json:"ended_at"`
	DurationMS int64             `json:"duration_ms"`
	Status     string            `json:"status"`
	Counts     map[string]int    `json:"counts,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

type IndexCacheReport struct {
	Enabled               bool   `json:"enabled"`
	Hit                   bool   `json:"hit"`
	Key                   string `json:"key,omitempty"`
	Path                  string `json:"path,omitempty"`
	SchemaVersion         int    `json:"schema_version,omitempty"`
	Reason                string `json:"reason,omitempty"`
	CorpusFingerprint     string `json:"corpus_fingerprint,omitempty"`
	ProvenanceFingerprint string `json:"provenance_fingerprint,omitempty"`
}

type BudgetReport struct {
	MaxCorpusFiles       int           `json:"max_corpus_files,omitempty"`
	MaxSourceFiles       int           `json:"max_source_files,omitempty"`
	MaxTestCaseArtifacts int           `json:"max_test_case_artifacts,omitempty"`
	MaxCodeComments      int           `json:"max_code_comments,omitempty"`
	MaxCaseSeconds       int           `json:"max_case_seconds,omitempty"`
	Applied              []BudgetEvent `json:"applied,omitempty"`
}

type BudgetEvent struct {
	Name    string `json:"name"`
	Before  int    `json:"before"`
	After   int    `json:"after"`
	Message string `json:"message,omitempty"`
}

type CorpusSummary struct {
	PlanningArtifacts       CorpusSlice `json:"planning_artifacts"`
	MarkdownFiles           CorpusSlice `json:"markdown_files"`
	SourceContextCandidates CorpusSlice `json:"source_context_candidates"`
	FullCandidateCorpus     CorpusSlice `json:"full_candidate_corpus"`
}

type CorpusSlice struct {
	FileScope                string   `json:"file_scope"`
	IncludesSourceCandidates bool     `json:"includes_source_candidates"`
	Files                    int      `json:"files"`
	Tokens                   int      `json:"tokens"`
	Artifacts                []string `json:"artifacts,omitempty"`
}

func Run(fixture string, opts Options) (*Result, error) {
	telemetry := newPhaseRecorder()
	counter := opts.TokenCounter
	if counter == nil {
		counter = ApproxTokenCounter{}
	}
	retriever := opts.Retriever
	if retriever == nil {
		evidenceMode := ""
		if opts.ExperimentalBalancedEvidence {
			evidenceMode = retrieval.EvidenceModeBalanced
		}
		retriever = retrieval.WeightedFilesRetrieverV0{
			DisableSectionAware: opts.DisableSectionAwareRetrieval,
			EvidenceMode:        evidenceMode,
			ConceptBackfill:     opts.ExperimentalConceptBackfill,
			GlossaryConcepts:    opts.ExperimentalGlossaryConcepts,
			TieredConceptOutput: opts.ExperimentalTieredConceptOutput,
			AnchorFirstRanking:  opts.ExperimentalAnchorFirstRanking,
			AnchorFirstMode:     opts.ExperimentalAnchorFirstMode,
		}
	}
	tokenizerProfile := tokenizerProfile(counter)
	fixtureAbs, err := filepath.Abs(fixture)
	if err != nil {
		return nil, err
	}
	loadPhase := telemetry.start("load_cases")
	caseFile, err := loadCaseFile(fixtureAbs)
	if err != nil {
		loadPhase.finish("error", nil, map[string]string{"error": err.Error()})
		return nil, err
	}
	loadPhase.finish("ok", map[string]int{"cases": len(caseFile.Cases)}, nil)
	corpusSource := defaultString(opts.CorpusSource, CorpusSourceSQLiteIndex)
	indexPhase := telemetry.start("index_or_load_corpus")
	files, cacheReport, budgetEvents, err := collectCorpusFiles(fixtureAbs, corpusSource, opts, telemetry)
	if err != nil {
		indexPhase.finish("error", nil, map[string]string{"error": err.Error()})
		return nil, err
	}
	indexPhase.finish("ok", map[string]int{"files": len(files)}, nil)
	var commandOutputs map[string]CommandCaseOutput
	if opts.CommandRunner != nil {
		commandPhase := telemetry.start("command_runner")
		commandOutputs, err = opts.CommandRunner(fixtureAbs, caseFile.Cases)
		if err != nil {
			commandPhase.finish("error", nil, map[string]string{"error": err.Error()})
			return nil, err
		}
		commandPhase.finish("ok", map[string]int{"cases": len(commandOutputs)}, nil)
	}

	preparePhase := telemetry.start("prepare_corpus_contexts")
	fullPlanning := fullPlanningCorpus(files)
	allMarkdown := filterFiles(files, func(f File) bool {
		return strings.EqualFold(filepath.Ext(f.Path), ".md")
	})
	sourceCandidates := sourceContextCandidates(files)
	fullCandidate := mergeFiles(fullPlanning, sourceCandidates)

	fullContext := renderContext("full planning corpus", fullPlanning)
	allMarkdownContext := renderContext("all markdown", allMarkdown)
	sourceContext := renderContext("source/context candidates", sourceCandidates)
	fullCandidateContext := renderContext("full candidate corpus", fullCandidate)
	corpusPaths := candidatePathSet(files)
	preparePhase.finish("ok", map[string]int{
		"planning_files":        len(fullPlanning),
		"markdown_files":        len(allMarkdown),
		"source_context_files":  len(sourceCandidates),
		"full_candidate_files":  len(fullCandidate),
		"full_candidate_tokens": counter.Count(fullCandidateContext),
		"planning_tokens":       counter.Count(fullContext),
		"source_context_tokens": counter.Count(sourceContext),
		"all_markdown_tokens":   counter.Count(allMarkdownContext),
	}, nil)

	result := &Result{
		Fixture:          filepath.ToSlash(fixture),
		FixtureVersion:   defaultString(caseFile.FixtureVersion, "agentic-saas-fragmented-v0"),
		EvalStage:        defaultString(caseFile.EvalStage, "seed_smoke"),
		CorpusSource:     corpusSource,
		ProductPath:      productPathForRun(corpusSource, opts.CommandUnderTest),
		CommandUnderTest: strings.TrimSpace(opts.CommandUnderTest),
		FindRuntime:      strings.TrimSpace(opts.FindRuntime),
		Retriever:        retriever.Name(),
		TokenCounter:     counter.Name(),
		TokenizerProfile: tokenizerProfile,
		PricingProfile:   tokenizerProfile.Pricing,
		Corpus: CorpusSummary{
			PlanningArtifacts:       corpusSlice("planning/intent docs only: openspec/**/*.md, docs/**/*.md, .cursor/**/*.md, .claude/**/*.md, plans/**/*.md, scratch/**/*.md", false, fullPlanning, counter.Count(fullContext)),
			MarkdownFiles:           corpusSlice("all *.md files, excluding ignored dirs and cases.yaml", false, allMarkdown, counter.Count(allMarkdownContext)),
			SourceContextCandidates: corpusSlice("non-markdown text/code candidates considered by filesystem retrieval", true, sourceCandidates, counter.Count(sourceContext)),
			FullCandidateCorpus:     corpusSlice("planning/intent docs plus non-markdown source/context candidates", true, fullCandidate, counter.Count(fullCandidateContext)),
		},
		IndexCache: cacheReport,
		Budgets:    budgetReportFromOptions(opts, budgetEvents),
	}
	for _, c := range caseFile.Cases {
		casePhase := telemetry.start("case")
		devspecsFiles := retriever.Retrieve(files, c.Query)
		queryFiles := retrieval.QueryBaseline(files, c.Query)

		artifactReasons := retrieval.ExplainCandidates(devspecsFiles, c.Query)
		preBudgetTokens := 0
		var droppedByContextBudget []string
		if commandOutputs != nil {
			output, ok := commandOutputs[c.ID]
			if !ok {
				return nil, fmt.Errorf("command eval missing output for case %q", c.ID)
			}
			devspecsFiles = output.Artifacts
			artifactReasons = output.ArtifactReasons
			if len(artifactReasons) == 0 {
				artifactReasons = retrieval.ExplainCandidates(devspecsFiles, c.Query)
			}
		} else if opts.ExperimentalBudgetedPacking && opts.ContextTokenBudget > 0 {
			var dropped []string
			devspecsFiles, preBudgetTokens, dropped = applyContextTokenBudget(c.Query, devspecsFiles, opts.ContextTokenBudget, counter)
			droppedByContextBudget = dropped
			artifactReasons = retrieval.ExplainCandidates(devspecsFiles, c.Query)
		}
		primaryFiles := devspecsFiles
		relatedFiles := []File{}
		relatedReasons := []ArtifactReason{}
		if opts.ExperimentalTieredConceptOutput {
			primaryFiles, relatedFiles = splitPackTierFiles(devspecsFiles)
			primaryReasons, splitRelatedReasons := splitArtifactReasonsByFiles(artifactReasons, relatedFiles)
			devspecsFiles = primaryFiles
			artifactReasons = primaryReasons
			relatedReasons = splitRelatedReasons
		}
		devContext := renderContext(c.Query, devspecsFiles)
		if commandOutputs != nil {
			if output := commandOutputs[c.ID]; output.Context != "" {
				devContext = output.Context
			}
		}
		relatedContext := ""
		combinedTieredFiles := devspecsFiles
		combinedTieredContext := devContext
		if opts.ExperimentalTieredConceptOutput {
			relatedContext = renderContext(c.Query+" related evidence", relatedFiles)
			combinedTieredFiles = append(append([]File(nil), primaryFiles...), relatedFiles...)
			combinedTieredContext = renderContext(c.Query, combinedTieredFiles)
		}
		relatedTokens := 0
		if len(relatedFiles) > 0 {
			relatedTokens = counter.Count(relatedContext)
		}
		queryContext := renderContext(c.Query, queryFiles)

		cr := CaseResult{
			ID:                           c.ID,
			Query:                        c.Query,
			DevSpecsTokens:               counter.Count(devContext),
			FullPlanningTokens:           counter.Count(fullContext),
			AllMarkdownTokens:            counter.Count(allMarkdownContext),
			FullCandidateCorpusTokens:    counter.Count(fullCandidateContext),
			QueryFileBaselineTokens:      counter.Count(queryContext),
			PreBudgetDevSpecsTokens:      preBudgetTokens,
			ContextTokenBudget:           contextTokenBudgetForCase(opts),
			ArtifactsIncluded:            rels(devspecsFiles),
			ArtifactReasons:              artifactReasons,
			RelatedArtifacts:             rels(relatedFiles),
			RelatedArtifactReasons:       relatedReasons,
			RelatedDevSpecsTokens:        relatedTokens,
			CombinedTieredArtifacts:      tieredArtifactsForCase(opts, combinedTieredFiles),
			CombinedTieredDevSpecsTokens: tieredTokensForCase(opts, counter.Count(combinedTieredContext)),
			Baselines: []BaselineMetrics{
				baselineMetrics("full_planning_corpus", "planning/intent docs only: openspec/**/*.md, docs/**/*.md, .cursor/**/*.md, .claude/**/*.md, plans/**/*.md, scratch/**/*.md", false, fullPlanning, expectedPaths(c.ExpectedRelevant), counter.Count(fullContext)),
				baselineMetrics("all_markdown", "all *.md files, excluding ignored dirs and cases.yaml", false, allMarkdown, expectedPaths(c.ExpectedRelevant), counter.Count(allMarkdownContext)),
				baselineMetrics("full_candidate_corpus", "planning/intent docs plus non-markdown source/context candidates", true, fullCandidate, expectedPaths(c.ExpectedRelevant), counter.Count(fullCandidateContext)),
				baselineMetrics("query_file_baseline", "deterministic query term matches across all text/code candidates, including source files", true, queryFiles, expectedPaths(c.ExpectedRelevant), counter.Count(queryContext)),
			},
		}
		cr.TokenReductionVsFullPlanning = tokenReduction(cr.DevSpecsTokens, cr.FullPlanningTokens)
		cr.TokenReductionVsAllMarkdown = tokenReduction(cr.DevSpecsTokens, cr.AllMarkdownTokens)
		cr.TokenReductionVsFullCandidate = tokenReduction(cr.DevSpecsTokens, cr.FullCandidateCorpusTokens)
		cr.TokenReductionVsQueryFile = tokenReduction(cr.DevSpecsTokens, cr.QueryFileBaselineTokens)
		cr.ContextBudgetDroppedArtifacts = droppedByContextBudget
		cr.ContextBudgetDroppedCount = len(droppedByContextBudget)
		if opts.PackDiagnostics {
			pack := retrieval.BuildRoleGroupedPack(devspecsFiles, artifactReasonMap(artifactReasons), c.Query)
			cr.PackDiagnostics = &pack
		}
		applyPackingMetrics(&cr, devspecsFiles)
		applyArtifactMetrics(&cr, c)
		applyDiscoveryDiagnostics(&cr, c, corpusPaths)
		applyConceptMissDiagnostics(&cr, c, queryFiles, opts.ExperimentalGlossaryConcepts)
		cr.ContextSufficiency = evaluateSufficiency(c.SuccessCriteria, devContext, cr.ArtifactsIncluded)
		if opts.ExperimentalTieredConceptOutput {
			cr.CombinedTieredContextSufficiency = evaluateSufficiency(c.SuccessCriteria, combinedTieredContext, cr.CombinedTieredArtifacts)
			applyRelatedTierMetrics(&cr, c, relatedFiles, relatedReasons)
		}
		applyAgentCaseMetrics(&cr, c, devspecsFiles)
		applyPrimaryFalsePositiveDiagnostics(&cr, c)
		cr.CaseDurationMS = casePhase.durationMS()
		if opts.MaxCaseSeconds > 0 && cr.CaseDurationMS > int64(opts.MaxCaseSeconds)*1000 {
			cr.CaseBudgetExceeded = true
			cr.CaseBudgetSeconds = opts.MaxCaseSeconds
			cr.ThresholdFailures = append(cr.ThresholdFailures, fmt.Sprintf("case duration %.1fs exceeded budget %ds", float64(cr.CaseDurationMS)/1000.0, opts.MaxCaseSeconds))
		}
		applyThresholds(&cr, opts)
		result.Cases = append(result.Cases, cr)
		casePhase.finish("ok", map[string]int{
			"artifacts_included": len(cr.ArtifactsIncluded),
			"devspecs_tokens":    cr.DevSpecsTokens,
		}, map[string]string{"case_id": c.ID})
	}
	summarizePhase := telemetry.start("summarize")
	result.Summary = summarize(result.Cases)
	result.Diagnostics = summarizeDiagnostics(result.Cases)
	result.Diagnostics.UnindexedDocumentSummaries = summarizeUnindexedDocuments(fixtureAbs, files)
	result.AgentMetrics = summarizeAgentMetrics(result.Cases)
	result.LaneMetrics = summarizeLaneMetrics(result.Cases)
	result.MetricNotes = agentMetricNotes()
	result.Summary.AgentMetrics = result.AgentMetrics
	if corpusSource == CorpusSourceSQLiteIndex {
		if metrics := openSpecMetricsFromFiles(fixtureAbs, files); metrics != nil {
			result.Diagnostics.OpenSpec = metrics
		}
	}
	result.Summary.FailedThresholdCount += len(CheckSummaryThresholds(result, opts))
	summarizePhase.finish("ok", map[string]int{"cases": len(result.Cases)}, nil)
	result.PhaseTelemetry = telemetry.phases
	return result, nil
}

type phaseRecorder struct {
	phases []PhaseTelemetry
}

type activePhase struct {
	rec     *phaseRecorder
	name    string
	started time.Time
}

func newPhaseRecorder() *phaseRecorder {
	return &phaseRecorder{}
}

func (r *phaseRecorder) start(name string) *activePhase {
	if r == nil {
		return nil
	}
	return &activePhase{rec: r, name: name, started: time.Now().UTC()}
}

func (p *activePhase) durationMS() int64 {
	if p == nil {
		return 0
	}
	return time.Since(p.started).Milliseconds()
}

func (p *activePhase) finish(status string, counts map[string]int, details map[string]string) {
	if p == nil || p.rec == nil {
		return
	}
	ended := time.Now().UTC()
	p.rec.phases = append(p.rec.phases, PhaseTelemetry{
		Name:       p.name,
		StartedAt:  p.started.Format(time.RFC3339Nano),
		EndedAt:    ended.Format(time.RFC3339Nano),
		DurationMS: ended.Sub(p.started).Milliseconds(),
		Status:     status,
		Counts:     counts,
		Details:    details,
	})
}

func tokenizerProfile(counter TokenCounter) TokenizerProfile {
	if profiled, ok := counter.(ProfiledTokenCounter); ok {
		return profiled.Profile()
	}
	return TokenizerProfile{
		Name:     counter.Name(),
		Provider: "custom",
		Pricing: PricingProfile{
			Name: "none",
		},
	}
}

func loadCaseFile(fixtureAbs string) (*CaseFile, error) {
	data, err := os.ReadFile(filepath.Join(fixtureAbs, "cases.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load cases.yaml: %w", err)
	}
	var cf CaseFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse cases.yaml: %w", err)
	}
	if len(cf.Cases) == 0 {
		return nil, fmt.Errorf("cases.yaml has no cases")
	}
	for i := range cf.Cases {
		c := &cf.Cases[i]
		if strings.TrimSpace(c.ID) == "" {
			return nil, fmt.Errorf("cases[%d]: id is required", i)
		}
		if strings.TrimSpace(c.Query) == "" {
			return nil, fmt.Errorf("cases[%d]: query is required", i)
		}
		if len(c.ExpectedRelevant) == 0 {
			return nil, fmt.Errorf("cases[%d]: expected_relevant is required", i)
		}
		for j := range c.ExpectedRelevant {
			c.ExpectedRelevant[j].Path = filepath.ToSlash(strings.TrimSpace(c.ExpectedRelevant[j].Path))
			if c.ExpectedRelevant[j].Path == "" {
				return nil, fmt.Errorf("cases[%d].expected_relevant[%d]: path is required", i, j)
			}
			importance, err := normalizeImportance(c.ExpectedRelevant[j].Importance)
			if err != nil {
				return nil, fmt.Errorf("cases[%d].expected_relevant[%d]: %w", i, j, err)
			}
			c.ExpectedRelevant[j].Importance = importance
		}
		for j := range c.ExpectedExcluded {
			c.ExpectedExcluded[j] = filepath.ToSlash(strings.TrimSpace(c.ExpectedExcluded[j]))
		}
		normalizeCriteriaPaths(&c.SuccessCriteria)
	}
	return &cf, nil
}

func normalizeImportance(value string) (string, error) {
	importance := strings.ToLower(strings.TrimSpace(value))
	if importance == "" {
		importance = "must"
	}
	switch importance {
	case "must", "helpful", "background", "same_cluster":
		return importance, nil
	default:
		return "", fmt.Errorf("importance must be must, helpful, background, or same_cluster, got %q", value)
	}
}

func normalizeCriteriaPaths(c *SuccessCriteria) {
	for i := range c.MustContainArtifacts {
		c.MustContainArtifacts[i] = filepath.ToSlash(strings.TrimSpace(c.MustContainArtifacts[i]))
	}
	for i := range c.MustNotContainArtifacts {
		c.MustNotContainArtifacts[i] = filepath.ToSlash(strings.TrimSpace(c.MustNotContainArtifacts[i]))
	}
	for i := range c.MustContainTerms {
		c.MustContainTerms[i] = strings.TrimSpace(c.MustContainTerms[i])
	}
	for i := range c.MustNotContainTerms {
		c.MustNotContainTerms[i] = strings.TrimSpace(c.MustNotContainTerms[i])
	}
	for i := range c.LegacyMustNotContain {
		c.LegacyMustNotContain[i] = strings.TrimSpace(c.LegacyMustNotContain[i])
	}
}

type File = retrieval.Candidate

const evalIndexCacheSchemaVersion = 3

type indexedCorpusCacheFile struct {
	SchemaVersion int    `json:"schema_version"`
	Key           string `json:"key"`
	CreatedAt     string `json:"created_at"`
	Root          string `json:"root"`
	Files         []File `json:"files"`
}

func collectCorpusFiles(root, corpusSource string, opts Options, telemetry *phaseRecorder) ([]File, *IndexCacheReport, []BudgetEvent, error) {
	switch corpusSource {
	case "", CorpusSourceFilesystemFixture:
		files, err := collectFiles(root)
		if err != nil {
			return nil, nil, nil, err
		}
		files, events := applyEvalBudgets(files, opts)
		return files, nil, events, nil
	case CorpusSourceSQLiteIndex:
		return collectIndexedFiles(root, opts, telemetry)
	default:
		return nil, nil, nil, fmt.Errorf("unknown eval corpus source %q", corpusSource)
	}
}

func productPathForRun(corpusSource, commandUnderTest string) string {
	if strings.TrimSpace(commandUnderTest) != "" {
		return ProductPathLiveCLICommand
	}
	switch corpusSource {
	case CorpusSourceSQLiteIndex:
		return ProductPathIndexedHarness
	default:
		return ProductPathLabOnly
	}
}

func collectFiles(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldIgnore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "cases.yaml" {
			return nil
		}
		if d.IsDir() || !isTextArtifact(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files = append(files, File{Path: rel, Body: string(data)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func collectIndexedFiles(root string, opts Options, telemetry *phaseRecorder) ([]File, *IndexCacheReport, []BudgetEvent, error) {
	cacheReport := &IndexCacheReport{}
	cachePath := ""
	cacheKey := ""
	if strings.TrimSpace(opts.IndexCacheDir) != "" {
		cacheReport.Enabled = true
		cacheReport.SchemaVersion = evalIndexCacheSchemaVersion
		cacheReport.CorpusFingerprint = evalIndexedCorpusIndexFingerprint()
		cacheReport.ProvenanceFingerprint = evalRunProvenanceFingerprint()
		key, err := indexedCorpusCacheKey(root, opts)
		if err != nil {
			return nil, cacheReport, nil, fmt.Errorf("compute indexed eval cache key: %w", err)
		}
		cacheKey = key
		cachePath = filepath.Join(opts.IndexCacheDir, key+".json")
		cacheReport.Key = key
		cacheReport.Path = filepath.ToSlash(cachePath)
		if !opts.RefreshIndexCache {
			cacheReadPhase := telemetry.start("index_cache_read")
			if cached, ok := readIndexedCorpusCache(cachePath, key); ok {
				cacheReport.Hit = true
				cacheReport.Reason = "cache_hit"
				files, events := applyEvalBudgets(cached.Files, opts)
				cacheReadPhase.finish("hit", map[string]int{"files": len(cached.Files)}, map[string]string{"key": key})
				return files, cacheReport, events, nil
			}
			cacheReadPhase.finish("miss", nil, map[string]string{"key": key})
		}
		if opts.RefreshIndexCache {
			cacheReport.Reason = "refresh_requested"
		} else {
			cacheReport.Reason = "cache_miss"
		}
	}

	tempDir, err := os.MkdirTemp("", "devspecs-eval-index-*")
	if err != nil {
		return nil, cacheReport, nil, fmt.Errorf("create indexed eval temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := store.Open(filepath.Join(tempDir, "devspecs.db"))
	if err != nil {
		return nil, cacheReport, nil, fmt.Errorf("open indexed eval database: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	cfg, err := config.LoadRepoConfig(root)
	if err != nil {
		return nil, cacheReport, nil, fmt.Errorf("load fixture repo config: %w", err)
	}
	cfg = config.WithDefaultIntentCandidateDiscovery(cfg, true)
	if opts.TestCaseArtifacts {
		cfg = config.WithTestCaseArtifacts(cfg, true)
	}
	if opts.CodeCommentArtifacts {
		cfg = config.WithCodeCommentArtifacts(cfg, true)
	}
	adpts := []adapters.Adapter{
		&openspec.Adapter{},
		&adr.Adapter{},
		&markdown.Adapter{},
		&sourcecontext.Adapter{},
	}
	if cfg.TestCaseArtifactsEnabled(false) {
		adpts = append(adpts, &testcase.Adapter{})
	}
	if cfg.CodeCommentArtifactsEnabled(false) {
		adpts = append(adpts, &codecomment.Adapter{})
	}
	scanner := scan.New(db, idgen.NewFactory(), adpts)
	scanPhase := telemetry.start("sqlite_scan")
	if _, err := scanner.RunWithOptions(context.Background(), root, cfg, scan.RunOptions{
		MaxCandidatesByAdapter: evalCandidateLimits(opts),
		UseTransaction:         true,
		SkipAuthoredAtLookup:   true,
		FreshIndex:             true,
		Progress:               evalScanProgressCallback(root, opts),
		ProgressInterval:       opts.ProgressInterval,
	}); err != nil {
		scanPhase.finish("error", nil, map[string]string{"error": err.Error()})
		return nil, cacheReport, nil, fmt.Errorf("scan fixture into indexed eval database: %w", err)
	}
	scanPhase.finish("ok", nil, nil)

	readbackPhase := telemetry.start("sqlite_readback")
	files, err := collectIndexedFilesFromDB(db, root)
	if err != nil {
		readbackPhase.finish("error", nil, map[string]string{"error": err.Error()})
		return nil, cacheReport, nil, fmt.Errorf("read indexed eval artifacts: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	readbackPhase.finish("ok", map[string]int{"files": len(files)}, nil)
	if cachePath != "" && cacheKey != "" {
		cacheWritePhase := telemetry.start("index_cache_write")
		if err := writeIndexedCorpusCache(cachePath, indexedCorpusCacheFile{
			SchemaVersion: evalIndexCacheSchemaVersion,
			Key:           cacheKey,
			CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			Root:          filepath.ToSlash(root),
			Files:         files,
		}); err != nil {
			cacheWritePhase.finish("error", map[string]int{"files": len(files)}, map[string]string{"error": err.Error(), "key": cacheKey})
			cacheReport.Reason = "cache_write_failed: " + err.Error()
		} else {
			cacheWritePhase.finish("ok", map[string]int{"files": len(files)}, map[string]string{"key": cacheKey})
		}
	}
	files, events := applyEvalBudgets(files, opts)
	return files, cacheReport, events, nil
}

func collectIndexedFilesFromDB(db *store.DB, root string) ([]File, error) {
	seen := map[string]bool{}
	artifacts, bodies, extracted, err := listIndexedArtifactsWithRevisions(db, root)
	if err != nil {
		return nil, err
	}
	sourcesByArtifact, err := listIndexedSourcesByArtifact(db, root)
	if err != nil {
		return nil, err
	}
	todosByArtifact, err := listIndexedTodosByArtifact(db, root)
	if err != nil {
		return nil, err
	}
	linksByArtifact, err := listIndexedLinksByArtifact(db, root)
	if err != nil {
		return nil, err
	}
	sectionsByArtifact, err := listIndexedSectionsByArtifact(db, root)
	if err != nil {
		return nil, err
	}

	candidates := make([]File, 0, len(artifacts))
	for _, art := range artifacts {
		candidates = append(candidates, indexquery.ArtifactCandidateWithLinks(
			art,
			sourcesByArtifact[art.ID],
			linksByArtifact[art.ID],
			todosByArtifact[art.ID],
			sectionsByArtifact[art.ID],
			bodies[art.ID],
			extracted[art.ID],
		))
	}
	files := make([]File, 0, len(candidates))
	for _, candidate := range candidates {
		rel := filepath.ToSlash(candidate.Path)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		files = append(files, candidate)
	}
	return files, nil
}

func listIndexedArtifactsWithRevisions(db *store.DB, root string) ([]store.ArtifactRow, map[string]string, map[string]string, error) {
	rows, err := db.Query(`SELECT a.id, a.repo_id, COALESCE(a.short_id,''), a.kind, COALESCE(a.subtype,''), a.title, a.status, COALESCE(a.current_revision_id,''), a.created_at, a.updated_at, a.last_observed_at, COALESCE(ar.body,''), COALESCE(ar.extracted_json,'')
FROM artifacts a
JOIN repos r ON a.repo_id = r.id
LEFT JOIN artifact_revisions ar ON ar.id = a.current_revision_id
WHERE r.root_path = ?
ORDER BY a.last_observed_at DESC`, root)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	var artifacts []store.ArtifactRow
	bodies := map[string]string{}
	extracted := map[string]string{}
	for rows.Next() {
		var art store.ArtifactRow
		var body string
		var extractedJSON string
		if err := rows.Scan(&art.ID, &art.RepoID, &art.ShortID, &art.Kind, &art.Subtype, &art.Title, &art.Status, &art.CurrentRevID, &art.CreatedAt, &art.UpdatedAt, &art.LastObservedAt, &body, &extractedJSON); err != nil {
			return nil, nil, nil, err
		}
		artifacts = append(artifacts, art)
		bodies[art.ID] = body
		extracted[art.ID] = extractedJSON
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}
	return artifacts, bodies, extracted, nil
}

func listIndexedSourcesByArtifact(db *store.DB, root string) (map[string][]store.SourceRow, error) {
	rows, err := db.Query(`SELECT s.id, s.artifact_id, s.source_type, COALESCE(s.path,''), s.source_identity, COALESCE(s.format_profile,''), COALESCE(s.layout_group,'')
FROM sources s
JOIN repos r ON s.repo_id = r.id
WHERE r.root_path = ?
ORDER BY s.artifact_id, s.path, s.id`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]store.SourceRow{}
	for rows.Next() {
		var src store.SourceRow
		if err := rows.Scan(&src.ID, &src.ArtifactID, &src.SourceType, &src.Path, &src.SourceIdentity, &src.FormatProfile, &src.LayoutGroup); err != nil {
			return nil, err
		}
		out[src.ArtifactID] = append(out[src.ArtifactID], src)
	}
	return out, rows.Err()
}

func listIndexedTodosByArtifact(db *store.DB, root string) (map[string][]store.TodoRow, error) {
	rows, err := db.Query(`SELECT t.id, t.artifact_id, t.revision_id, t.ordinal, t.text, t.done, t.source_file, t.source_line
FROM artifact_todos t
JOIN artifacts a ON t.artifact_id = a.id
JOIN repos r ON a.repo_id = r.id
WHERE r.root_path = ?
ORDER BY t.artifact_id, t.ordinal`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]store.TodoRow{}
	for rows.Next() {
		var todo store.TodoRow
		if err := rows.Scan(&todo.ID, &todo.ArtifactID, &todo.RevisionID, &todo.Ordinal, &todo.Text, &todo.Done, &todo.SourceFile, &todo.SourceLine); err != nil {
			return nil, err
		}
		out[todo.ArtifactID] = append(out[todo.ArtifactID], todo)
	}
	return out, rows.Err()
}

func listIndexedLinksByArtifact(db *store.DB, root string) (map[string][]store.LinkRow, error) {
	rows, err := db.Query(`SELECT l.id, l.artifact_id, l.link_type, l.target, l.created_at
FROM links l
JOIN artifacts a ON l.artifact_id = a.id
JOIN repos r ON a.repo_id = r.id
WHERE r.root_path = ?
ORDER BY l.artifact_id, l.link_type, l.target`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]store.LinkRow{}
	for rows.Next() {
		var link store.LinkRow
		if err := rows.Scan(&link.ID, &link.ArtifactID, &link.LinkType, &link.Target, &link.CreatedAt); err != nil {
			return nil, err
		}
		out[link.ArtifactID] = append(out[link.ArtifactID], link)
	}
	return out, rows.Err()
}

func listIndexedSectionsByArtifact(db *store.DB, root string) (map[string][]store.SectionRow, error) {
	rows, err := db.Query(`SELECT s.id, s.artifact_id, s.revision_id, s.source_path, s.heading_path, s.heading_depth, s.start_line, s.end_line, s.title, s.body, s.token_estimate, s.section_kind, s.metadata_json
FROM artifact_sections s
JOIN artifacts a ON s.artifact_id = a.id
JOIN repos r ON a.repo_id = r.id
WHERE r.root_path = ?
ORDER BY s.artifact_id, s.start_line, s.heading_path`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]store.SectionRow{}
	for rows.Next() {
		var section store.SectionRow
		if err := rows.Scan(&section.ID, &section.ArtifactID, &section.RevisionID, &section.SourcePath, &section.HeadingPath, &section.HeadingDepth, &section.StartLine, &section.EndLine, &section.Title, &section.Body, &section.TokenEstimate, &section.SectionKind, &section.MetadataJSON); err != nil {
			return nil, err
		}
		out[section.ArtifactID] = append(out[section.ArtifactID], section)
	}
	return out, rows.Err()
}

func indexedCorpusCacheKey(root string, opts Options) (string, error) {
	repoDigest, err := repoSnapshotDigest(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "schema=%d\n", evalIndexCacheSchemaVersion)
	fmt.Fprintf(h, "index_shape=%s\n", evalIndexedCorpusIndexFingerprint())
	fmt.Fprintf(h, "repo=%s\n", repoDigest)
	fmt.Fprintf(h, "corpus=%s\n", CorpusSourceSQLiteIndex)
	fmt.Fprintf(h, "tests=%t\n", opts.TestCaseArtifacts)
	fmt.Fprintf(h, "comments=%t\n", opts.CodeCommentArtifacts)
	fmt.Fprintf(h, "max_corpus_files=%d\n", opts.MaxCorpusFiles)
	fmt.Fprintf(h, "max_source_files=%d\n", opts.MaxSourceFiles)
	fmt.Fprintf(h, "max_test_case_artifacts=%d\n", opts.MaxTestCaseArtifacts)
	fmt.Fprintf(h, "max_code_comments=%d\n", opts.MaxCodeComments)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func evalIndexedCorpusIndexFingerprint() string {
	parts := []string{
		"eval-indexed-corpus-v3",
		"adapters:openspec,adr,markdown,sourcecontext,testcase,codecomment",
		"retrieval-candidate-shape:v2-section-aware",
	}
	if root, ok := evalSourceRoot(); ok {
		if digest, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths()); err == nil && digest != "" {
			parts = append(parts, "index_shape_tree="+digest)
			return strings.Join(parts, "|")
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		parts = append(parts, "module="+info.Main.Path+"@"+info.Main.Version)
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				parts = append(parts, setting.Key+"="+setting.Value)
			}
		}
	}
	return strings.Join(parts, "|")
}

func evalRunProvenanceFingerprint() string {
	parts := []string{"eval-run-provenance-v1"}
	if root, ok := evalSourceRoot(); ok {
		if digest, err := sourceTreeDigest(root, evalRunProvenanceFingerprintPaths()); err == nil && digest != "" {
			parts = append(parts, "source_tree="+digest)
			return strings.Join(parts, "|")
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		parts = append(parts, "module="+info.Main.Path+"@"+info.Main.Version)
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				parts = append(parts, setting.Key+"="+setting.Value)
			}
		}
	}
	return strings.Join(parts, "|")
}

func indexedCorpusFingerprintPaths() []string {
	return []string{
		filepath.Join("internal", "adapters"),
		filepath.Join("internal", "classify"),
		filepath.Join("internal", "config"),
		filepath.Join("internal", "format"),
		filepath.Join("internal", "ignore"),
		filepath.Join("internal", "indexquery"),
		filepath.Join("internal", "scan"),
		filepath.Join("internal", "sections"),
		filepath.Join("internal", "store"),
	}
}

func evalRunProvenanceFingerprintPaths() []string {
	return []string{
		filepath.Join("internal", "commands"),
		filepath.Join("internal", "evalharness"),
		filepath.Join("internal", "retrieval"),
	}
}

func evalSourceRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || strings.TrimSpace(file) == "" {
		return "", false
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if info, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && !info.IsDir() {
		return root, true
	}
	return "", false
}

func sourceTreeDigest(root string, relPaths []string) (string, error) {
	h := sha256.New()
	for _, relPath := range relPaths {
		path := filepath.Join(root, relPath)
		if err := hashSourcePath(h, root, path); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func hashSourcePath(h io.Writer, root, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return hashSourceFile(h, root, path, info)
	}
	return filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".go" && ext != ".sql" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return hashSourceFile(h, root, path, info)
	})
}

func hashSourceFile(h io.Writer, root, path string, info os.FileInfo) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".go" && ext != ".sql" {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	fmt.Fprintf(h, "source=%s size=%d\n", rel, info.Size())
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(h, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Fprintln(h)
	return nil
}

func repoSnapshotDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldIgnore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if rel != "cases.yaml" && !isTextArtifact(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "path=%s size=%d\n", rel, info.Size())
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(h, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintln(h)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func readIndexedCorpusCache(path, key string) (indexedCorpusCacheFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return indexedCorpusCacheFile{}, false
	}
	var cached indexedCorpusCacheFile
	if err := json.Unmarshal(data, &cached); err != nil {
		return indexedCorpusCacheFile{}, false
	}
	if cached.SchemaVersion != evalIndexCacheSchemaVersion || cached.Key != key {
		return indexedCorpusCacheFile{}, false
	}
	return cached, true
}

func writeIndexedCorpusCache(path string, cached indexedCorpusCacheFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func budgetReportFromOptions(opts Options, events []BudgetEvent) BudgetReport {
	return BudgetReport{
		MaxCorpusFiles:       opts.MaxCorpusFiles,
		MaxSourceFiles:       opts.MaxSourceFiles,
		MaxTestCaseArtifacts: opts.MaxTestCaseArtifacts,
		MaxCodeComments:      opts.MaxCodeComments,
		MaxCaseSeconds:       opts.MaxCaseSeconds,
		Applied:              events,
	}
}

func applyEvalBudgets(files []File, opts Options) ([]File, []BudgetEvent) {
	out := append([]File(nil), files...)
	var events []BudgetEvent
	out, events = applyTypedBudget(out, "source_context_files", opts.MaxSourceFiles, func(f File) bool {
		return retrieval.IsSourceContextCandidate(f)
	}, events)
	out, events = applyTypedBudget(out, "test_case_artifacts", opts.MaxTestCaseArtifacts, isTestCaseFile, events)
	out, events = applyTypedBudget(out, "code_comment_artifacts", opts.MaxCodeComments, isCodeCommentFile, events)
	if opts.MaxCorpusFiles > 0 && len(out) > opts.MaxCorpusFiles {
		before := len(out)
		sort.SliceStable(out, func(i, j int) bool {
			pi := corpusBudgetPriority(out[i])
			pj := corpusBudgetPriority(out[j])
			if pi == pj {
				return out[i].Path < out[j].Path
			}
			return pi < pj
		})
		out = out[:opts.MaxCorpusFiles]
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		events = append(events, BudgetEvent{
			Name:    "corpus_files",
			Before:  before,
			After:   len(out),
			Message: "kept highest-priority planning and source context artifacts",
		})
	}
	return out, events
}

func evalCandidateLimits(opts Options) map[string]int {
	limits := map[string]int{}
	if opts.MaxSourceFiles > 0 {
		limits["source_context"] = opts.MaxSourceFiles
	}
	if opts.MaxTestCaseArtifacts > 0 {
		limits["test_case"] = opts.MaxTestCaseArtifacts
	}
	if opts.MaxCodeComments > 0 {
		limits["code_comment"] = opts.MaxCodeComments
	}
	if len(limits) == 0 {
		return nil
	}
	return limits
}

func evalScanProgressCallback(root string, opts Options) func(scan.ProgressEvent) {
	if opts.ProgressWriter == nil {
		return nil
	}
	fixture := filepath.ToSlash(root)
	return func(event scan.ProgressEvent) {
		payload := struct {
			Type    string             `json:"type"`
			Fixture string             `json:"fixture"`
			Event   scan.ProgressEvent `json:"event"`
		}{
			Type:    "eval_scan_progress",
			Fixture: fixture,
			Event:   event,
		}
		if data, err := json.Marshal(payload); err == nil {
			fmt.Fprintln(opts.ProgressWriter, string(data))
		}
	}
}

func applyTypedBudget(files []File, name string, limit int, match func(File) bool, events []BudgetEvent) ([]File, []BudgetEvent) {
	if limit <= 0 {
		return files, events
	}
	var matched int
	for _, f := range files {
		if match(f) {
			matched++
		}
	}
	if matched <= limit {
		return files, events
	}
	kept := 0
	out := make([]File, 0, len(files)-(matched-limit))
	for _, f := range files {
		if !match(f) {
			out = append(out, f)
			continue
		}
		if kept < limit {
			out = append(out, f)
			kept++
		}
	}
	events = append(events, BudgetEvent{
		Name:    name,
		Before:  matched,
		After:   kept,
		Message: "deterministic path-order cap",
	})
	return out, events
}

func corpusBudgetPriority(f File) int {
	switch {
	case strings.EqualFold(filepath.Ext(f.Path), ".md") && retrieval.IsPlanningIntentPath(f.Path):
		return 0
	case strings.EqualFold(filepath.Ext(f.Path), ".md"):
		return 1
	case isTestCaseFile(f):
		return 2
	case isCodeCommentFile(f):
		return 3
	case retrieval.IsSourceContextCandidate(f):
		return 4
	default:
		return 5
	}
}

func openSpecMetricsFromFiles(repoRoot string, files []File) *openspecmetrics.Metrics {
	artifacts := make([]openspecmetrics.Artifact, 0, len(files))
	for _, f := range files {
		artifact := openspecmetrics.Artifact{
			Path:    f.Path,
			Subtype: f.Subtype,
		}
		if f.Metadata != nil {
			artifact.SourceType = f.Metadata["source_type"]
			artifact.SourceIdentity = f.Metadata["source_identity"]
			artifact.ArtifactScope = f.Metadata["artifact_scope"]
			artifact.OpenSpecRole = f.Metadata["openspec_role"]
		}
		artifacts = append(artifacts, artifact)
	}
	metrics := openspecmetrics.Analyze(repoRoot, artifacts)
	if !metrics.HasData() {
		return nil
	}
	return &metrics
}

func shouldIgnore(rel string, isDir bool) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	ignoredDirs := map[string]bool{
		".git": true, "node_modules": true, "dist": true, "build": true,
		".next": true, "coverage": true, "tmp": true, "vendor": true,
	}
	if isDir {
		if ignoredDirs[parts[len(parts)-1]] {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(rel))
	return strings.HasSuffix(base, ".lock") || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock"
}

func isTextArtifact(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".mdx", ".py", ".go", ".rs", ".java",
		".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue",
		".toml", ".sql", ".yaml", ".yml", ".json":
		return true
	default:
		base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
		return base == "dockerfile" || base == "containerfile" ||
			strings.HasPrefix(base, "dockerfile.") || strings.HasPrefix(base, "containerfile.") ||
			strings.HasSuffix(base, ".dockerfile") || strings.HasSuffix(base, ".containerfile")
	}
}

func fullPlanningCorpus(files []File) []File {
	return filterFiles(files, func(f File) bool {
		rel := f.Path
		if !evalMarkdownLikePath(rel) {
			return false
		}
		for _, prefix := range []string{"openspec/", "docs/", ".cursor/", ".claude/", "plans/", "scratch/"} {
			if strings.HasPrefix(rel, prefix) {
				return true
			}
		}
		return retrieval.IsPlanningIntentPath(rel)
	})
}

func sourceContextCandidates(files []File) []File {
	return filterFiles(files, func(f File) bool {
		if evalMarkdownLikePath(f.Path) {
			return false
		}
		if retrieval.IsPlanningIntentPath(f.Path) {
			return false
		}
		return true
	})
}

func mergeFiles(groups ...[]File) []File {
	seen := map[string]bool{}
	var out []File
	for _, group := range groups {
		for _, f := range group {
			if seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func filterFiles(files []File, keep func(File) bool) []File {
	var out []File
	for _, f := range files {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}

func corpusSlice(scope string, includesSource bool, files []File, tokens int) CorpusSlice {
	return CorpusSlice{
		FileScope:                scope,
		IncludesSourceCandidates: includesSource,
		Files:                    len(files),
		Tokens:                   tokens,
		Artifacts:                rels(files),
	}
}

func renderContext(label string, files []File) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# DevSpecs Eval Context\n\nQuery: %s\n\n", label)
	for _, f := range files {
		fmt.Fprintf(&b, "## %s\n\n```text\n%s\n```\n\n", f.Path, strings.TrimRight(f.Body, "\r\n"))
	}
	return b.String()
}

func contextTokenBudgetForCase(opts Options) int {
	if !opts.ExperimentalBudgetedPacking || opts.ContextTokenBudget <= 0 {
		return 0
	}
	return opts.ContextTokenBudget
}

func applyContextTokenBudget(query string, files []File, budget int, counter TokenCounter) ([]File, int, []string) {
	preBudgetTokens := counter.Count(renderContext(query, files))
	if budget <= 0 || len(files) == 0 || preBudgetTokens <= budget {
		return files, preBudgetTokens, nil
	}
	kept := make([]File, 0, len(files))
	for _, f := range files {
		candidate := append(append([]File(nil), kept...), f)
		if counter.Count(renderContext(query, candidate)) <= budget {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		kept = append(kept, files[0])
	}
	keptSet := map[string]bool{}
	for _, f := range kept {
		keptSet[f.Path] = true
	}
	var dropped []string
	for _, f := range files {
		if !keptSet[f.Path] {
			dropped = append(dropped, f.Path)
		}
	}
	return kept, preBudgetTokens, dropped
}

func rels(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func splitPackTierFiles(files []File) ([]File, []File) {
	primary := make([]File, 0, len(files))
	var related []File
	for _, f := range files {
		switch retrieval.CandidatePackTier(f) {
		case retrieval.PackTierRelated, retrieval.PackTierDiagnostic:
			related = append(related, f)
		default:
			primary = append(primary, f)
		}
	}
	return primary, related
}

func splitArtifactReasonsByFiles(reasons []ArtifactReason, relatedFiles []File) ([]ArtifactReason, []ArtifactReason) {
	if len(relatedFiles) == 0 {
		return reasons, nil
	}
	related := stringSet(rels(relatedFiles))
	primaryReasons := make([]ArtifactReason, 0, len(reasons))
	relatedReasons := make([]ArtifactReason, 0, len(relatedFiles))
	for _, reason := range reasons {
		if related[filepath.ToSlash(reason.Path)] {
			relatedReasons = append(relatedReasons, reason)
			continue
		}
		primaryReasons = append(primaryReasons, reason)
	}
	return primaryReasons, relatedReasons
}

func artifactReasonMap(reasons []ArtifactReason) map[string][]string {
	if len(reasons) == 0 {
		return nil
	}
	out := make(map[string][]string, len(reasons))
	for _, reason := range reasons {
		out[reason.Path] = reason.Reasons
	}
	return out
}

func tieredArtifactsForCase(opts Options, files []File) []string {
	if !opts.ExperimentalTieredConceptOutput {
		return nil
	}
	return rels(files)
}

func tieredTokensForCase(opts Options, tokens int) int {
	if !opts.ExperimentalTieredConceptOutput {
		return 0
	}
	return tokens
}

func expectedPaths(items []ExpectedArtifact) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, filepath.ToSlash(item.Path))
	}
	return out
}

func expectedImportanceSet(items []ExpectedArtifact) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		importance, err := normalizeImportance(item.Importance)
		if err != nil {
			importance = "must"
		}
		out[filepath.ToSlash(item.Path)] = importance
	}
	return out
}

func evaluateSufficiency(criteria SuccessCriteria, context string, included []string) SufficiencyResult {
	result := SufficiencyResult{
		Configured:                criteria.Configured(),
		MissingTerms:              []string{},
		MissingArtifacts:          []string{},
		ForbiddenTermsPresent:     []string{},
		ForbiddenArtifactsPresent: []string{},
		Failures:                  []string{},
	}
	if !result.Configured {
		return result
	}
	contextLower := strings.ToLower(context)
	includedSet := stringSet(included)
	for _, term := range criteria.MustContainTerms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if !strings.Contains(contextLower, strings.ToLower(term)) {
			result.MissingTerms = append(result.MissingTerms, term)
		}
	}
	for _, artifact := range criteria.MustContainArtifacts {
		artifact = filepath.ToSlash(strings.TrimSpace(artifact))
		if artifact == "" {
			continue
		}
		if !evalArtifactPathInSetByIdentity(artifact, includedSet) {
			result.MissingArtifacts = append(result.MissingArtifacts, artifact)
		}
	}
	for _, term := range append([]string{}, criteria.MustNotContainTerms...) {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(contextLower, strings.ToLower(term)) {
			result.ForbiddenTermsPresent = append(result.ForbiddenTermsPresent, term)
		}
	}
	for _, term := range criteria.LegacyMustNotContain {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(contextLower, strings.ToLower(term)) {
			result.ForbiddenTermsPresent = append(result.ForbiddenTermsPresent, term)
		}
	}
	for _, artifact := range criteria.MustNotContainArtifacts {
		artifact = filepath.ToSlash(strings.TrimSpace(artifact))
		if artifact == "" {
			continue
		}
		if evalArtifactPathInSetByIdentity(artifact, includedSet) {
			result.ForbiddenArtifactsPresent = append(result.ForbiddenArtifactsPresent, artifact)
		}
	}
	if len(result.MissingTerms) > 0 {
		result.Failures = append(result.Failures, "missing required terms: "+strings.Join(result.MissingTerms, ", "))
	}
	if len(result.MissingArtifacts) > 0 {
		result.Failures = append(result.Failures, "missing required artifacts: "+strings.Join(result.MissingArtifacts, ", "))
	}
	if len(result.ForbiddenTermsPresent) > 0 {
		result.Failures = append(result.Failures, "forbidden terms present: "+strings.Join(result.ForbiddenTermsPresent, ", "))
	}
	if len(result.ForbiddenArtifactsPresent) > 0 {
		result.Failures = append(result.Failures, "forbidden artifacts present: "+strings.Join(result.ForbiddenArtifactsPresent, ", "))
	}
	result.Passed = len(result.Failures) == 0
	return result
}

func baselineMetrics(name, scope string, includesSource bool, files []File, expected []string, tokens int) BaselineMetrics {
	rel := rels(files)
	expectedSet := stringSet(expected)
	relevant := 0
	for _, path := range rel {
		if evalArtifactPathInSetByIdentity(path, expectedSet) {
			relevant++
		}
	}
	return BaselineMetrics{
		Name:                     name,
		FileScope:                scope,
		IncludesSourceCandidates: includesSource,
		Tokens:                   tokens,
		ArtifactCount:            len(rel),
		Artifacts:                rel,
		RelevantIncluded:         relevant,
		IrrelevantCount:          len(rel) - relevant,
	}
}

func applyPackingMetrics(cr *CaseResult, files []File) {
	for _, f := range files {
		if isTestCaseFile(f) {
			cr.TestCaseArtifactCount++
		}
		if isCodeCommentFile(f) {
			cr.CodeCommentArtifactCount++
		}
		if f.Metadata != nil && f.Metadata["indexed_section_retrieval_mode"] == "section_aware" {
			cr.SectionSelectedArtifacts = append(cr.SectionSelectedArtifacts, f.Path)
			if count, err := strconv.Atoi(strings.TrimSpace(f.Metadata["indexed_section_match_count"])); err == nil {
				cr.SectionSelectedCount += count
			}
		}
		if f.Metadata != nil && f.Metadata["section_pack_mode"] == "sections" {
			cr.PackedSectionArtifacts = append(cr.PackedSectionArtifacts, f.Path)
			if count, err := strconv.Atoi(strings.TrimSpace(f.Metadata["section_pack_count"])); err == nil {
				cr.PackedSectionCount += count
			}
			continue
		}
		if evalMarkdownLikePath(f.Path) {
			cr.FullFileArtifactCount++
		}
	}
}

func isTestCaseFile(f File) bool {
	if f.Subtype == config.SubtypeTestCase {
		return true
	}
	if f.Metadata == nil {
		return false
	}
	return f.Metadata["source_type"] == "test_case"
}

func isCodeCommentFile(f File) bool {
	if f.Subtype == config.SubtypeCodeComment {
		return true
	}
	if f.Metadata == nil {
		return false
	}
	return f.Metadata["source_type"] == "code_comment"
}

func applyArtifactMetrics(cr *CaseResult, spec CaseSpec) {
	expected := expectedImportanceSet(spec.ExpectedRelevant)
	excluded := stringSet(spec.ExpectedExcluded)
	included := stringSet(cr.ArtifactsIncluded)
	matchedExpected := map[string]bool{}

	cr.RelevantIncluded = []string{}
	cr.IrrelevantIncluded = []string{}
	cr.MissedExpectedRelevant = []string{}
	cr.UnexpectedExcludedHits = []string{}
	cr.ExpectedRelevantCount = len(spec.ExpectedRelevant)
	for _, artifact := range spec.ExpectedRelevant {
		switch artifact.Importance {
		case "must":
			cr.MustExpectedCount++
		case "helpful":
			cr.HelpfulExpectedCount++
		case "background":
			cr.BackgroundExpectedCount++
		}
	}
	for _, rel := range cr.ArtifactsIncluded {
		if importance, expectedPath, ok := matchExpectedIncludedArtifact(rel, expected, included, matchedExpected); ok {
			matchedExpected[expectedPath] = true
			cr.RelevantIncluded = append(cr.RelevantIncluded, rel)
			switch importance {
			case "must":
				cr.MustRelevantRetrieved++
			case "helpful":
				cr.HelpfulRelevantRetrieved++
			case "background":
				cr.BackgroundRelevantRetrieved++
			}
		} else {
			cr.IrrelevantIncluded = append(cr.IrrelevantIncluded, rel)
		}
		if evalArtifactPathInSetByIdentity(rel, excluded) {
			cr.UnexpectedExcludedHits = append(cr.UnexpectedExcludedHits, rel)
		}
	}
	for _, artifact := range spec.ExpectedRelevant {
		path := filepath.ToSlash(artifact.Path)
		if !matchedExpected[path] {
			cr.MissedExpectedRelevant = append(cr.MissedExpectedRelevant, path)
		}
	}
	cr.RelevantRetrieved = len(cr.RelevantIncluded)
	if cr.ExpectedRelevantCount > 0 {
		cr.ArtifactRecall = float64(cr.RelevantRetrieved) / float64(cr.ExpectedRelevantCount)
	}
	if cr.MustExpectedCount > 0 {
		cr.MustHaveRecall = float64(cr.MustRelevantRetrieved) / float64(cr.MustExpectedCount)
	}
	if cr.HelpfulExpectedCount > 0 {
		cr.HelpfulRecall = float64(cr.HelpfulRelevantRetrieved) / float64(cr.HelpfulExpectedCount)
	}
	if cr.BackgroundExpectedCount > 0 {
		cr.BackgroundRecall = float64(cr.BackgroundRelevantRetrieved) / float64(cr.BackgroundExpectedCount)
	}
	if len(cr.ArtifactsIncluded) > 0 {
		cr.ArtifactPrecision = float64(len(cr.RelevantIncluded)) / float64(len(cr.ArtifactsIncluded))
	}
}

func matchExpectedIncludedArtifact(path string, expected map[string]string, included map[string]bool, matched map[string]bool) (string, string, bool) {
	path = filepath.ToSlash(path)
	if importance, ok := expected[path]; ok {
		return importance, path, true
	}
	for expectedPath, importance := range expected {
		if matched[expectedPath] || included[expectedPath] {
			continue
		}
		if evalArtifactIdentityMatch(path, expectedPath) {
			return importance, expectedPath, true
		}
	}
	return "", "", false
}

func applyRelatedTierMetrics(cr *CaseResult, spec CaseSpec, relatedFiles []File, relatedReasons []ArtifactReason) {
	if len(relatedFiles) == 0 {
		return
	}
	related := CaseResult{
		ArtifactsIncluded:  rels(relatedFiles),
		ArtifactReasons:    relatedReasons,
		ContextSufficiency: cr.CombinedTieredContextSufficiency,
	}
	applyArtifactMetrics(&related, spec)
	applyAgentCaseMetrics(&related, spec, relatedFiles)
	cr.RelatedRelevantIncluded = related.RelevantIncluded
	cr.RelatedIrrelevantIncluded = related.IrrelevantIncluded
	cr.RelatedArtifactPrecision = related.ArtifactPrecision
	cr.RelatedAgentMetrics = related.AgentMetrics
	cr.RelatedArtifactGrades = related.ArtifactGrades
}

func applyPrimaryFalsePositiveDiagnostics(cr *CaseResult, spec CaseSpec) {
	if len(cr.ArtifactGrades) == 0 {
		return
	}
	reasonsByPath := artifactReasonsByPath(cr.ArtifactReasons)
	positionByPath := map[string]int{}
	for i, path := range cr.ArtifactsIncluded {
		positionByPath[normalizeMetricPath(path)] = i + 1
	}
	queryType := classifyEvalQueryType(spec.Query)
	for _, grade := range cr.ArtifactGrades {
		if grade.Exact {
			continue
		}
		norm := normalizeMetricPath(grade.Path)
		reasons := limitStrings(reasonsByPath[norm], 3)
		example := FalsePositiveExample{
			CaseID:      spec.ID,
			QueryType:   queryType,
			Path:        filepath.ToSlash(grade.Path),
			Position:    positionByPath[norm],
			Lane:        grade.Lane,
			Role:        diagnosticRole(grade.Path),
			Grade:       grade.Grade,
			Weight:      grade.Weight,
			ReasonClass: classifyReasonClass(reasonsByPath[norm]),
			Reasons:     reasons,
		}
		cr.PrimaryFalsePositiveDiagnostics = append(cr.PrimaryFalsePositiveDiagnostics, example)
	}
}

func artifactReasonsByPath(reasons []ArtifactReason) map[string][]string {
	out := map[string][]string{}
	for _, reason := range reasons {
		path := normalizeMetricPath(reason.Path)
		if path == "" {
			continue
		}
		for _, text := range reason.Reasons {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			out[path] = append(out[path], text)
		}
	}
	return out
}

func classifyEvalQueryType(query string) string {
	q := strings.ToLower(query)
	switch {
	case containsAny(q, "test", "tests", "coverage", "regression", "behavior", "behaviour"):
		return "test_behavior"
	case containsAny(q, "openspec", "open spec", "change bundle", "proposal.md", "tasks.md", "spec.md"):
		return "openspec"
	case containsAny(q, "rfc", "request for comments"):
		return "rfc"
	case containsAny(q, "prd", "product", "requirement", "requirements", "user story", "user stories"):
		return "product_requirements"
	case containsAny(q, "adr", "decision", "why", "rationale"):
		return "decision_rationale"
	case containsAny(q, "architecture", "design", "technical design", "system design"):
		return "architecture_design"
	case containsAny(q, "plan", "roadmap", "proposal", "resume", "continue"):
		return "plan_proposal"
	case containsAny(q, "agent", "claude", "codex", "cursor", "skill", "instructions"):
		return "agent_protocol"
	case containsAny(q, "template", "scaffold"):
		return "template"
	case containsAny(q, "source", "implementation", "code", "function", "class"):
		return "implementation_context"
	default:
		return "general_intent"
	}
}

func classifyReasonClass(reasons []string) string {
	joined := strings.ToLower(strings.Join(reasons, "\n"))
	switch {
	case strings.Contains(joined, "concept backfill"):
		return "concept_backfill"
	case strings.Contains(joined, "pack tier"):
		return "tiered_related"
	case strings.Contains(joined, "section-packed"):
		return "section_packed"
	case strings.Contains(joined, "indexed section") || strings.Contains(joined, "section match"):
		return "section_match"
	case strings.Contains(joined, "test-name"):
		return "test_name_anchor"
	case strings.Contains(joined, "test-case behavior"):
		return "test_behavior_signal"
	case strings.Contains(joined, "code comment"):
		return "code_comment_signal"
	case strings.Contains(joined, "relationship expansion"):
		return "relationship_expansion"
	case strings.Contains(joined, "authority prior"):
		return "authority_prior"
	case strings.Contains(joined, "classifier"):
		return "classifier_signal"
	case strings.Contains(joined, "path") || strings.Contains(joined, "title"):
		return "path_title_signal"
	case strings.TrimSpace(joined) == "":
		return "unexplained"
	default:
		return "term_overlap"
	}
}

func applyDiscoveryDiagnostics(cr *CaseResult, spec CaseSpec, corpusPaths map[string]bool) {
	included := stringSet(cr.ArtifactsIncluded)
	availableRetrieved := 0
	cr.ExpectedAvailableCount = 0
	cr.ExpectedMissingFromCorpus = nil
	cr.MissedAfterDiscovery = nil
	for _, artifact := range spec.ExpectedRelevant {
		path := filepath.ToSlash(artifact.Path)
		if evalArtifactPathInSetByIdentity(path, corpusPaths) {
			cr.ExpectedAvailableCount++
			if evalArtifactPathInSetByIdentity(path, included) {
				availableRetrieved++
			} else {
				cr.MissedAfterDiscovery = append(cr.MissedAfterDiscovery, path)
			}
			continue
		}
		cr.ExpectedMissingFromCorpus = append(cr.ExpectedMissingFromCorpus, path)
	}
	if cr.ExpectedRelevantCount > 0 {
		cr.DiscoveryCoverage = float64(cr.ExpectedAvailableCount) / float64(cr.ExpectedRelevantCount)
	}
	if cr.ExpectedAvailableCount > 0 {
		cr.RetrievalCoverageOfDiscovered = float64(availableRetrieved) / float64(cr.ExpectedAvailableCount)
	}
}

func applyConceptMissDiagnostics(cr *CaseResult, spec CaseSpec, candidatePool []File, glossaryConcepts bool) {
	if cr.MustExpectedCount == 0 {
		return
	}
	included := stringSet(cr.ArtifactsIncluded)
	ranks := retrieval.RankConceptCandidates(candidatePool, spec.Query)
	if glossaryConcepts {
		ranks = retrieval.RankConceptCandidatesWithGlossary(candidatePool, spec.Query)
	}
	rankByPath := map[string]ConceptMissDiagnostic{}
	for i, rank := range ranks {
		path := filepath.ToSlash(rank.Path)
		if path == "" {
			continue
		}
		if _, exists := rankByPath[path]; exists {
			continue
		}
		rankByPath[path] = ConceptMissDiagnostic{
			ExpectedPath:     path,
			InCandidatePool:  true,
			ConceptRank:      i + 1,
			ConceptScore:     rank.Score,
			MatchedCompacts:  append([]string(nil), rank.MatchedCompacts...),
			MatchedPhrases:   append([]string(nil), rank.MatchedPhrases...),
			MatchedPathTerms: append([]string(nil), rank.MatchedPathTerms...),
			GlossaryMatches:  append([]string(nil), rank.GlossaryMatches...),
			GlossaryEvidence: append([]string(nil), rank.GlossaryEvidence...),
		}
	}
	poolPaths := candidatePathSet(candidatePool)
	for _, artifact := range spec.ExpectedRelevant {
		if artifact.Importance != "must" {
			continue
		}
		path := filepath.ToSlash(artifact.Path)
		if evalArtifactPathInSetByIdentity(path, included) {
			continue
		}
		if diag, ok := rankByPath[path]; ok {
			cr.MissedMustConceptDiagnostics = append(cr.MissedMustConceptDiagnostics, diag)
			continue
		}
		if diag, ok := conceptDiagnosticByIdentity(path, rankByPath); ok {
			cr.MissedMustConceptDiagnostics = append(cr.MissedMustConceptDiagnostics, diag)
			continue
		}
		cr.MissedMustConceptDiagnostics = append(cr.MissedMustConceptDiagnostics, ConceptMissDiagnostic{
			ExpectedPath:    path,
			InCandidatePool: evalArtifactPathInSetByIdentity(path, poolPaths),
		})
	}
}

func conceptDiagnosticByIdentity(path string, rankByPath map[string]ConceptMissDiagnostic) (ConceptMissDiagnostic, bool) {
	for candidate, diag := range rankByPath {
		if evalArtifactIdentityMatch(path, candidate) {
			diag.ExpectedPath = path
			return diag, true
		}
	}
	return ConceptMissDiagnostic{}, false
}

func summarizeDiagnostics(cases []CaseResult) Diagnostics {
	var out Diagnostics
	if len(cases) == 0 {
		return out
	}
	missingFromCorpus := map[string]bool{}
	missedAfterDiscovery := map[string]bool{}
	roles := map[string]*RoleDiagnostic{}
	missClasses := map[string]*MissClassDiagnostic{}
	falsePositives := map[string]*FalsePositiveDiagnostic{}
	extensions := map[string]*ExtensionDiagnostic{}
	addMissClass := func(class, path string) {
		diag := missClasses[class]
		if diag == nil {
			diag = &MissClassDiagnostic{Class: class}
			missClasses[class] = diag
		}
		diag.Count++
		if len(diag.Examples) < 8 {
			diag.Examples = append(diag.Examples, path)
		}
	}
	ensureExtension := func(path string) *ExtensionDiagnostic {
		ext, role := diagnosticExtensionRole(path)
		key := ext + "|" + role
		diag := extensions[key]
		if diag == nil {
			diag = &ExtensionDiagnostic{Extension: ext, Role: role}
			extensions[key] = diag
		}
		if len(diag.Examples) < 8 {
			diag.Examples = append(diag.Examples, filepath.ToSlash(path))
		}
		return diag
	}
	for _, c := range cases {
		out.ExpectedRelevantCount += c.ExpectedRelevantCount
		out.ExpectedAvailableCount += c.ExpectedAvailableCount
		out.ExpectedMissingFromCorpusCount += len(c.ExpectedMissingFromCorpus)
		out.MissedAfterDiscoveryCount += len(c.MissedAfterDiscovery)
		for _, path := range c.ExpectedMissingFromCorpus {
			missingFromCorpus[path] = true
			addMissClass("missing_from_corpus/"+diagnosticRole(path)+"/"+diagnosticAnchorClass(path), path)
			ensureExtension(path).MissingFromCorpus++
		}
		for _, path := range c.MissedAfterDiscovery {
			missedAfterDiscovery[path] = true
			addMissClass("missed_after_discovery/"+diagnosticRole(path)+"/"+diagnosticAnchorClass(path), path)
			ensureExtension(path).MissedAfterDiscovery++
		}
		for _, path := range append([]string{}, c.RelevantIncluded...) {
			role := diagnosticRole(path)
			ensureRoleDiagnostic(roles, role).Retrieved++
			ensureExtension(path).ExactRetrieved++
		}
		for _, path := range c.IrrelevantIncluded {
			role := diagnosticRole(path)
			ensureRoleDiagnostic(roles, role).IrrelevantRetrieved++
		}
		for _, path := range append(append([]string{}, c.RelevantIncluded...), c.MissedExpectedRelevant...) {
			role := diagnosticRole(path)
			ensureRoleDiagnostic(roles, role).Expected++
			ensureExtension(path).Expected++
		}
		for _, path := range append([]string{}, c.RelevantIncluded...) {
			role := diagnosticRole(path)
			roles[role].ExpectedAvailable++
		}
		for _, path := range c.MissedAfterDiscovery {
			role := diagnosticRole(path)
			diag := ensureRoleDiagnostic(roles, role)
			diag.ExpectedAvailable++
			diag.MissedAfterDiscovery++
		}
		for _, path := range c.ExpectedMissingFromCorpus {
			role := diagnosticRole(path)
			ensureRoleDiagnostic(roles, role).MissingFromCorpus++
		}
		for _, fp := range c.PrimaryFalsePositiveDiagnostics {
			class := falsePositiveClass(fp)
			diag := falsePositives[class]
			if diag == nil {
				diag = &FalsePositiveDiagnostic{
					Class:       class,
					QueryType:   fp.QueryType,
					Lane:        fp.Lane,
					Role:        fp.Role,
					ReasonClass: fp.ReasonClass,
				}
				falsePositives[class] = diag
			}
			diag.Count++
			addGradeCount(&diag.GradeCounts, fp.Grade)
			if len(diag.Examples) < 8 {
				diag.Examples = append(diag.Examples, fp)
			}
			extDiag := ensureExtension(fp.Path)
			extDiag.PrimaryFalsePositive++
			addGradeCount(&extDiag.PrimaryFalsePositiveGrades, fp.Grade)
		}
	}
	out.ExpectedMissingFromCorpus = sortedSet(missingFromCorpus)
	out.MissedAfterDiscovery = sortedSet(missedAfterDiscovery)
	if out.ExpectedRelevantCount > 0 {
		out.DiscoveryCoverage = float64(out.ExpectedAvailableCount) / float64(out.ExpectedRelevantCount)
	}
	if out.ExpectedAvailableCount > 0 {
		retrieved := out.ExpectedAvailableCount - out.MissedAfterDiscoveryCount
		if retrieved < 0 {
			retrieved = 0
		}
		out.RetrievalCoverageOfDiscovered = float64(retrieved) / float64(out.ExpectedAvailableCount)
	}
	roleNames := make([]string, 0, len(roles))
	for role := range roles {
		roleNames = append(roleNames, role)
	}
	sort.Strings(roleNames)
	for _, role := range roleNames {
		diag := *roles[role]
		if diag.Expected > 0 {
			diag.DiscoveryCoverage = float64(diag.ExpectedAvailable) / float64(diag.Expected)
		}
		if diag.ExpectedAvailable > 0 {
			diag.RetrievalCoverageOfDiscovered = float64(diag.Retrieved) / float64(diag.ExpectedAvailable)
		}
		out.RoleSummaries = append(out.RoleSummaries, diag)
	}
	classNames := make([]string, 0, len(missClasses))
	for class := range missClasses {
		classNames = append(classNames, class)
	}
	sort.Strings(classNames)
	for _, class := range classNames {
		diag := *missClasses[class]
		sort.Strings(diag.Examples)
		out.MissClassSummaries = append(out.MissClassSummaries, diag)
	}
	fpClasses := make([]string, 0, len(falsePositives))
	for class := range falsePositives {
		fpClasses = append(fpClasses, class)
	}
	sort.Slice(fpClasses, func(i, j int) bool {
		left := falsePositives[fpClasses[i]]
		right := falsePositives[fpClasses[j]]
		if left.Count == right.Count {
			return left.Class < right.Class
		}
		return left.Count > right.Count
	})
	for _, class := range fpClasses {
		out.FalsePositiveSummaries = append(out.FalsePositiveSummaries, *falsePositives[class])
	}
	extensionKeys := make([]string, 0, len(extensions))
	for key := range extensions {
		extensionKeys = append(extensionKeys, key)
	}
	sort.Slice(extensionKeys, func(i, j int) bool {
		left := extensions[extensionKeys[i]]
		right := extensions[extensionKeys[j]]
		leftProblems := left.MissingFromCorpus + left.MissedAfterDiscovery + left.PrimaryFalsePositive
		rightProblems := right.MissingFromCorpus + right.MissedAfterDiscovery + right.PrimaryFalsePositive
		if leftProblems == rightProblems {
			return left.Extension+"|"+left.Role < right.Extension+"|"+right.Role
		}
		return leftProblems > rightProblems
	})
	for _, key := range extensionKeys {
		out.ExtensionSummaries = append(out.ExtensionSummaries, *extensions[key])
	}
	return out
}

func falsePositiveClass(fp FalsePositiveExample) string {
	return "query=" + fp.QueryType + "/lane=" + fp.Lane + "/role=" + fp.Role + "/reason=" + fp.ReasonClass + "/grade=" + fp.Grade
}

func summarizeUnindexedDocuments(root string, indexedFiles []File) []UnindexedDocumentDiagnostic {
	indexed := map[string]bool{}
	for _, f := range indexedFiles {
		path := stripLineRef(filepath.ToSlash(f.Path))
		if path != "" {
			indexed[path] = true
		}
	}
	byKey := map[string]*UnindexedDocumentDiagnostic{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch strings.ToLower(name) {
			case ".git", ".devspecs", "node_modules", "vendor", "dist", "build", "target":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.EqualFold(rel, "cases.yaml") || indexed[rel] {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if !isDocumentGapExtension(ext) {
			return nil
		}
		role := diagnosticRole(rel)
		key := ext + "|" + role
		diag := byKey[key]
		if diag == nil {
			diag = &UnindexedDocumentDiagnostic{Extension: ext, Role: role}
			byKey[key] = diag
		}
		diag.Count++
		if len(diag.Examples) < 12 {
			diag.Examples = append(diag.Examples, rel)
		}
		return nil
	})
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := byKey[keys[i]]
		right := byKey[keys[j]]
		if left.Count == right.Count {
			return left.Extension+"|"+left.Role < right.Extension+"|"+right.Role
		}
		return left.Count > right.Count
	})
	out := make([]UnindexedDocumentDiagnostic, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	return out
}

func isDocumentGapExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".adoc", ".asciidoc", ".asc", ".rst", ".mdx", ".org":
		return true
	default:
		return false
	}
}

func stripLineRef(path string) string {
	lower := strings.ToLower(path)
	if idx := strings.Index(lower, "#l"); idx >= 0 {
		return path[:idx]
	}
	return path
}

func ensureRoleDiagnostic(roles map[string]*RoleDiagnostic, role string) *RoleDiagnostic {
	if roles[role] == nil {
		roles[role] = &RoleDiagnostic{Role: role}
	}
	return roles[role]
}

func diagnosticExtensionRole(path string) (string, string) {
	withoutLine := filepath.ToSlash(path)
	if idx := strings.Index(strings.ToLower(withoutLine), "#l"); idx >= 0 {
		withoutLine = withoutLine[:idx]
	}
	ext := strings.ToLower(filepath.Ext(withoutLine))
	if ext == "" {
		ext = "(none)"
	}
	return ext, diagnosticRole(path)
}

func diagnosticRole(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.HasSuffix(path, ".adoc") || strings.HasSuffix(path, ".asciidoc") || strings.HasSuffix(path, ".asc"):
		return "asciidoc"
	case isOpenSpecDiagnosticPath(path) && strings.Contains(path, "/specs/") && !strings.Contains(path, "/changes/") && strings.HasSuffix(path, "/spec.md"):
		return "openspec_base_spec"
	case isOpenSpecDiagnosticPath(path) && strings.HasSuffix(path, "/proposal.md"):
		return "openspec_proposal"
	case isOpenSpecDiagnosticPath(path) && strings.HasSuffix(path, "/design.md"):
		return "openspec_design"
	case isOpenSpecDiagnosticPath(path) && strings.HasSuffix(path, "/tasks.md"):
		return "openspec_tasks"
	case isOpenSpecDiagnosticPath(path) && strings.Contains(path, "/changes/") && strings.Contains(path, "/specs/") && strings.HasSuffix(path, "/spec.md"):
		return "openspec_spec_delta"
	case strings.HasPrefix(path, "docs/adr/") || strings.HasPrefix(path, "docs/adrs/") || strings.Contains(path, "/docs/adr/") || strings.Contains(path, "/docs/adrs/"):
		return "adr"
	case strings.HasPrefix(path, "rfcs/") || strings.HasPrefix(path, "rfc/") || strings.HasPrefix(path, "docs/rfcs/") || strings.HasPrefix(path, "docs/rfc/") || strings.Contains(path, "/rfcs/") || strings.Contains(path, "/rfc/"):
		return "rfc"
	case strings.HasPrefix(path, "docs/prd/") || strings.HasPrefix(path, "docs/prds/") || strings.Contains(path, "/docs/prd/") || strings.Contains(path, "/docs/prds/"):
		return "prd"
	case strings.HasPrefix(path, "docs/product-specs/") || strings.Contains(path, "/docs/product-specs/") || strings.Contains(path, "/product-specs/"):
		return "prd"
	case strings.HasPrefix(path, ".cursor/"):
		return "cursor_plan"
	case strings.HasPrefix(path, ".claude/"):
		if strings.Contains(path, "/skills/") {
			return "skill"
		}
		return "claude_plan"
	case strings.HasPrefix(path, ".codex/"):
		if strings.Contains(path, "/skills/") {
			return "skill"
		}
		return "codex_plan"
	case strings.Contains(path, "/agents/") || strings.HasSuffix(path, ".agent.md"):
		return "agent_instruction"
	case strings.Contains(path, "/plans/") || strings.HasPrefix(path, "plans/") || strings.HasSuffix(path, ".plan.md"):
		return "plan"
	case strings.Contains(path, "/requirements/") || strings.HasPrefix(filepath.Base(path), "req_") || strings.HasPrefix(filepath.Base(path), "req-"):
		return "prd"
	case strings.EqualFold(filepath.Ext(path), ".md"):
		return "markdown"
	default:
		return "source"
	}
}

func diagnosticAnchorClass(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	role := diagnosticRole(path)
	switch {
	case strings.Contains(path, "#l") && role == "source":
		return "line_scoped_source"
	case looksLikeDiagnosticTestPath(path):
		return "test_or_source_anchor"
	case role == "source":
		return "source_anchor"
	default:
		return "document_anchor"
	}
}

func looksLikeDiagnosticTestPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	switch {
	case strings.Contains(path, "/tests/") || strings.Contains(path, "/__tests__/") || strings.Contains(path, "/spec/"):
		return true
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")):
		return true
	case (ext == ".java" || ext == ".kt" || ext == ".kts") &&
		(strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "spec")):
		return true
	default:
		return false
	}
}

func isOpenSpecDiagnosticPath(path string) bool {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "openspec" || strings.HasPrefix(path, "openspec/") || strings.HasSuffix(path, "/openspec") {
		return true
	}
	return strings.Contains(path, "/openspec/")
}

func applyThresholds(cr *CaseResult, opts Options) {
	if opts.MinRecall != nil && cr.ArtifactRecall < *opts.MinRecall {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("recall %.1f%% below minimum %.1f%%", cr.ArtifactRecall*100, *opts.MinRecall*100))
	}
	if opts.MinReductionFull != nil && cr.TokenReductionVsFullPlanning < *opts.MinReductionFull {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("token reduction vs full planning %.1f%% below minimum %.1f%%", cr.TokenReductionVsFullPlanning*100, *opts.MinReductionFull*100))
	}
	if opts.MinMustRecall != nil && cr.MustExpectedCount > 0 && cr.MustHaveRecall < *opts.MinMustRecall {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("must-have recall %.1f%% below minimum %.1f%%", cr.MustHaveRecall*100, *opts.MinMustRecall*100))
	}
}

func summarize(cases []CaseResult) Summary {
	s := Summary{Cases: len(cases)}
	if len(cases) == 0 {
		return s
	}
	var reductionsFull, reductionsQueryFile []float64
	worstRecall := math.MaxFloat64
	largestTokens := -1
	mustCases := 0
	helpfulCases := 0
	backgroundCases := 0
	relatedCases := 0
	for _, c := range cases {
		reductionsFull = append(reductionsFull, c.TokenReductionVsFullPlanning)
		reductionsQueryFile = append(reductionsQueryFile, c.TokenReductionVsQueryFile)
		s.MeanTokenReductionVsFullPlanning += c.TokenReductionVsFullPlanning
		s.MeanTokenReductionVsQueryFileBaseline += c.TokenReductionVsQueryFile
		s.MeanArtifactRecall += c.ArtifactRecall
		if c.MustExpectedCount > 0 {
			s.MeanMustHaveRecall += c.MustHaveRecall
			mustCases++
		}
		if c.HelpfulExpectedCount > 0 {
			s.MeanHelpfulRecall += c.HelpfulRecall
			helpfulCases++
		}
		if c.BackgroundExpectedCount > 0 {
			s.MeanBackgroundRecall += c.BackgroundRecall
			backgroundCases++
		}
		s.MeanArtifactPrecision += c.ArtifactPrecision
		s.MeanGradedPrecision += c.AgentMetrics.GradedPrecision
		s.MeanPenalizedUtilityPrecision += c.AgentMetrics.PenalizedUtilityPrecision
		addGradeCounts(&s.GradeCounts, c.AgentMetrics.GradeCounts)
		if len(c.RelatedArtifacts) > 0 {
			relatedCases++
			s.RelatedArtifactCount += len(c.RelatedArtifacts)
			s.RelatedRelevantCount += len(c.RelatedRelevantIncluded)
			s.MeanRelatedArtifactPrecision += c.RelatedArtifactPrecision
			s.MeanRelatedGradedPrecision += c.RelatedAgentMetrics.GradedPrecision
			addGradeCounts(&s.RelatedGradeCounts, c.RelatedAgentMetrics.GradeCounts)
		}
		s.FailedThresholdCount += len(c.ThresholdFailures)
		if c.ContextSufficiency.Configured {
			s.ContextSufficiencyCases++
			if c.ContextSufficiency.Passed {
				s.ContextSufficiencyPassed++
			}
		}
		if c.CombinedTieredContextSufficiency.Configured {
			s.CombinedTieredContextSufficiencyCases++
			if c.CombinedTieredContextSufficiency.Passed {
				s.CombinedTieredContextSufficiencyPassed++
			}
		}
		if c.ArtifactRecall < worstRecall {
			worstRecall = c.ArtifactRecall
			s.WorstRecallCase = c.ID
		}
		if c.DevSpecsTokens > largestTokens {
			largestTokens = c.DevSpecsTokens
			s.LargestTokenContextCase = c.ID
		}
	}
	n := float64(len(cases))
	s.MeanTokenReductionVsFullPlanning /= n
	s.MeanTokenReductionVsQueryFileBaseline /= n
	s.MeanArtifactRecall /= n
	if mustCases > 0 {
		s.MeanMustHaveRecall /= float64(mustCases)
	}
	if helpfulCases > 0 {
		s.MeanHelpfulRecall /= float64(helpfulCases)
	}
	if backgroundCases > 0 {
		s.MeanBackgroundRecall /= float64(backgroundCases)
	}
	s.MeanArtifactPrecision /= n
	s.MeanGradedPrecision /= n
	s.MeanPenalizedUtilityPrecision /= n
	s.RelatedCases = relatedCases
	if relatedCases > 0 {
		s.MeanRelatedArtifactPrecision /= float64(relatedCases)
		s.MeanRelatedGradedPrecision /= float64(relatedCases)
	}
	if s.ContextSufficiencyCases > 0 {
		s.ContextSufficiencyPassRate = float64(s.ContextSufficiencyPassed) / float64(s.ContextSufficiencyCases)
	}
	if s.CombinedTieredContextSufficiencyCases > 0 {
		s.CombinedTieredContextSufficiencyPassRate = float64(s.CombinedTieredContextSufficiencyPassed) / float64(s.CombinedTieredContextSufficiencyCases)
	}
	s.MedianTokenReductionVsFullPlanning = median(reductionsFull)
	s.MedianTokenReductionVsQueryFileBaseline = median(reductionsQueryFile)
	s.Pareto = ParetoSummary{
		MeanTokenReductionVsFullPlanning:      s.MeanTokenReductionVsFullPlanning,
		MeanTokenReductionVsQueryFileBaseline: s.MeanTokenReductionVsQueryFileBaseline,
		MeanArtifactRecall:                    s.MeanArtifactRecall,
		MeanMustHaveRecall:                    s.MeanMustHaveRecall,
		MeanArtifactPrecision:                 s.MeanArtifactPrecision,
		MeanGradedPrecision:                   s.MeanGradedPrecision,
		MeanPenalizedUtilityPrecision:         s.MeanPenalizedUtilityPrecision,
		ContextSufficiencyPassRate:            s.ContextSufficiencyPassRate,
	}
	return s
}

func addGradeCounts(total *GradeCounts, next GradeCounts) {
	total.Must += next.Must
	total.Helpful += next.Helpful
	total.Background += next.Background
	total.SameCluster += next.SameCluster
	total.Unlabeled += next.Unlabeled
	total.HardNegative += next.HardNegative
}

func CheckSummaryThresholds(r *Result, opts Options) []string {
	var failures []string
	if opts.MinMeanRecall != nil && r.Summary.MeanArtifactRecall < *opts.MinMeanRecall {
		failures = append(failures,
			fmt.Sprintf("mean recall %.1f%% below minimum %.1f%%", r.Summary.MeanArtifactRecall*100, *opts.MinMeanRecall*100))
	}
	if opts.MinSufficiency != nil && r.Summary.ContextSufficiencyPassRate < *opts.MinSufficiency {
		failures = append(failures,
			fmt.Sprintf("context sufficiency pass rate %.1f%% below minimum %.1f%%", r.Summary.ContextSufficiencyPassRate*100, *opts.MinSufficiency*100))
	}
	return failures
}

func tokenReduction(devspecs, baseline int) float64 {
	if baseline <= 0 {
		return 0
	}
	return 1.0 - (float64(devspecs) / float64(baseline))
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[filepath.ToSlash(item)] = true
	}
	return out
}

func candidatePathSet(files []File) map[string]bool {
	out := make(map[string]bool, len(files))
	for _, f := range files {
		out[filepath.ToSlash(f.Path)] = true
	}
	return out
}

func sortedSet(items map[string]bool) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:limit]...)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func FormatText(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DevSpecs Eval: %s\n\n", r.Fixture)
	fmt.Fprintf(&b, "Cases: %d\n", r.Summary.Cases)
	fmt.Fprintf(&b, "Fixture version: %s\n", r.FixtureVersion)
	fmt.Fprintf(&b, "Eval stage: %s\n", r.EvalStage)
	fmt.Fprintf(&b, "Corpus source: %s\n", r.CorpusSource)
	fmt.Fprintf(&b, "Product path: %s\n", r.ProductPath)
	if r.CommandUnderTest != "" {
		fmt.Fprintf(&b, "Command under test: %s\n", r.CommandUnderTest)
	}
	if r.FindRuntime != "" {
		fmt.Fprintf(&b, "Find runtime: %s\n", r.FindRuntime)
	}
	fmt.Fprintf(&b, "Retriever: %s\n", r.Retriever)
	fmt.Fprintf(&b, "Token counter: %s\n", r.TokenCounter)
	if r.TokenizerProfile.Approximation != "" {
		fmt.Fprintf(&b, "Token counter detail: %s\n", r.TokenizerProfile.Approximation)
	}
	if r.PricingProfile.Name != "" {
		fmt.Fprintf(&b, "Pricing profile: %s\n", r.PricingProfile.Name)
	}
	if r.ResultsFile != "" {
		fmt.Fprintf(&b, "Results file: %s\n", r.ResultsFile)
	}
	if r.IndexCache != nil && r.IndexCache.Enabled {
		fmt.Fprintf(&b, "Index cache: hit=%t key=%s\n", r.IndexCache.Hit, shortKey(r.IndexCache.Key))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Corpus")
	fmt.Fprintf(&b, "- Planning artifacts: %d files / %s tokens\n", r.Corpus.PlanningArtifacts.Files, comma(r.Corpus.PlanningArtifacts.Tokens))
	fmt.Fprintf(&b, "- Markdown files: %d files / %s tokens\n", r.Corpus.MarkdownFiles.Files, comma(r.Corpus.MarkdownFiles.Tokens))
	fmt.Fprintf(&b, "- Source/context candidates: %d files / %s tokens\n", r.Corpus.SourceContextCandidates.Files, comma(r.Corpus.SourceContextCandidates.Tokens))
	fmt.Fprintf(&b, "- Full candidate corpus: %d files / %s tokens\n\n", r.Corpus.FullCandidateCorpus.Files, comma(r.Corpus.FullCandidateCorpus.Tokens))
	if len(r.Budgets.Applied) > 0 {
		fmt.Fprintln(&b, "Budgets")
		for _, event := range r.Budgets.Applied {
			fmt.Fprintf(&b, "- %s: %d -> %d\n", event.Name, event.Before, event.After)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "Summary")
	fmt.Fprintf(&b, "- Median token reduction vs full planning corpus: %s\n", pct(r.Summary.MedianTokenReductionVsFullPlanning))
	fmt.Fprintf(&b, "- Mean token reduction vs full planning corpus: %s\n", pct(r.Summary.MeanTokenReductionVsFullPlanning))
	fmt.Fprintf(&b, "- Median token reduction vs query file baseline: %s\n", pct(r.Summary.MedianTokenReductionVsQueryFileBaseline))
	fmt.Fprintf(&b, "- Mean token reduction vs query file baseline: %s\n", pct(r.Summary.MeanTokenReductionVsQueryFileBaseline))
	fmt.Fprintf(&b, "- Mean artifact recall: %s\n", pct(r.Summary.MeanArtifactRecall))
	fmt.Fprintf(&b, "- Mean must-have recall: %s\n", pct(r.Summary.MeanMustHaveRecall))
	fmt.Fprintf(&b, "- Mean helpful recall: %s\n", pct(r.Summary.MeanHelpfulRecall))
	fmt.Fprintf(&b, "- Mean background recall: %s\n", pct(r.Summary.MeanBackgroundRecall))
	fmt.Fprintf(&b, "- Mean artifact precision: %s\n", pct(r.Summary.MeanArtifactPrecision))
	fmt.Fprintf(&b, "- Mean graded precision: %s\n", pct(r.Summary.MeanGradedPrecision))
	fmt.Fprintf(&b, "- Mean penalized utility precision: %s\n", pct(r.Summary.MeanPenalizedUtilityPrecision))
	if r.Summary.RelatedArtifactCount > 0 {
		fmt.Fprintf(&b, "- Related tier: %d artifacts / precision %s / graded precision %s\n",
			r.Summary.RelatedArtifactCount,
			pct(r.Summary.MeanRelatedArtifactPrecision),
			pct(r.Summary.MeanRelatedGradedPrecision))
	}
	if r.Summary.ContextSufficiencyCases > 0 {
		fmt.Fprintf(&b, "- Context sufficiency pass rate: %d/%d = %s\n", r.Summary.ContextSufficiencyPassed, r.Summary.ContextSufficiencyCases, pct(r.Summary.ContextSufficiencyPassRate))
	}
	if r.Summary.CombinedTieredContextSufficiencyCases > 0 {
		fmt.Fprintf(&b, "- Combined tiered sufficiency pass rate: %d/%d = %s\n",
			r.Summary.CombinedTieredContextSufficiencyPassed,
			r.Summary.CombinedTieredContextSufficiencyCases,
			pct(r.Summary.CombinedTieredContextSufficiencyPassRate))
	}
	fmt.Fprintf(&b, "- Must-hit@3: %s\n", pct(r.AgentMetrics.MustHitAt3))
	fmt.Fprintf(&b, "- Mean first must rank: %.2f\n", r.AgentMetrics.MeanFirstMustRank)
	for _, budget := range r.AgentMetrics.ContextSufficiencyAtTokenBudget {
		fmt.Fprintf(&b, "- Sufficiency within %s tokens: %d/%d = %s\n", comma(budget.BudgetTokens), budget.PassedCases, budget.EligibleCases, pct(budget.PassRate))
	}
	fmt.Fprintf(&b, "- Pareto: reduction %s / recall %s / must-have recall %s / precision %s / graded precision %s / sufficiency %s\n",
		pct(r.Summary.Pareto.MeanTokenReductionVsFullPlanning),
		pct(r.Summary.Pareto.MeanArtifactRecall),
		pct(r.Summary.Pareto.MeanMustHaveRecall),
		pct(r.Summary.Pareto.MeanArtifactPrecision),
		pct(r.Summary.Pareto.MeanGradedPrecision),
		pct(r.Summary.Pareto.ContextSufficiencyPassRate))
	if r.Summary.FailedThresholdCount > 0 {
		fmt.Fprintf(&b, "- Failed thresholds: %d\n", r.Summary.FailedThresholdCount)
	}
	fmt.Fprintln(&b)

	if len(r.LaneMetrics) > 0 {
		fmt.Fprintln(&b, "Lane Metrics")
		for _, lane := range r.LaneMetrics {
			fmt.Fprintf(&b, "- %s: precision %s / graded precision %s / recall %s / included %d / exact relevant %d",
				lane.Lane,
				pct(lane.StrictPrecision),
				pct(lane.GradedPrecision),
				pct(lane.Recall),
				lane.IncludedArtifacts,
				lane.ExactRelevantArtifacts)
			if lane.SameClusterArtifacts > 0 {
				fmt.Fprintf(&b, " / same-cluster %d", lane.SameClusterArtifacts)
			}
			if lane.HardNegativeArtifacts > 0 {
				fmt.Fprintf(&b, " / hard-negative %d", lane.HardNegativeArtifacts)
			}
			if lane.PackedSectionCount > 0 {
				fmt.Fprintf(&b, " / packed sections %d", lane.PackedSectionCount)
			}
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "Diagnostics")
	fmt.Fprintf(&b, "- Discovery coverage: %d/%d = %s\n", r.Diagnostics.ExpectedAvailableCount, r.Diagnostics.ExpectedRelevantCount, pct(r.Diagnostics.DiscoveryCoverage))
	fmt.Fprintf(&b, "- Retrieval coverage of discovered expected artifacts: %s\n", recallText(r.Diagnostics.ExpectedAvailableCount-r.Diagnostics.MissedAfterDiscoveryCount, r.Diagnostics.ExpectedAvailableCount, r.Diagnostics.RetrievalCoverageOfDiscovered))
	fmt.Fprintf(&b, "- Expected missing from corpus: %s\n", listOrNone(r.Diagnostics.ExpectedMissingFromCorpus))
	fmt.Fprintf(&b, "- Missed after discovery: %s\n", listOrNone(r.Diagnostics.MissedAfterDiscovery))
	if len(r.Diagnostics.RoleSummaries) > 0 {
		fmt.Fprintln(&b, "- Role summaries:")
		for _, role := range r.Diagnostics.RoleSummaries {
			fmt.Fprintf(&b, "  - %s: expected %d / available %d / retrieved %d / missing-from-corpus %d / missed-after-discovery %d / irrelevant %d\n",
				role.Role,
				role.Expected,
				role.ExpectedAvailable,
				role.Retrieved,
				role.MissingFromCorpus,
				role.MissedAfterDiscovery,
				role.IrrelevantRetrieved)
		}
	}
	if len(r.Diagnostics.MissClassSummaries) > 0 {
		fmt.Fprintln(&b, "- Miss classes:")
		for _, class := range r.Diagnostics.MissClassSummaries {
			fmt.Fprintf(&b, "  - %s: %d", class.Class, class.Count)
			if len(class.Examples) > 0 {
				fmt.Fprintf(&b, " (examples: %s)", strings.Join(class.Examples, "; "))
			}
			fmt.Fprintln(&b)
		}
	}
	if len(r.Diagnostics.FalsePositiveSummaries) > 0 {
		fmt.Fprintln(&b, "- Primary false-positive classes:")
		for i, class := range r.Diagnostics.FalsePositiveSummaries {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "  - %s: %d", class.Class, class.Count)
			if class.GradeCounts.SameCluster > 0 || class.GradeCounts.Unlabeled > 0 || class.GradeCounts.HardNegative > 0 {
				fmt.Fprintf(&b, " (same-cluster %d / unlabeled %d / hard-negative %d)",
					class.GradeCounts.SameCluster,
					class.GradeCounts.Unlabeled,
					class.GradeCounts.HardNegative)
			}
			if len(class.Examples) > 0 {
				examples := make([]string, 0, minInt(len(class.Examples), 3))
				for _, example := range class.Examples {
					examples = append(examples, example.CaseID+":"+example.Path)
					if len(examples) >= 3 {
						break
					}
				}
				fmt.Fprintf(&b, " (examples: %s)", strings.Join(examples, "; "))
			}
			fmt.Fprintln(&b)
		}
	}
	if len(r.Diagnostics.ExtensionSummaries) > 0 {
		fmt.Fprintln(&b, "- Extension coverage:")
		for i, ext := range r.Diagnostics.ExtensionSummaries {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "  - %s/%s: expected %d / exact %d / missing-from-corpus %d / missed-after-discovery %d / primary false-positive %d\n",
				ext.Extension,
				ext.Role,
				ext.Expected,
				ext.ExactRetrieved,
				ext.MissingFromCorpus,
				ext.MissedAfterDiscovery,
				ext.PrimaryFalsePositive)
		}
	}
	if len(r.Diagnostics.UnindexedDocumentSummaries) > 0 {
		fmt.Fprintln(&b, "- Unindexed document formats:")
		for i, doc := range r.Diagnostics.UnindexedDocumentSummaries {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "  - %s/%s: %d", doc.Extension, doc.Role, doc.Count)
			if len(doc.Examples) > 0 {
				fmt.Fprintf(&b, " (examples: %s)", strings.Join(limitStrings(doc.Examples, 3), "; "))
			}
			fmt.Fprintln(&b)
		}
	}
	if r.Diagnostics.OpenSpec != nil {
		openSpec := r.Diagnostics.OpenSpec
		indexedExpectedBundles := openSpec.ExpectedBundles - len(openSpec.MissingBundles)
		if indexedExpectedBundles < 0 {
			indexedExpectedBundles = 0
		}
		indexedExpectedChildren := openSpec.ExpectedChildArtifacts - len(openSpec.MissingChildRoles)
		if indexedExpectedChildren < 0 {
			indexedExpectedChildren = 0
		}
		fmt.Fprintln(&b, "- OpenSpec:")
		fmt.Fprintf(&b, "  - Bundle recall: %s\n", recallText(indexedExpectedBundles, openSpec.ExpectedBundles, openSpec.BundleRecall))
		fmt.Fprintf(&b, "  - Child-role recall: %s\n", recallText(indexedExpectedChildren, openSpec.ExpectedChildArtifacts, openSpec.ChildRoleRecall))
		fmt.Fprintf(&b, "  - Duplicate pressure: %.2f child artifacts per bundle\n", openSpec.DuplicatePressure)
		fmt.Fprintf(&b, "  - Markdown leakage: %d\n", openSpec.MarkdownLeakage)
	}
	fmt.Fprintln(&b)

	for _, c := range r.Cases {
		fmt.Fprintf(&b, "Case: %s\n", c.ID)
		fmt.Fprintf(&b, "- DevSpecs context: %s tokens\n", comma(c.DevSpecsTokens))
		if c.ContextTokenBudget > 0 {
			fmt.Fprintf(&b, "- Context budget: %s tokens", comma(c.ContextTokenBudget))
			if c.PreBudgetDevSpecsTokens > 0 {
				fmt.Fprintf(&b, " (pre-budget %s)", comma(c.PreBudgetDevSpecsTokens))
			}
			fmt.Fprintln(&b)
			if c.ContextBudgetDroppedCount > 0 {
				fmt.Fprintf(&b, "- Context budget dropped: %s\n", listOrNone(c.ContextBudgetDroppedArtifacts))
			}
		}
		fmt.Fprintf(&b, "- Full planning corpus: %s tokens\n", comma(c.FullPlanningTokens))
		fmt.Fprintf(&b, "- All markdown: %s tokens\n", comma(c.AllMarkdownTokens))
		fmt.Fprintf(&b, "- Full candidate corpus: %s tokens\n", comma(c.FullCandidateCorpusTokens))
		fmt.Fprintf(&b, "- Query file baseline: %s tokens\n", comma(c.QueryFileBaselineTokens))
		fmt.Fprintf(&b, "- Reduction vs full planning corpus: %s\n", pct(c.TokenReductionVsFullPlanning))
		fmt.Fprintf(&b, "- Reduction vs all markdown: %s\n", pct(c.TokenReductionVsAllMarkdown))
		fmt.Fprintf(&b, "- Reduction vs full candidate corpus: %s\n", pct(c.TokenReductionVsFullCandidate))
		fmt.Fprintf(&b, "- Reduction vs query file baseline: %s\n", pct(c.TokenReductionVsQueryFile))
		fmt.Fprintf(&b, "- Recall: %d/%d = %s\n", c.RelevantRetrieved, c.ExpectedRelevantCount, pct(c.ArtifactRecall))
		fmt.Fprintf(&b, "- Must-have recall: %s\n", recallText(c.MustRelevantRetrieved, c.MustExpectedCount, c.MustHaveRecall))
		fmt.Fprintf(&b, "- Helpful recall: %s\n", recallText(c.HelpfulRelevantRetrieved, c.HelpfulExpectedCount, c.HelpfulRecall))
		fmt.Fprintf(&b, "- Background recall: %s\n", recallText(c.BackgroundRelevantRetrieved, c.BackgroundExpectedCount, c.BackgroundRecall))
		fmt.Fprintf(&b, "- Precision: %d/%d = %s\n", len(c.RelevantIncluded), len(c.ArtifactsIncluded), pct(c.ArtifactPrecision))
		fmt.Fprintf(&b, "- Graded precision: %s\n", pct(c.AgentMetrics.GradedPrecision))
		if c.AgentMetrics.FirstMustRank > 0 {
			fmt.Fprintf(&b, "- First must rank: %d\n", c.AgentMetrics.FirstMustRank)
		}
		if c.ContextSufficiency.Configured {
			fmt.Fprintf(&b, "- Sufficiency: %s\n", passFail(c.ContextSufficiency.Passed))
			if len(c.ContextSufficiency.Failures) > 0 {
				fmt.Fprintf(&b, "- Sufficiency failures: %s\n", strings.Join(c.ContextSufficiency.Failures, "; "))
			}
		} else {
			fmt.Fprintf(&b, "- Sufficiency: not configured\n")
		}
		fmt.Fprintf(&b, "- Artifacts included: %s\n", listOrNone(c.ArtifactsIncluded))
		if len(c.RelatedArtifacts) > 0 {
			fmt.Fprintf(&b, "- Related artifacts: %s\n", listOrNone(c.RelatedArtifacts))
			fmt.Fprintf(&b, "- Related precision: %d/%d = %s\n", len(c.RelatedRelevantIncluded), len(c.RelatedArtifacts), pct(c.RelatedArtifactPrecision))
			if c.CombinedTieredContextSufficiency.Configured {
				fmt.Fprintf(&b, "- Combined tiered sufficiency: %s\n", passFail(c.CombinedTieredContextSufficiency.Passed))
			}
		}
		fmt.Fprintf(&b, "- Relevant included: %s\n", listOrNone(c.RelevantIncluded))
		fmt.Fprintf(&b, "- Irrelevant included: %s\n", listOrNone(c.IrrelevantIncluded))
		if len(c.PrimaryFalsePositiveDiagnostics) > 0 {
			examples := make([]string, 0, minInt(len(c.PrimaryFalsePositiveDiagnostics), 5))
			for _, fp := range c.PrimaryFalsePositiveDiagnostics {
				examples = append(examples, fmt.Sprintf("#%d %s [%s/%s/%s]", fp.Position, fp.Path, fp.Lane, fp.Role, fp.Grade))
				if len(examples) >= 5 {
					break
				}
			}
			fmt.Fprintf(&b, "- Primary false positives: %s\n", strings.Join(examples, "; "))
		}
		fmt.Fprintf(&b, "- Missed: %s\n", listOrNone(c.MissedExpectedRelevant))
		fmt.Fprintf(&b, "- Discovery: %d/%d = %s\n", c.ExpectedAvailableCount, c.ExpectedRelevantCount, pct(c.DiscoveryCoverage))
		fmt.Fprintf(&b, "- Retrieval of discovered expected artifacts: %s\n", recallText(c.ExpectedAvailableCount-len(c.MissedAfterDiscovery), c.ExpectedAvailableCount, c.RetrievalCoverageOfDiscovered))
		fmt.Fprintf(&b, "- Expected missing from corpus: %s\n", listOrNone(c.ExpectedMissingFromCorpus))
		fmt.Fprintf(&b, "- Missed after discovery: %s\n", listOrNone(c.MissedAfterDiscovery))
		fmt.Fprintf(&b, "- Unexpected excluded hits: %s\n", listOrNone(c.UnexpectedExcludedHits))
		if len(c.ThresholdFailures) > 0 {
			fmt.Fprintf(&b, "- Threshold failures: %s\n", strings.Join(c.ThresholdFailures, "; "))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func FormatJSON(r *Result) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func pct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

func recallText(retrieved, total int, recall float64) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d = %s", retrieved, total, pct(recall))
}

func passFail(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func shortKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:12]
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
