package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/classify"
	"github.com/devspecs-com/devspecs-cli/internal/evalharness"
	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/spf13/cobra"
)

type firstIndexReportOptions struct {
	ClassifierFixtures []string
	ResultsDir         string
	NoSave             bool
	GeneratedAt        time.Time
	InputUSDPer1M      float64
}

type firstIndexReport struct {
	GeneratedAt          string                       `json:"generated_at"`
	Summary              firstIndexNorthStarSummary   `json:"summary"`
	Retrieval            firstIndexRetrievalReport    `json:"retrieval"`
	Classifiers          []firstIndexClassifierReport `json:"classifiers,omitempty"`
	WeakSpots            []firstIndexWeakSpot         `json:"weak_spots,omitempty"`
	ThresholdFailures    []string                     `json:"threshold_failures,omitempty"`
	FailedThresholdCount int                          `json:"failed_threshold_count,omitempty"`
}

type firstIndexBatchReport struct {
	GeneratedAt          string                       `json:"generated_at"`
	FixtureRoot          string                       `json:"fixture_root"`
	Summary              firstIndexNorthStarSummary   `json:"summary"`
	Retrievals           []firstIndexRetrievalReport  `json:"retrievals"`
	ClassifierFixtures   []firstIndexClassifierReport `json:"classifiers,omitempty"`
	WeakSpots            []firstIndexWeakSpot         `json:"weak_spots,omitempty"`
	ThresholdFailures    []string                     `json:"threshold_failures,omitempty"`
	FailedThresholdCount int                          `json:"failed_threshold_count,omitempty"`
}

type firstIndexNorthStarSummary struct {
	MeanTokenReductionVsFullPlanning      float64 `json:"mean_token_reduction_vs_full_planning"`
	MeanTokenReductionVsQueryFileBaseline float64 `json:"mean_token_reduction_vs_query_file_baseline"`
	MeanArtifactPrecision                 float64 `json:"mean_artifact_precision"`
	MeanGradedPrecision                   float64 `json:"mean_graded_precision"`
	MeanPenalizedUtilityPrecision         float64 `json:"mean_penalized_utility_precision"`
	MeanArtifactRecall                    float64 `json:"mean_artifact_recall"`
	MeanMustHaveRecall                    float64 `json:"mean_must_have_recall"`
	ContextSufficiencyPassRate            float64 `json:"context_sufficiency_pass_rate"`
	DiscoveryCoverage                     float64 `json:"discovery_coverage"`
	SavedInputTokensVsFullPlanning        int     `json:"saved_input_tokens_vs_full_planning"`
	EstimatedInputUSDSaved                float64 `json:"estimated_input_usd_saved,omitempty"`
	ClassifierCases                       int     `json:"classifier_cases,omitempty"`
	ClassifierPassedCases                 int     `json:"classifier_passed_cases,omitempty"`
	ClassifierAccuracy                    float64 `json:"classifier_accuracy,omitempty"`
	ClassifierAmbiguityRate               float64 `json:"classifier_ambiguity_rate,omitempty"`
	ClassifierGenericFallbackRate         float64 `json:"classifier_generic_fallback_rate,omitempty"`
}

type firstIndexRetrievalReport struct {
	Fixture                          string                        `json:"fixture"`
	FixtureVersion                   string                        `json:"fixture_version,omitempty"`
	EvalStage                        string                        `json:"eval_stage,omitempty"`
	CorpusSource                     string                        `json:"corpus_source"`
	ProductPath                      string                        `json:"product_path"`
	CommandUnderTest                 string                        `json:"command_under_test,omitempty"`
	Retriever                        string                        `json:"retriever"`
	TokenCounter                     string                        `json:"token_counter"`
	PricingProfile                   string                        `json:"pricing_profile,omitempty"`
	ResultsFile                      string                        `json:"results_file,omitempty"`
	Cases                            int                           `json:"cases"`
	MeanTokenReductionVsFullPlanning float64                       `json:"mean_token_reduction_vs_full_planning"`
	MeanTokenReductionVsQueryFile    float64                       `json:"mean_token_reduction_vs_query_file_baseline"`
	MeanArtifactPrecision            float64                       `json:"mean_artifact_precision"`
	MeanGradedPrecision              float64                       `json:"mean_graded_precision"`
	MeanPenalizedUtilityPrecision    float64                       `json:"mean_penalized_utility_precision"`
	MeanArtifactRecall               float64                       `json:"mean_artifact_recall"`
	MeanMustHaveRecall               float64                       `json:"mean_must_have_recall"`
	ContextSufficiencyCases          int                           `json:"context_sufficiency_cases"`
	ContextSufficiencyPassed         int                           `json:"context_sufficiency_passed"`
	ContextSufficiencyPassRate       float64                       `json:"context_sufficiency_pass_rate"`
	DiscoveryCoverage                float64                       `json:"discovery_coverage"`
	RetrievalCoverageOfDiscovered    float64                       `json:"retrieval_coverage_of_discovered"`
	DevSpecsTokens                   int                           `json:"devspecs_tokens"`
	FullPlanningTokens               int                           `json:"full_planning_tokens"`
	SavedInputTokensVsFullPlanning   int                           `json:"saved_input_tokens_vs_full_planning"`
	PackedSectionArtifactCount       int                           `json:"packed_section_artifact_count,omitempty"`
	PackedSectionCount               int                           `json:"packed_section_count,omitempty"`
	SectionSelectedArtifactCount     int                           `json:"section_selected_artifact_count,omitempty"`
	SectionSelectedCount             int                           `json:"section_selected_count,omitempty"`
	FullFileArtifactCount            int                           `json:"full_file_artifact_count,omitempty"`
	TestCaseArtifactCount            int                           `json:"test_case_artifact_count,omitempty"`
	CodeCommentArtifactCount         int                           `json:"code_comment_artifact_count,omitempty"`
	AgentMetrics                     evalharness.AgentMetrics      `json:"agent_metrics"`
	LaneMetrics                      []evalharness.LaneMetric      `json:"lane_metrics,omitempty"`
	IndexCache                       *evalharness.IndexCacheReport `json:"index_cache,omitempty"`
	Budgets                          evalharness.BudgetReport      `json:"budgets,omitempty"`
	PhaseTelemetry                   []evalharness.PhaseTelemetry  `json:"phase_telemetry,omitempty"`
	InputUSDPer1M                    float64                       `json:"input_usd_per_1m,omitempty"`
	EstimatedInputUSDSaved           float64                       `json:"estimated_input_usd_saved,omitempty"`
	PlanningArtifactFiles            int                           `json:"planning_artifact_files"`
	PlanningArtifactTokens           int                           `json:"planning_artifact_tokens"`
	MarkdownFiles                    int                           `json:"markdown_files"`
	MarkdownTokens                   int                           `json:"markdown_tokens"`
	FullCandidateCorpusFiles         int                           `json:"full_candidate_corpus_files"`
	FullCandidateCorpusTokens        int                           `json:"full_candidate_corpus_tokens"`
	ExpectedRelevantCount            int                           `json:"expected_relevant_count"`
	ExpectedAvailableCount           int                           `json:"expected_available_count"`
	ExpectedMissingFromCorpusCount   int                           `json:"expected_missing_from_corpus_count"`
	MissedAfterDiscoveryCount        int                           `json:"missed_after_discovery_count"`
	WorstRecallCase                  string                        `json:"worst_recall_case,omitempty"`
	LargestTokenContextCase          string                        `json:"largest_token_context_case,omitempty"`
	FailedPerCaseThresholdCount      int                           `json:"failed_per_case_threshold_count,omitempty"`
}

type firstIndexClassifierReport struct {
	Fixture                 string                      `json:"fixture"`
	FixtureVersion          string                      `json:"fixture_version,omitempty"`
	EvalStage               string                      `json:"eval_stage,omitempty"`
	Evaluator               string                      `json:"evaluator"`
	ClassifierProfile       string                      `json:"classifier_profile"`
	ConfigVersion           int                         `json:"config_version"`
	ResultsFile             string                      `json:"results_file,omitempty"`
	Cases                   int                         `json:"cases"`
	PassedCases             int                         `json:"passed_cases"`
	Accuracy                float64                     `json:"accuracy"`
	SubformatFamilyCases    int                         `json:"subformat_family_cases"`
	SubformatFamilyAccuracy float64                     `json:"subformat_family_accuracy,omitempty"`
	DiscoveryCoverage       float64                     `json:"discovery_coverage"`
	AmbiguousCases          int                         `json:"ambiguous_cases"`
	AmbiguityRate           float64                     `json:"ambiguity_rate"`
	GenericFallbackCases    int                         `json:"generic_fallback_cases"`
	GenericFallbackRate     float64                     `json:"generic_fallback_rate"`
	RejectedCases           int                         `json:"rejected_cases"`
	RejectRate              float64                     `json:"reject_rate"`
	ReasonCoverageRate      float64                     `json:"reason_coverage_rate,omitempty"`
	ChildCandidateCoverage  float64                     `json:"child_candidate_coverage,omitempty"`
	ChildCandidateExpected  int                         `json:"child_candidate_expected,omitempty"`
	ChildCandidateMatched   int                         `json:"child_candidate_matched,omitempty"`
	Models                  []classify.EvalModelSummary `json:"models"`
	Confusions              []classify.EvalConfusion    `json:"confusions,omitempty"`
}

type firstIndexWeakSpot struct {
	Lane    string `json:"lane"`
	Fixture string `json:"fixture"`
	CaseID  string `json:"case_id,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func buildRetrievalEvalOptions(cmd *cobra.Command, asJSON, filesystem, indexed bool, commandUnderTest, findRuntime string, includeTests, includeCodeComments, disableSectionAwareRetrieval, experimentalBalancedEvidence, experimentalBudgetedPacking, experimentalConceptBackfill, experimentalGlossaryConcepts, experimentalTieredConceptOutput, experimentalAnchorFirstRanking bool, experimentalAnchorFirstMode string, packDiagnostics bool, evalIndexCacheDir string, refreshIndexCache bool, maxCorpusFiles, maxSourceFiles, maxTestCaseArtifacts, maxCodeComments, maxCaseSeconds, contextTokenBudget, progressIntervalSec int, minRecall, minMeanRecall, minMustRecall, minSufficiency, minReductionFull float64) (evalharness.Options, error) {
	normalizedAnchorMode := retrieval.NormalizeAnchorFirstMode(experimentalAnchorFirstMode)
	if normalizedAnchorMode == "" {
		return evalharness.Options{}, fmt.Errorf("unknown --experimental-anchor-first-mode; valid values: %s", strings.Join(retrieval.ValidAnchorFirstModes(), ", "))
	}
	opts := evalharness.Options{
		JSON:                            asJSON,
		TestCaseArtifacts:               includeTests,
		CodeCommentArtifacts:            includeCodeComments,
		DisableSectionAwareRetrieval:    disableSectionAwareRetrieval,
		ExperimentalBalancedEvidence:    experimentalBalancedEvidence,
		ExperimentalBudgetedPacking:     experimentalBudgetedPacking,
		ExperimentalConceptBackfill:     experimentalConceptBackfill,
		ExperimentalGlossaryConcepts:    experimentalGlossaryConcepts,
		ExperimentalTieredConceptOutput: experimentalTieredConceptOutput,
		ExperimentalAnchorFirstRanking:  experimentalAnchorFirstRanking,
		ExperimentalAnchorFirstMode:     normalizedAnchorMode,
		PackDiagnostics:                 packDiagnostics,
		ContextTokenBudget:              contextTokenBudget,
		IndexCacheDir:                   strings.TrimSpace(evalIndexCacheDir),
		RefreshIndexCache:               refreshIndexCache,
		MaxCorpusFiles:                  maxCorpusFiles,
		MaxSourceFiles:                  maxSourceFiles,
		MaxTestCaseArtifacts:            maxTestCaseArtifacts,
		MaxCodeComments:                 maxCodeComments,
		MaxCaseSeconds:                  maxCaseSeconds,
	}
	if progressIntervalSec > 0 {
		opts.ProgressWriter = cmd.ErrOrStderr()
		opts.ProgressInterval = time.Duration(progressIntervalSec) * time.Second
	}
	if opts.MaxCorpusFiles < 0 || opts.MaxSourceFiles < 0 || opts.MaxTestCaseArtifacts < 0 || opts.MaxCodeComments < 0 || opts.MaxCaseSeconds < 0 || opts.ContextTokenBudget < 0 || progressIntervalSec < 0 {
		return evalharness.Options{}, fmt.Errorf("eval budget flags must be non-negative")
	}
	if opts.ExperimentalBudgetedPacking && opts.ContextTokenBudget == 0 {
		opts.ContextTokenBudget = 8192
	}
	if filesystem {
		opts.CorpusSource = evalharness.CorpusSourceFilesystemFixture
	} else if indexed {
		opts.CorpusSource = evalharness.CorpusSourceSQLiteIndex
	}
	if strings.TrimSpace(commandUnderTest) != "" {
		if filesystem {
			return evalharness.Options{}, fmt.Errorf("--command requires the indexed eval corpus; remove --filesystem")
		}
		normalized, err := normalizeEvalCommand(commandUnderTest)
		if err != nil {
			return evalharness.Options{}, err
		}
		opts.CommandUnderTest = normalized
		normalizedRuntime, err := indexquery.ParseRuntimeMode(findRuntime)
		if err != nil {
			return evalharness.Options{}, err
		}
		opts.FindRuntime = string(normalizedRuntime)
		opts.CommandRunner = func(fixtureAbs string, cases []evalharness.CaseSpec) (map[string]evalharness.CommandCaseOutput, error) {
			return runLiveCommandEval(normalized, string(normalizedRuntime), fixtureAbs, cases, includeTests, includeCodeComments, experimentalAnchorFirstRanking, normalizedAnchorMode)
		}
	} else if strings.TrimSpace(findRuntime) != "" {
		if _, err := indexquery.ParseRuntimeMode(findRuntime); err != nil {
			return evalharness.Options{}, err
		}
	}
	if cmd.Flags().Changed("min-recall") {
		opts.MinRecall = &minRecall
	}
	if cmd.Flags().Changed("min-mean-recall") {
		opts.MinMeanRecall = &minMeanRecall
	}
	if cmd.Flags().Changed("min-must-recall") {
		opts.MinMustRecall = &minMustRecall
	}
	if cmd.Flags().Changed("min-sufficiency-rate") {
		opts.MinSufficiency = &minSufficiency
	}
	if cmd.Flags().Changed("min-reduction-full") {
		opts.MinReductionFull = &minReductionFull
	}
	return opts, nil
}

func runFirstIndexReport(fixture string, opts evalharness.Options, reportOpts firstIndexReportOptions) (*firstIndexReport, error) {
	if reportOpts.GeneratedAt.IsZero() {
		reportOpts.GeneratedAt = nowUTC()
	}
	retrievalResult, err := evalharness.Run(fixture, opts)
	if err != nil {
		return nil, err
	}
	summaryFailures := evalharness.CheckSummaryThresholds(retrievalResult, opts)
	if !reportOpts.NoSave {
		resultsFile, err := saveEvalResult(retrievalResult, reportOpts.ResultsDir, reportOpts.GeneratedAt)
		if err != nil {
			return nil, err
		}
		retrievalResult.ResultsFile = filepath.ToSlash(resultsFile)
	}

	classifierFixtures := firstIndexClassifierFixtures(fixture, reportOpts.ClassifierFixtures)
	classifierResults := make([]*classify.EvalResult, 0, len(classifierFixtures))
	for _, classifierFixture := range classifierFixtures {
		classifierResult, err := classify.RunEval(classifierFixture, classify.EvalOptions{})
		if err != nil {
			return nil, fmt.Errorf("classifier fixture %s: %w", classifierFixture, err)
		}
		if !reportOpts.NoSave {
			resultsFile, err := saveClassifierEvalResult(classifierResult, reportOpts.ResultsDir, reportOpts.GeneratedAt)
			if err != nil {
				return nil, err
			}
			classifierResult.ResultsFile = filepath.ToSlash(resultsFile)
		}
		classifierResults = append(classifierResults, classifierResult)
	}
	return buildFirstIndexReport(retrievalResult, classifierResults, summaryFailures, reportOpts), nil
}

func runFirstIndexBatchReport(root string, opts evalharness.Options, reportOpts firstIndexReportOptions) (*firstIndexBatchReport, error) {
	if reportOpts.GeneratedAt.IsZero() {
		reportOpts.GeneratedAt = nowUTC()
	}
	fixtures, err := discoverFirstIndexBatchFixtures(root)
	if err != nil {
		return nil, err
	}
	if len(fixtures) == 0 {
		return nil, fmt.Errorf("no child fixtures containing cases.yaml found under %s", root)
	}

	retrievalResults := make([]*evalharness.Result, 0, len(fixtures))
	retrievalReports := make([]firstIndexRetrievalReport, 0, len(fixtures))
	var summaryFailures []string
	for _, fixture := range fixtures {
		result, err := evalharness.Run(fixture, opts)
		if err != nil {
			return nil, fmt.Errorf("fixture %s: %w", fixture, err)
		}
		failures := evalharness.CheckSummaryThresholds(result, opts)
		for _, failure := range failures {
			summaryFailures = append(summaryFailures, fmt.Sprintf("%s: %s", fixture, failure))
		}
		if !reportOpts.NoSave {
			resultsFile, err := saveEvalResult(result, reportOpts.ResultsDir, reportOpts.GeneratedAt)
			if err != nil {
				return nil, err
			}
			result.ResultsFile = filepath.ToSlash(resultsFile)
		}
		retrievalResults = append(retrievalResults, result)
		retrievalReports = append(retrievalReports, buildFirstIndexRetrievalReport(result, reportOpts.InputUSDPer1M))
	}

	classifierResults := make([]*classify.EvalResult, 0, len(reportOpts.ClassifierFixtures))
	for _, classifierFixture := range firstIndexClassifierFixtures("", reportOpts.ClassifierFixtures) {
		classifierResult, err := classify.RunEval(classifierFixture, classify.EvalOptions{})
		if err != nil {
			return nil, fmt.Errorf("classifier fixture %s: %w", classifierFixture, err)
		}
		if !reportOpts.NoSave {
			resultsFile, err := saveClassifierEvalResult(classifierResult, reportOpts.ResultsDir, reportOpts.GeneratedAt)
			if err != nil {
				return nil, err
			}
			classifierResult.ResultsFile = filepath.ToSlash(resultsFile)
		}
		classifierResults = append(classifierResults, classifierResult)
	}

	return buildFirstIndexBatchReport(root, retrievalResults, retrievalReports, classifierResults, summaryFailures, reportOpts), nil
}

func discoverFirstIndexBatchFixtures(root string) ([]string, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat fixture root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("fixture root must be a directory: %s", root)
	}
	var roots []string
	if firstIndexHasCases(root) {
		roots = append(roots, root)
	}
	for _, childRoot := range []string{root, filepath.Join(root, "repos")} {
		info, err := os.Stat(childRoot)
		if err != nil || !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(childRoot)
		if err != nil {
			return nil, fmt.Errorf("read fixture root %s: %w", childRoot, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(childRoot, entry.Name())
			if firstIndexHasCases(candidate) {
				roots = append(roots, candidate)
			}
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(roots))
	for _, fixture := range roots {
		abs, err := filepath.Abs(fixture)
		if err != nil {
			return nil, fmt.Errorf("resolve fixture %s: %w", fixture, err)
		}
		key := filepath.Clean(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fixture)
	}
	sort.Strings(out)
	return out, nil
}

func firstIndexHasCases(fixture string) bool {
	info, err := os.Stat(filepath.Join(fixture, "cases.yaml"))
	return err == nil && !info.IsDir()
}

func firstIndexClassifierFixtures(primary string, explicit []string) []string {
	if len(explicit) == 0 {
		if firstIndexHasClassifierCases(primary) {
			return []string{primary}
		}
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(explicit))
	for _, fixture := range explicit {
		fixture = strings.TrimSpace(fixture)
		if fixture == "" || seen[fixture] {
			continue
		}
		seen[fixture] = true
		out = append(out, fixture)
	}
	return out
}

func firstIndexHasClassifierCases(fixture string) bool {
	info, err := os.Stat(filepath.Join(fixture, "classifier_cases.yaml"))
	return err == nil && !info.IsDir()
}

func buildFirstIndexReport(retrievalResult *evalharness.Result, classifierResults []*classify.EvalResult, summaryFailures []string, opts firstIndexReportOptions) *firstIndexReport {
	retrievalReport := buildFirstIndexRetrievalReport(retrievalResult, opts.InputUSDPer1M)
	classifierReports := make([]firstIndexClassifierReport, 0, len(classifierResults))
	var classifierCases, classifierPassed, ambiguousCases, genericFallbackCases int
	for _, result := range classifierResults {
		report := buildFirstIndexClassifierReport(result)
		classifierReports = append(classifierReports, report)
		classifierCases += report.Cases
		classifierPassed += report.PassedCases
		ambiguousCases += report.AmbiguousCases
		genericFallbackCases += report.GenericFallbackCases
	}
	summary := firstIndexNorthStarSummary{
		MeanTokenReductionVsFullPlanning: retrievalReport.MeanTokenReductionVsFullPlanning,
		MeanArtifactPrecision:            retrievalReport.MeanArtifactPrecision,
		MeanArtifactRecall:               retrievalReport.MeanArtifactRecall,
		MeanMustHaveRecall:               retrievalReport.MeanMustHaveRecall,
		ContextSufficiencyPassRate:       retrievalReport.ContextSufficiencyPassRate,
		DiscoveryCoverage:                retrievalReport.DiscoveryCoverage,
		SavedInputTokensVsFullPlanning:   retrievalReport.SavedInputTokensVsFullPlanning,
		EstimatedInputUSDSaved:           retrievalReport.EstimatedInputUSDSaved,
		ClassifierCases:                  classifierCases,
		ClassifierPassedCases:            classifierPassed,
	}
	if classifierCases > 0 {
		summary.ClassifierAccuracy = float64(classifierPassed) / float64(classifierCases)
		summary.ClassifierAmbiguityRate = float64(ambiguousCases) / float64(classifierCases)
		summary.ClassifierGenericFallbackRate = float64(genericFallbackCases) / float64(classifierCases)
	}
	report := &firstIndexReport{
		GeneratedAt:       opts.GeneratedAt.UTC().Format(time.RFC3339),
		Summary:           summary,
		Retrieval:         retrievalReport,
		Classifiers:       classifierReports,
		WeakSpots:         firstIndexWeakSpots(retrievalResult, classifierResults),
		ThresholdFailures: append([]string(nil), summaryFailures...),
	}
	report.FailedThresholdCount = retrievalResult.Summary.FailedThresholdCount + len(summaryFailures)
	return report
}

func buildFirstIndexBatchReport(root string, retrievalResults []*evalharness.Result, retrievalReports []firstIndexRetrievalReport, classifierResults []*classify.EvalResult, summaryFailures []string, opts firstIndexReportOptions) *firstIndexBatchReport {
	classifierReports := make([]firstIndexClassifierReport, 0, len(classifierResults))
	var classifierCases, classifierPassed, ambiguousCases, genericFallbackCases int
	for _, result := range classifierResults {
		report := buildFirstIndexClassifierReport(result)
		classifierReports = append(classifierReports, report)
		classifierCases += report.Cases
		classifierPassed += report.PassedCases
		ambiguousCases += report.AmbiguousCases
		genericFallbackCases += report.GenericFallbackCases
	}

	var (
		cases, contextCases, contextPassed                           int
		devspecsTokens, fullPlanningTokens, savedTokens              int
		expectedRelevant, expectedAvailable                          int
		failedThresholds                                             int
		weightedReduction, weightedQueryReduction, weightedPrecision float64
		weightedGradedPrecision, weightedPenalizedUtility            float64
		weightedRecall, weightedMust                                 float64
		weakSpots                                                    []firstIndexWeakSpot
	)
	for i, result := range retrievalResults {
		report := retrievalReports[i]
		n := report.Cases
		if n == 0 {
			continue
		}
		cases += n
		contextCases += report.ContextSufficiencyCases
		contextPassed += report.ContextSufficiencyPassed
		devspecsTokens += report.DevSpecsTokens
		fullPlanningTokens += report.FullPlanningTokens
		savedTokens += report.SavedInputTokensVsFullPlanning
		expectedRelevant += report.ExpectedRelevantCount
		expectedAvailable += report.ExpectedAvailableCount
		failedThresholds += report.FailedPerCaseThresholdCount
		weightedReduction += report.MeanTokenReductionVsFullPlanning * float64(n)
		weightedQueryReduction += report.MeanTokenReductionVsQueryFile * float64(n)
		weightedPrecision += report.MeanArtifactPrecision * float64(n)
		weightedGradedPrecision += report.MeanGradedPrecision * float64(n)
		weightedPenalizedUtility += report.MeanPenalizedUtilityPrecision * float64(n)
		weightedRecall += report.MeanArtifactRecall * float64(n)
		weightedMust += report.MeanMustHaveRecall * float64(n)
		weakSpots = append(weakSpots, firstIndexWeakSpots(result, nil)...)
	}

	summary := firstIndexNorthStarSummary{
		SavedInputTokensVsFullPlanning: savedTokens,
		EstimatedInputUSDSaved:         0,
		ClassifierCases:                classifierCases,
		ClassifierPassedCases:          classifierPassed,
	}
	if cases > 0 {
		summary.MeanTokenReductionVsFullPlanning = weightedReduction / float64(cases)
		summary.MeanTokenReductionVsQueryFileBaseline = weightedQueryReduction / float64(cases)
		summary.MeanArtifactPrecision = weightedPrecision / float64(cases)
		summary.MeanGradedPrecision = weightedGradedPrecision / float64(cases)
		summary.MeanPenalizedUtilityPrecision = weightedPenalizedUtility / float64(cases)
		summary.MeanArtifactRecall = weightedRecall / float64(cases)
		summary.MeanMustHaveRecall = weightedMust / float64(cases)
	}
	if contextCases > 0 {
		summary.ContextSufficiencyPassRate = float64(contextPassed) / float64(contextCases)
	}
	if expectedRelevant > 0 {
		summary.DiscoveryCoverage = float64(expectedAvailable) / float64(expectedRelevant)
	}
	if opts.InputUSDPer1M > 0 {
		summary.EstimatedInputUSDSaved = float64(savedTokens) / 1_000_000.0 * opts.InputUSDPer1M
	}
	if classifierCases > 0 {
		summary.ClassifierAccuracy = float64(classifierPassed) / float64(classifierCases)
		summary.ClassifierAmbiguityRate = float64(ambiguousCases) / float64(classifierCases)
		summary.ClassifierGenericFallbackRate = float64(genericFallbackCases) / float64(classifierCases)
	}

	report := &firstIndexBatchReport{
		GeneratedAt:        opts.GeneratedAt.UTC().Format(time.RFC3339),
		FixtureRoot:        root,
		Summary:            summary,
		Retrievals:         retrievalReports,
		ClassifierFixtures: classifierReports,
		WeakSpots:          capFirstIndexWeakSpots(weakSpots, 16),
		ThresholdFailures:  append([]string(nil), summaryFailures...),
	}
	report.FailedThresholdCount = failedThresholds + len(summaryFailures)
	return report
}

func capFirstIndexWeakSpots(in []firstIndexWeakSpot, limit int) []firstIndexWeakSpot {
	if len(in) <= limit {
		return in
	}
	return append([]firstIndexWeakSpot(nil), in[:limit]...)
}

func buildFirstIndexRetrievalReport(r *evalharness.Result, inputUSDPer1M float64) firstIndexRetrievalReport {
	var devspecsTokens, fullPlanningTokens int
	var packedSectionArtifacts, packedSections, sectionSelectedArtifacts, sectionSelected, fullFileArtifacts, testCaseArtifacts, codeCommentArtifacts int
	for _, c := range r.Cases {
		devspecsTokens += c.DevSpecsTokens
		fullPlanningTokens += c.FullPlanningTokens
		packedSectionArtifacts += len(c.PackedSectionArtifacts)
		packedSections += c.PackedSectionCount
		sectionSelectedArtifacts += len(c.SectionSelectedArtifacts)
		sectionSelected += c.SectionSelectedCount
		fullFileArtifacts += c.FullFileArtifactCount
		testCaseArtifacts += c.TestCaseArtifactCount
		codeCommentArtifacts += c.CodeCommentArtifactCount
	}
	savedTokens := fullPlanningTokens - devspecsTokens
	if savedTokens < 0 {
		savedTokens = 0
	}
	estimatedSaved := 0.0
	if inputUSDPer1M > 0 {
		estimatedSaved = float64(savedTokens) / 1_000_000.0 * inputUSDPer1M
	}
	return firstIndexRetrievalReport{
		Fixture:                          r.Fixture,
		FixtureVersion:                   r.FixtureVersion,
		EvalStage:                        r.EvalStage,
		CorpusSource:                     r.CorpusSource,
		ProductPath:                      r.ProductPath,
		CommandUnderTest:                 r.CommandUnderTest,
		Retriever:                        r.Retriever,
		TokenCounter:                     r.TokenCounter,
		PricingProfile:                   r.PricingProfile.Name,
		ResultsFile:                      r.ResultsFile,
		Cases:                            r.Summary.Cases,
		MeanTokenReductionVsFullPlanning: r.Summary.MeanTokenReductionVsFullPlanning,
		MeanTokenReductionVsQueryFile:    r.Summary.MeanTokenReductionVsQueryFileBaseline,
		MeanArtifactPrecision:            r.Summary.MeanArtifactPrecision,
		MeanGradedPrecision:              r.Summary.MeanGradedPrecision,
		MeanPenalizedUtilityPrecision:    r.Summary.MeanPenalizedUtilityPrecision,
		MeanArtifactRecall:               r.Summary.MeanArtifactRecall,
		MeanMustHaveRecall:               r.Summary.MeanMustHaveRecall,
		ContextSufficiencyCases:          r.Summary.ContextSufficiencyCases,
		ContextSufficiencyPassed:         r.Summary.ContextSufficiencyPassed,
		ContextSufficiencyPassRate:       r.Summary.ContextSufficiencyPassRate,
		DiscoveryCoverage:                r.Diagnostics.DiscoveryCoverage,
		RetrievalCoverageOfDiscovered:    r.Diagnostics.RetrievalCoverageOfDiscovered,
		DevSpecsTokens:                   devspecsTokens,
		FullPlanningTokens:               fullPlanningTokens,
		SavedInputTokensVsFullPlanning:   savedTokens,
		PackedSectionArtifactCount:       packedSectionArtifacts,
		PackedSectionCount:               packedSections,
		SectionSelectedArtifactCount:     sectionSelectedArtifacts,
		SectionSelectedCount:             sectionSelected,
		FullFileArtifactCount:            fullFileArtifacts,
		TestCaseArtifactCount:            testCaseArtifacts,
		CodeCommentArtifactCount:         codeCommentArtifacts,
		AgentMetrics:                     r.AgentMetrics,
		LaneMetrics:                      append([]evalharness.LaneMetric(nil), r.LaneMetrics...),
		IndexCache:                       r.IndexCache,
		Budgets:                          r.Budgets,
		PhaseTelemetry:                   append([]evalharness.PhaseTelemetry(nil), r.PhaseTelemetry...),
		InputUSDPer1M:                    inputUSDPer1M,
		EstimatedInputUSDSaved:           estimatedSaved,
		PlanningArtifactFiles:            r.Corpus.PlanningArtifacts.Files,
		PlanningArtifactTokens:           r.Corpus.PlanningArtifacts.Tokens,
		MarkdownFiles:                    r.Corpus.MarkdownFiles.Files,
		MarkdownTokens:                   r.Corpus.MarkdownFiles.Tokens,
		FullCandidateCorpusFiles:         r.Corpus.FullCandidateCorpus.Files,
		FullCandidateCorpusTokens:        r.Corpus.FullCandidateCorpus.Tokens,
		ExpectedRelevantCount:            r.Diagnostics.ExpectedRelevantCount,
		ExpectedAvailableCount:           r.Diagnostics.ExpectedAvailableCount,
		ExpectedMissingFromCorpusCount:   r.Diagnostics.ExpectedMissingFromCorpusCount,
		MissedAfterDiscoveryCount:        r.Diagnostics.MissedAfterDiscoveryCount,
		WorstRecallCase:                  r.Summary.WorstRecallCase,
		LargestTokenContextCase:          r.Summary.LargestTokenContextCase,
		FailedPerCaseThresholdCount:      r.Summary.FailedThresholdCount,
	}
}

func buildFirstIndexClassifierReport(r *classify.EvalResult) firstIndexClassifierReport {
	return firstIndexClassifierReport{
		Fixture:                 r.Fixture,
		FixtureVersion:          r.FixtureVersion,
		EvalStage:               r.EvalStage,
		Evaluator:               r.Evaluator,
		ClassifierProfile:       r.ClassifierProfile,
		ConfigVersion:           r.ConfigVersion,
		ResultsFile:             r.ResultsFile,
		Cases:                   r.Summary.Cases,
		PassedCases:             r.Summary.PassedCases,
		Accuracy:                r.Summary.Accuracy,
		SubformatFamilyCases:    r.Summary.SubformatFamilyCases,
		SubformatFamilyAccuracy: r.Summary.SubformatFamilyAccuracy,
		DiscoveryCoverage:       r.Summary.DiscoveryCoverage,
		AmbiguousCases:          r.Summary.AmbiguousCases,
		AmbiguityRate:           r.Summary.AmbiguityRate,
		GenericFallbackCases:    r.Summary.GenericFallbackCases,
		GenericFallbackRate:     r.Summary.GenericFallbackRate,
		RejectedCases:           r.Summary.RejectedCases,
		RejectRate:              r.Summary.RejectRate,
		ReasonCoverageRate:      r.Summary.ReasonCoverageRate,
		ChildCandidateCoverage:  r.Summary.ChildCandidateCoverage,
		ChildCandidateExpected:  r.Summary.ChildCandidateExpected,
		ChildCandidateMatched:   r.Summary.ChildCandidateMatched,
		Models:                  append([]classify.EvalModelSummary(nil), r.Models...),
		Confusions:              append([]classify.EvalConfusion(nil), r.Confusions...),
	}
}

func firstIndexWeakSpots(retrievalResult *evalharness.Result, classifierResults []*classify.EvalResult) []firstIndexWeakSpot {
	var out []firstIndexWeakSpot
	for _, c := range retrievalResult.Cases {
		var problems []string
		if c.ArtifactPrecision < 0.75 {
			problems = append(problems, fmt.Sprintf("precision %s", firstIndexPct(c.ArtifactPrecision)))
		}
		if c.ArtifactRecall < 1.0 {
			problems = append(problems, fmt.Sprintf("recall %s", firstIndexPct(c.ArtifactRecall)))
		}
		if c.MustExpectedCount > 0 && c.MustHaveRecall < 1.0 {
			problems = append(problems, fmt.Sprintf("must-have recall %s", firstIndexPct(c.MustHaveRecall)))
		}
		if c.ContextSufficiency.Configured && !c.ContextSufficiency.Passed {
			problems = append(problems, "context sufficiency failed")
		}
		if len(c.ThresholdFailures) > 0 {
			problems = append(problems, strings.Join(c.ThresholdFailures, "; "))
		}
		if len(problems) == 0 {
			continue
		}
		out = append(out, firstIndexWeakSpot{
			Lane:    "retrieval",
			Fixture: retrievalResult.Fixture,
			CaseID:  c.ID,
			Message: strings.Join(problems, "; "),
		})
		if len(out) >= 8 {
			break
		}
	}
	for _, result := range classifierResults {
		for _, c := range result.Cases {
			if c.Passed {
				continue
			}
			out = append(out, firstIndexWeakSpot{
				Lane:    "classifier",
				Fixture: result.Fixture,
				CaseID:  c.ID,
				Path:    c.Path,
				Message: fmt.Sprintf("expected %s, got %s", c.ExpectedClassifier, c.ActualClassifier),
			})
			if len(out) >= 16 {
				return out
			}
		}
	}
	return out
}

func formatFirstIndexReportJSON(report *firstIndexReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func formatFirstIndexBatchReportJSON(report *firstIndexBatchReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

func formatFirstIndexBatchReportText(report *firstIndexBatchReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "DevSpecs First-Index Batch Eval Report")
	fmt.Fprintf(&b, "Generated: %s\n\n", report.GeneratedAt)

	totalCases := 0
	contextCases := 0
	contextPassed := 0
	for _, retrieval := range report.Retrievals {
		totalCases += retrieval.Cases
		contextCases += retrieval.ContextSufficiencyCases
		contextPassed += retrieval.ContextSufficiencyPassed
	}

	fmt.Fprintln(&b, "North Star")
	fmt.Fprintf(&b, "- Fixtures: %d under %s\n", len(report.Retrievals), report.FixtureRoot)
	fmt.Fprintf(&b, "- Token reduction: %s mean vs full planning corpus; saved %s input tokens across %d retrieval cases\n",
		firstIndexPct(report.Summary.MeanTokenReductionVsFullPlanning),
		firstIndexComma(report.Summary.SavedInputTokensVsFullPlanning),
		totalCases)
	fmt.Fprintf(&b, "- Query-search token reduction: %s mean vs query file baseline\n",
		firstIndexPct(report.Summary.MeanTokenReductionVsQueryFileBaseline))
	if report.Summary.EstimatedInputUSDSaved > 0 {
		fmt.Fprintf(&b, "- Estimated input cost saved: $%.4f\n", report.Summary.EstimatedInputUSDSaved)
	}
	fmt.Fprintf(&b, "- Retrieval: precision %s / graded precision %s / recall %s / must-have recall %s\n",
		firstIndexPct(report.Summary.MeanArtifactPrecision),
		firstIndexPct(report.Summary.MeanGradedPrecision),
		firstIndexPct(report.Summary.MeanArtifactRecall),
		firstIndexPct(report.Summary.MeanMustHaveRecall))
	fmt.Fprintf(&b, "- Sufficiency: %d/%d = %s\n",
		contextPassed,
		contextCases,
		firstIndexPct(report.Summary.ContextSufficiencyPassRate))
	fmt.Fprintf(&b, "- Agent: must-hit@3 %s\n", firstIndexPct(meanRetrievalMustHitAt3(report.Retrievals)))
	fmt.Fprintf(&b, "- Discovery: %s\n", firstIndexPct(report.Summary.DiscoveryCoverage))
	if report.Summary.ClassifierCases > 0 {
		fmt.Fprintf(&b, "- Classifier: %d/%d = %s; ambiguity %s; generic fallback %s\n",
			report.Summary.ClassifierPassedCases,
			report.Summary.ClassifierCases,
			firstIndexPct(report.Summary.ClassifierAccuracy),
			firstIndexPct(report.Summary.ClassifierAmbiguityRate),
			firstIndexPct(report.Summary.ClassifierGenericFallbackRate))
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Retrieval Fixtures")
	for _, retrieval := range report.Retrievals {
		fmt.Fprintf(&b, "- %s: cases %d, precision %s, graded %s, recall %s, must %s, sufficiency %d/%d, discovery %s\n",
			retrieval.Fixture,
			retrieval.Cases,
			firstIndexPct(retrieval.MeanArtifactPrecision),
			firstIndexPct(retrieval.MeanGradedPrecision),
			firstIndexPct(retrieval.MeanArtifactRecall),
			firstIndexPct(retrieval.MeanMustHaveRecall),
			retrieval.ContextSufficiencyPassed,
			retrieval.ContextSufficiencyCases,
			firstIndexPct(retrieval.DiscoveryCoverage))
		if retrieval.PackedSectionArtifactCount > 0 {
			fmt.Fprintf(&b, "  Section packing: %d artifacts / %d sections\n",
				retrieval.PackedSectionArtifactCount,
				retrieval.PackedSectionCount)
		}
		if retrieval.SectionSelectedArtifactCount > 0 {
			fmt.Fprintf(&b, "  Section retrieval: %d artifacts / %d section hits\n",
				retrieval.SectionSelectedArtifactCount,
				retrieval.SectionSelectedCount)
		}
		if retrieval.TestCaseArtifactCount > 0 {
			fmt.Fprintf(&b, "  Test-case artifacts included: %d\n", retrieval.TestCaseArtifactCount)
		}
		fmt.Fprintf(&b, "  Agent: must-hit@3 %s, first useful rank %.2f\n",
			firstIndexPct(retrieval.AgentMetrics.MustHitAt3),
			retrieval.AgentMetrics.MeanFirstUsefulRank)
	}
	fmt.Fprintln(&b)

	if len(report.ClassifierFixtures) > 0 {
		fmt.Fprintln(&b, "Classifier Fixtures")
		for _, classifier := range report.ClassifierFixtures {
			fmt.Fprintf(&b, "- %s: %d/%d = %s\n",
				classifier.Fixture,
				classifier.PassedCases,
				classifier.Cases,
				firstIndexPct(classifier.Accuracy))
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "Residual Risks")
	if len(report.WeakSpots) == 0 && len(report.ThresholdFailures) == 0 {
		fmt.Fprintln(&b, "- None from this report.")
		return b.String()
	}
	for _, weak := range report.WeakSpots {
		caseRef := weak.CaseID
		if weak.Path != "" {
			caseRef += " (" + weak.Path + ")"
		}
		if caseRef == "" {
			caseRef = weak.Fixture
		}
		fmt.Fprintf(&b, "- %s: %s\n", caseRef, weak.Message)
	}
	for _, failure := range report.ThresholdFailures {
		fmt.Fprintf(&b, "- Threshold failure: %s\n", failure)
	}
	return b.String()
}

func formatFirstIndexReportText(report *firstIndexReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "DevSpecs First-Index Eval Report")
	fmt.Fprintf(&b, "Generated: %s\n\n", report.GeneratedAt)

	fmt.Fprintln(&b, "North Star")
	fmt.Fprintf(&b, "- Token reduction: %s mean vs full planning corpus; saved %s input tokens across %d retrieval cases\n",
		firstIndexPct(report.Summary.MeanTokenReductionVsFullPlanning),
		firstIndexComma(report.Summary.SavedInputTokensVsFullPlanning),
		report.Retrieval.Cases)
	fmt.Fprintf(&b, "- Query-search token reduction: %s mean vs query file baseline\n",
		firstIndexPct(report.Summary.MeanTokenReductionVsQueryFileBaseline))
	if report.Summary.EstimatedInputUSDSaved > 0 {
		fmt.Fprintf(&b, "- Estimated input cost saved: $%.4f at $%.4f / 1M input tokens\n",
			report.Summary.EstimatedInputUSDSaved,
			report.Retrieval.InputUSDPer1M)
	}
	fmt.Fprintf(&b, "- Retrieval: precision %s / graded precision %s / recall %s / must-have recall %s\n",
		firstIndexPct(report.Summary.MeanArtifactPrecision),
		firstIndexPct(report.Summary.MeanGradedPrecision),
		firstIndexPct(report.Summary.MeanArtifactRecall),
		firstIndexPct(report.Summary.MeanMustHaveRecall))
	fmt.Fprintf(&b, "- Sufficiency: %d/%d = %s\n",
		report.Retrieval.ContextSufficiencyPassed,
		report.Retrieval.ContextSufficiencyCases,
		firstIndexPct(report.Summary.ContextSufficiencyPassRate))
	fmt.Fprintf(&b, "- Agent: must-hit@3 %s, first useful rank %.2f\n",
		firstIndexPct(report.Retrieval.AgentMetrics.MustHitAt3),
		report.Retrieval.AgentMetrics.MeanFirstUsefulRank)
	fmt.Fprintf(&b, "- Discovery: %s\n", firstIndexPct(report.Summary.DiscoveryCoverage))
	if report.Summary.ClassifierCases > 0 {
		fmt.Fprintf(&b, "- Classifier: %d/%d = %s; ambiguity %s; generic fallback %s\n",
			report.Summary.ClassifierPassedCases,
			report.Summary.ClassifierCases,
			firstIndexPct(report.Summary.ClassifierAccuracy),
			firstIndexPct(report.Summary.ClassifierAmbiguityRate),
			firstIndexPct(report.Summary.ClassifierGenericFallbackRate))
	} else {
		fmt.Fprintln(&b, "- Classifier: not run")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Retrieval And Tokens")
	fmt.Fprintf(&b, "- Fixture: %s\n", report.Retrieval.Fixture)
	fmt.Fprintf(&b, "- Fixture version: %s\n", report.Retrieval.FixtureVersion)
	fmt.Fprintf(&b, "- Eval stage: %s\n", report.Retrieval.EvalStage)
	fmt.Fprintf(&b, "- Corpus source: %s\n", report.Retrieval.CorpusSource)
	fmt.Fprintf(&b, "- Product path: %s\n", report.Retrieval.ProductPath)
	if report.Retrieval.CommandUnderTest != "" {
		fmt.Fprintf(&b, "- Command under test: %s\n", report.Retrieval.CommandUnderTest)
	}
	fmt.Fprintf(&b, "- Retriever: %s\n", report.Retrieval.Retriever)
	fmt.Fprintf(&b, "- Token counter: %s\n", report.Retrieval.TokenCounter)
	if report.Retrieval.ResultsFile != "" {
		fmt.Fprintf(&b, "- Results file: %s\n", report.Retrieval.ResultsFile)
	}
	fmt.Fprintf(&b, "- DevSpecs context tokens: %s\n", firstIndexComma(report.Retrieval.DevSpecsTokens))
	fmt.Fprintf(&b, "- Full planning tokens: %s\n", firstIndexComma(report.Retrieval.FullPlanningTokens))
	if report.Retrieval.PackedSectionArtifactCount > 0 {
		fmt.Fprintf(&b, "- Section packing: %d artifacts / %d sections; %d full-file markdown artifacts\n",
			report.Retrieval.PackedSectionArtifactCount,
			report.Retrieval.PackedSectionCount,
			report.Retrieval.FullFileArtifactCount)
	}
	if report.Retrieval.SectionSelectedArtifactCount > 0 {
		fmt.Fprintf(&b, "- Section retrieval: %d artifacts / %d section hits\n",
			report.Retrieval.SectionSelectedArtifactCount,
			report.Retrieval.SectionSelectedCount)
	}
	if report.Retrieval.TestCaseArtifactCount > 0 {
		fmt.Fprintf(&b, "- Test-case artifacts included: %d\n", report.Retrieval.TestCaseArtifactCount)
	}
	if report.Retrieval.CodeCommentArtifactCount > 0 {
		fmt.Fprintf(&b, "- Code-comment artifacts included: %d\n", report.Retrieval.CodeCommentArtifactCount)
	}
	if len(report.Retrieval.LaneMetrics) > 0 {
		fmt.Fprintln(&b, "- Lane metrics:")
		for _, lane := range report.Retrieval.LaneMetrics {
			fmt.Fprintf(&b, "  - %s: precision %s / graded %s / recall %s / included %d\n",
				lane.Lane,
				firstIndexPct(lane.StrictPrecision),
				firstIndexPct(lane.GradedPrecision),
				firstIndexPct(lane.Recall),
				lane.IncludedArtifacts)
		}
	}
	fmt.Fprintf(&b, "- Planning corpus: %d files / %s tokens\n", report.Retrieval.PlanningArtifactFiles, firstIndexComma(report.Retrieval.PlanningArtifactTokens))
	fmt.Fprintf(&b, "- Full candidate corpus: %d files / %s tokens\n", report.Retrieval.FullCandidateCorpusFiles, firstIndexComma(report.Retrieval.FullCandidateCorpusTokens))
	fmt.Fprintf(&b, "- Expected artifacts available: %d/%d = %s\n",
		report.Retrieval.ExpectedAvailableCount,
		report.Retrieval.ExpectedRelevantCount,
		firstIndexPct(report.Retrieval.DiscoveryCoverage))
	fmt.Fprintf(&b, "- Retrieval coverage of discovered expected artifacts: %s\n", firstIndexPct(report.Retrieval.RetrievalCoverageOfDiscovered))
	fmt.Fprintln(&b)

	if len(report.Classifiers) > 0 {
		fmt.Fprintln(&b, "Classifier Fixtures")
		for _, classifier := range report.Classifiers {
			fmt.Fprintf(&b, "- Fixture: %s\n", classifier.Fixture)
			fmt.Fprintf(&b, "  Accuracy: %d/%d = %s\n", classifier.PassedCases, classifier.Cases, firstIndexPct(classifier.Accuracy))
			fmt.Fprintf(&b, "  Evaluator/profile: %s / %s\n", classifier.Evaluator, classifier.ClassifierProfile)
			if classifier.ResultsFile != "" {
				fmt.Fprintf(&b, "  Results file: %s\n", classifier.ResultsFile)
			}
			fmt.Fprintf(&b, "  Discovery: %s; ambiguity: %s; generic fallback: %s; reject: %s\n",
				firstIndexPct(classifier.DiscoveryCoverage),
				firstIndexPct(classifier.AmbiguityRate),
				firstIndexPct(classifier.GenericFallbackRate),
				firstIndexPct(classifier.RejectRate))
			if classifier.SubformatFamilyCases > 0 {
				fmt.Fprintf(&b, "  Subformat/family accuracy: %s\n", firstIndexPct(classifier.SubformatFamilyAccuracy))
			}
			if classifier.ChildCandidateExpected > 0 {
				fmt.Fprintf(&b, "  Child candidate coverage: %d/%d = %s\n",
					classifier.ChildCandidateMatched,
					classifier.ChildCandidateExpected,
					firstIndexPct(classifier.ChildCandidateCoverage))
			}
			for _, model := range classifier.Models {
				fmt.Fprintf(&b, "  Model %s: expected %d / predicted %d / precision %s / recall %s\n",
					model.Model,
					model.Expected,
					model.Predicted,
					firstIndexPct(model.Precision),
					firstIndexPct(model.Recall))
			}
			if len(classifier.Confusions) > 0 {
				var parts []string
				for _, confusion := range classifier.Confusions {
					parts = append(parts, fmt.Sprintf("%s -> %s: %d", confusion.Expected, confusion.Actual, confusion.Count))
				}
				fmt.Fprintf(&b, "  Confusions: %s\n", strings.Join(parts, "; "))
			}
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "Residual Risks")
	if len(report.WeakSpots) == 0 && len(report.ThresholdFailures) == 0 {
		fmt.Fprintln(&b, "- None from this report.")
		return b.String()
	}
	for _, weak := range report.WeakSpots {
		caseRef := weak.CaseID
		if weak.Path != "" {
			caseRef += " (" + weak.Path + ")"
		}
		fmt.Fprintf(&b, "- %s %s: %s\n", weak.Lane, caseRef, weak.Message)
	}
	for _, failure := range report.ThresholdFailures {
		fmt.Fprintf(&b, "- Threshold failure: %s\n", failure)
	}
	return b.String()
}

func firstIndexPct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

func meanRetrievalMustHitAt3(retrievals []firstIndexRetrievalReport) float64 {
	if len(retrievals) == 0 {
		return 0
	}
	var sum float64
	for _, retrieval := range retrievals {
		sum += retrieval.AgentMetrics.MustHitAt3
	}
	return sum / float64(len(retrievals))
}

func firstIndexComma(n int) string {
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
