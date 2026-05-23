package evalharness

import (
	"path/filepath"
	"strings"
)

const (
	LaneDocsPlans          = "docs_plans"
	LaneTestCase           = "test_case"
	LaneCodeComment        = "code_comment"
	LaneSourceContextOther = "source_context_other"
	LanePackedSections     = "packed_sections"
)

var agentMetricBudgets = []int{1024, 2048, 4096, 8192}

type AgentMetrics struct {
	MustHitAt1                      float64                  `json:"must_hit_at_1"`
	MustHitAt3                      float64                  `json:"must_hit_at_3"`
	MustHitAt5                      float64                  `json:"must_hit_at_5"`
	MustHitAt10                     float64                  `json:"must_hit_at_10"`
	MeanFirstMustRank               float64                  `json:"mean_first_must_rank,omitempty"`
	MeanFirstUsefulRank             float64                  `json:"mean_first_useful_rank,omitempty"`
	ContextSufficiencyAtTokenBudget []TokenBudgetSufficiency `json:"context_sufficiency_at_token_budget"`
	LowPrecisionSufficientCases     int                      `json:"low_precision_sufficient_cases"`
}

type TokenBudgetSufficiency struct {
	BudgetTokens  int     `json:"budget_tokens"`
	EligibleCases int     `json:"eligible_cases"`
	PassedCases   int     `json:"passed_cases"`
	PassRate      float64 `json:"pass_rate"`
}

type LaneMetric struct {
	Lane                   string  `json:"lane"`
	Cases                  int     `json:"cases"`
	CasesWithIncluded      int     `json:"cases_with_included"`
	CasesWithExpected      int     `json:"cases_with_expected"`
	IncludedArtifacts      int     `json:"included_artifacts"`
	ExactRelevantArtifacts int     `json:"exact_relevant_artifacts"`
	SameClusterArtifacts   int     `json:"same_cluster_artifacts"`
	HardNegativeArtifacts  int     `json:"hard_negative_artifacts"`
	ExpectedArtifacts      int     `json:"expected_artifacts"`
	GradedRelevanceWeight  float64 `json:"graded_relevance_weight"`
	StrictPrecision        float64 `json:"strict_precision,omitempty"`
	GradedPrecision        float64 `json:"graded_precision,omitempty"`
	Recall                 float64 `json:"recall,omitempty"`
	PackedSectionCount     int     `json:"packed_section_count,omitempty"`
}

type CaseAgentMetrics struct {
	IncludedArtifacts         int         `json:"included_artifacts"`
	ExactRelevantArtifacts    int         `json:"exact_relevant_artifacts"`
	SameClusterArtifacts      int         `json:"same_cluster_artifacts"`
	HardNegativeArtifacts     int         `json:"hard_negative_artifacts"`
	StrictPrecision           float64     `json:"strict_precision"`
	GradedPrecision           float64     `json:"graded_precision"`
	PenalizedUtilityPrecision float64     `json:"penalized_utility_precision"`
	FirstMustRank             int         `json:"first_must_rank,omitempty"`
	FirstUsefulRank           int         `json:"first_useful_rank,omitempty"`
	MustHitAt1                bool        `json:"must_hit_at_1"`
	MustHitAt3                bool        `json:"must_hit_at_3"`
	MustHitAt5                bool        `json:"must_hit_at_5"`
	MustHitAt10               bool        `json:"must_hit_at_10"`
	GradeCounts               GradeCounts `json:"grade_counts"`
	LaneCounts                LaneCounts  `json:"lane_counts"`
}

type GradeCounts struct {
	Must         int `json:"must"`
	Helpful      int `json:"helpful"`
	Background   int `json:"background"`
	SameCluster  int `json:"same_cluster"`
	Unlabeled    int `json:"unlabeled"`
	HardNegative int `json:"hard_negative"`
}

type LaneCounts struct {
	DocsPlans          int `json:"docs_plans"`
	TestCase           int `json:"test_case"`
	CodeComment        int `json:"code_comment"`
	SourceContextOther int `json:"source_context_other"`
	PackedSections     int `json:"packed_sections"`
}

type ArtifactGrade struct {
	Path         string  `json:"path"`
	Lane         string  `json:"lane"`
	Grade        string  `json:"grade"`
	Weight       float64 `json:"weight"`
	Exact        bool    `json:"exact"`
	SameCluster  bool    `json:"same_cluster,omitempty"`
	HardNegative bool    `json:"hard_negative,omitempty"`
}

type laneAccumulator struct {
	LaneMetric
}

type artifactGradeResult struct {
	grade        string
	weight       float64
	exact        bool
	sameCluster  bool
	hardNegative bool
}

func applyAgentCaseMetrics(cr *CaseResult, spec CaseSpec, files []File) {
	expected := normalizedExpectedImportanceSet(spec.ExpectedRelevant)
	must := map[string]bool{}
	lineExpectedBases := map[string]bool{}
	for _, artifact := range spec.ExpectedRelevant {
		path := normalizeMetricPath(artifact.Path)
		if artifact.Importance == "must" {
			must[path] = true
		}
		if hasMetricLineRef(path) {
			lineExpectedBases[metricBasePath(path)] = true
		}
	}
	for _, artifact := range spec.SuccessCriteria.MustContainArtifacts {
		path := normalizeMetricPath(artifact)
		if path == "" {
			continue
		}
		must[path] = true
		if _, ok := expected[path]; !ok {
			expected[path] = "must"
		}
		if hasMetricLineRef(path) {
			lineExpectedBases[metricBasePath(path)] = true
		}
	}

	hardNegatives := map[string]bool{}
	for _, path := range spec.ExpectedExcluded {
		hardNegatives[normalizeMetricPath(path)] = true
	}
	for _, path := range spec.SuccessCriteria.MustNotContainArtifacts {
		hardNegatives[normalizeMetricPath(path)] = true
	}
	for _, path := range cr.ContextSufficiency.ForbiddenArtifactsPresent {
		hardNegatives[normalizeMetricPath(path)] = true
	}

	fileByPath := map[string]File{}
	for _, f := range files {
		fileByPath[normalizeMetricPath(f.Path)] = f
	}
	reasonByPath := map[string]string{}
	for _, reason := range cr.ArtifactReasons {
		var parts []string
		for _, text := range reason.Reasons {
			parts = append(parts, strings.ToLower(text))
		}
		reasonByPath[normalizeMetricPath(reason.Path)] = strings.Join(parts, " ")
	}
	packed := normalizedStringSet(cr.PackedSectionArtifacts)

	var positiveWeight, penalizedWeight float64
	for i, artifact := range cr.ArtifactsIncluded {
		norm := normalizeMetricPath(artifact)
		lane := classifyMetricLane(artifact, fileByPath[norm], reasonByPath[norm])
		grade := gradeArtifactForAgentMetrics(norm, expected, hardNegatives, lineExpectedBases)
		if grade.weight > 0 {
			positiveWeight += grade.weight
			penalizedWeight += grade.weight
			if cr.AgentMetrics.FirstUsefulRank == 0 {
				cr.AgentMetrics.FirstUsefulRank = i + 1
			}
		} else {
			penalizedWeight += grade.weight
		}
		if must[norm] && cr.AgentMetrics.FirstMustRank == 0 {
			cr.AgentMetrics.FirstMustRank = i + 1
		}
		if grade.exact {
			cr.AgentMetrics.ExactRelevantArtifacts++
		}
		if grade.sameCluster {
			cr.AgentMetrics.SameClusterArtifacts++
		}
		if grade.hardNegative {
			cr.AgentMetrics.HardNegativeArtifacts++
		}
		addGradeCount(&cr.AgentMetrics.GradeCounts, grade.grade)
		addLaneCount(&cr.AgentMetrics.LaneCounts, lane)
		if packed[norm] {
			cr.AgentMetrics.LaneCounts.PackedSections++
		}
		cr.ArtifactGrades = append(cr.ArtifactGrades, ArtifactGrade{
			Path:         artifact,
			Lane:         lane,
			Grade:        grade.grade,
			Weight:       grade.weight,
			Exact:        grade.exact,
			SameCluster:  grade.sameCluster,
			HardNegative: grade.hardNegative,
		})
	}
	cr.AgentMetrics.IncludedArtifacts = len(cr.ArtifactsIncluded)
	cr.AgentMetrics.StrictPrecision = cr.ArtifactPrecision
	if len(cr.ArtifactsIncluded) > 0 {
		denominator := float64(len(cr.ArtifactsIncluded))
		cr.AgentMetrics.GradedPrecision = positiveWeight / denominator
		if penalizedWeight < 0 {
			penalizedWeight = 0
		}
		cr.AgentMetrics.PenalizedUtilityPrecision = penalizedWeight / denominator
	}
	cr.AgentMetrics.MustHitAt1 = cr.AgentMetrics.FirstMustRank > 0 && cr.AgentMetrics.FirstMustRank <= 1
	cr.AgentMetrics.MustHitAt3 = cr.AgentMetrics.FirstMustRank > 0 && cr.AgentMetrics.FirstMustRank <= 3
	cr.AgentMetrics.MustHitAt5 = cr.AgentMetrics.FirstMustRank > 0 && cr.AgentMetrics.FirstMustRank <= 5
	cr.AgentMetrics.MustHitAt10 = cr.AgentMetrics.FirstMustRank > 0 && cr.AgentMetrics.FirstMustRank <= 10
}

func summarizeAgentMetrics(cases []CaseResult) AgentMetrics {
	var out AgentMetrics
	if len(cases) == 0 {
		return out
	}
	var firstMustRanks, firstUsefulRanks []int
	budgetCounters := make(map[int]*TokenBudgetSufficiency, len(agentMetricBudgets))
	for _, budget := range agentMetricBudgets {
		budgetCounters[budget] = &TokenBudgetSufficiency{BudgetTokens: budget}
	}
	for _, c := range cases {
		if c.AgentMetrics.MustHitAt1 {
			out.MustHitAt1++
		}
		if c.AgentMetrics.MustHitAt3 {
			out.MustHitAt3++
		}
		if c.AgentMetrics.MustHitAt5 {
			out.MustHitAt5++
		}
		if c.AgentMetrics.MustHitAt10 {
			out.MustHitAt10++
		}
		if c.AgentMetrics.FirstMustRank > 0 {
			firstMustRanks = append(firstMustRanks, c.AgentMetrics.FirstMustRank)
		}
		if c.AgentMetrics.FirstUsefulRank > 0 {
			firstUsefulRanks = append(firstUsefulRanks, c.AgentMetrics.FirstUsefulRank)
		}
		if c.ContextSufficiency.Configured {
			for _, budget := range agentMetricBudgets {
				counter := budgetCounters[budget]
				counter.EligibleCases++
				if c.ContextSufficiency.Passed && c.DevSpecsTokens <= budget {
					counter.PassedCases++
				}
			}
		}
		if c.ContextSufficiency.Passed && c.ArtifactPrecision < 0.5 {
			out.LowPrecisionSufficientCases++
		}
	}
	n := float64(len(cases))
	out.MustHitAt1 /= n
	out.MustHitAt3 /= n
	out.MustHitAt5 /= n
	out.MustHitAt10 /= n
	out.MeanFirstMustRank = meanInts(firstMustRanks)
	out.MeanFirstUsefulRank = meanInts(firstUsefulRanks)
	for _, budget := range agentMetricBudgets {
		counter := budgetCounters[budget]
		if counter.EligibleCases > 0 {
			counter.PassRate = float64(counter.PassedCases) / float64(counter.EligibleCases)
		}
		out.ContextSufficiencyAtTokenBudget = append(out.ContextSufficiencyAtTokenBudget, *counter)
	}
	return out
}

func summarizeLaneMetrics(cases []CaseResult) []LaneMetric {
	lanes := []string{LaneDocsPlans, LaneTestCase, LaneCodeComment, LaneSourceContextOther, LanePackedSections}
	accs := make(map[string]*laneAccumulator, len(lanes))
	for _, lane := range lanes {
		accs[lane] = &laneAccumulator{LaneMetric: LaneMetric{Lane: lane}}
	}
	for _, c := range cases {
		for _, lane := range lanes {
			accs[lane].Cases++
		}
		expectedByLane := expectedLaneCountsFromCase(c)
		for lane, count := range expectedByLane {
			accs[lane].ExpectedArtifacts += count
			if count > 0 {
				accs[lane].CasesWithExpected++
			}
		}
		for _, grade := range c.ArtifactGrades {
			acc := accs[grade.Lane]
			acc.IncludedArtifacts++
			if grade.Exact {
				acc.ExactRelevantArtifacts++
			}
			if grade.SameCluster {
				acc.SameClusterArtifacts++
			}
			if grade.HardNegative {
				acc.HardNegativeArtifacts++
			}
			if grade.Weight > 0 {
				acc.GradedRelevanceWeight += grade.Weight
			}
		}
		addIncludedCaseCounts(accs, c.AgentMetrics.LaneCounts)
		for _, artifact := range c.PackedSectionArtifacts {
			norm := normalizeMetricPath(artifact)
			for _, grade := range c.ArtifactGrades {
				if normalizeMetricPath(grade.Path) != norm {
					continue
				}
				acc := accs[LanePackedSections]
				acc.IncludedArtifacts++
				if grade.Exact {
					acc.ExactRelevantArtifacts++
				}
				if grade.SameCluster {
					acc.SameClusterArtifacts++
				}
				if grade.HardNegative {
					acc.HardNegativeArtifacts++
				}
				if grade.Weight > 0 {
					acc.GradedRelevanceWeight += grade.Weight
				}
				break
			}
		}
		accs[LanePackedSections].PackedSectionCount += c.PackedSectionCount
	}
	out := make([]LaneMetric, 0, len(lanes))
	for _, lane := range lanes {
		metric := accs[lane].LaneMetric
		if metric.IncludedArtifacts > 0 {
			metric.StrictPrecision = float64(metric.ExactRelevantArtifacts) / float64(metric.IncludedArtifacts)
			metric.GradedPrecision = metric.GradedRelevanceWeight / float64(metric.IncludedArtifacts)
		}
		if metric.ExpectedArtifacts > 0 {
			metric.Recall = float64(metric.ExactRelevantArtifacts) / float64(metric.ExpectedArtifacts)
		}
		out = append(out, metric)
	}
	return out
}

func expectedLaneCountsFromCase(c CaseResult) map[string]int {
	out := map[string]int{}
	for _, path := range c.RelevantIncluded {
		out[classifyExpectedMetricLane(path)]++
	}
	for _, path := range c.MissedExpectedRelevant {
		out[classifyExpectedMetricLane(path)]++
	}
	return out
}

func addIncludedCaseCounts(accs map[string]*laneAccumulator, counts LaneCounts) {
	if counts.DocsPlans > 0 {
		accs[LaneDocsPlans].CasesWithIncluded++
	}
	if counts.TestCase > 0 {
		accs[LaneTestCase].CasesWithIncluded++
	}
	if counts.CodeComment > 0 {
		accs[LaneCodeComment].CasesWithIncluded++
	}
	if counts.SourceContextOther > 0 {
		accs[LaneSourceContextOther].CasesWithIncluded++
	}
	if counts.PackedSections > 0 {
		accs[LanePackedSections].CasesWithIncluded++
	}
}

func classifyMetricLane(path string, f File, reasonText string) string {
	switch {
	case isTestCaseFile(f):
		return LaneTestCase
	case isCodeCommentFile(f):
		return LaneCodeComment
	case strings.Contains(reasonText, "test-case behavior signal"):
		return LaneTestCase
	case strings.Contains(reasonText, "code comment") || strings.Contains(reasonText, "code-comment") || strings.Contains(reasonText, "comment signal"):
		return LaneCodeComment
	case isMarkdownMetricPath(path):
		return LaneDocsPlans
	case isTestLikeMetricPath(path):
		return LaneTestCase
	default:
		return LaneSourceContextOther
	}
}

func agentMetricNotes() map[string]string {
	return map[string]string{
		LaneDocsPlans:          "Markdown artifacts from artifacts_included.",
		LaneTestCase:           "Artifacts with test-case metadata, test-case behavior reasons, or test-like paths.",
		LaneCodeComment:        "Artifacts with code-comment metadata or code-comment reason text.",
		LaneSourceContextOther: "Non-markdown artifacts left after test_case and code_comment classification.",
		LanePackedSections:     "Overlay lane from packed_section_artifacts; it may overlap docs_plans and does not participate in source-lane totals.",
		"same_cluster":         "Unlabeled line-ref artifact in the same source/test file as an expected line-ref artifact.",
	}
}

func normalizedExpectedImportanceSet(items []ExpectedArtifact) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		importance, err := normalizeImportance(item.Importance)
		if err != nil {
			importance = "must"
		}
		out[normalizeMetricPath(item.Path)] = importance
	}
	return out
}

func normalizedStringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		item = normalizeMetricPath(item)
		if item != "" {
			out[item] = true
		}
	}
	return out
}

func classifyExpectedMetricLane(path string) string {
	switch {
	case isMarkdownMetricPath(path):
		return LaneDocsPlans
	case isTestLikeMetricPath(path):
		return LaneTestCase
	default:
		return LaneSourceContextOther
	}
}

func gradeArtifactForAgentMetrics(path string, expected map[string]string, hardNegatives map[string]bool, lineExpectedBases map[string]bool) artifactGradeResult {
	if importance, ok := expected[path]; ok {
		return artifactGradeResult{grade: importance, weight: gradeWeight(importance), exact: true}
	}
	base := metricBasePath(path)
	if hardNegatives[path] || hardNegatives[base] {
		return artifactGradeResult{grade: "hard_negative", weight: -1.0, hardNegative: true}
	}
	if hasMetricLineRef(path) && lineExpectedBases[base] {
		return artifactGradeResult{grade: "same_cluster", weight: 0.5, sameCluster: true}
	}
	return artifactGradeResult{grade: "unlabeled"}
}

func gradeWeight(importance string) float64 {
	switch strings.ToLower(strings.TrimSpace(importance)) {
	case "must":
		return 1.0
	case "helpful":
		return 0.6
	case "background":
		return 0.3
	case "same_cluster":
		return 0.5
	default:
		return 1.0
	}
}

func addGradeCount(counts *GradeCounts, grade string) {
	switch grade {
	case "must":
		counts.Must++
	case "helpful":
		counts.Helpful++
	case "background":
		counts.Background++
	case "same_cluster":
		counts.SameCluster++
	case "hard_negative":
		counts.HardNegative++
	default:
		counts.Unlabeled++
	}
}

func addLaneCount(counts *LaneCounts, lane string) {
	switch lane {
	case LaneDocsPlans:
		counts.DocsPlans++
	case LaneTestCase:
		counts.TestCase++
	case LaneCodeComment:
		counts.CodeComment++
	default:
		counts.SourceContextOther++
	}
}

func metricBasePath(path string) string {
	path = normalizeMetricPath(path)
	if idx := strings.LastIndex(path, "#l"); idx >= 0 {
		if allDigits(path[idx+2:]) {
			return path[:idx]
		}
	}
	return path
}

func hasMetricLineRef(path string) bool {
	path = normalizeMetricPath(path)
	idx := strings.LastIndex(path, "#l")
	return idx >= 0 && allDigits(path[idx+2:])
}

func normalizeMetricPath(path string) string {
	return strings.ToLower(strings.TrimSpace(filepath.ToSlash(path)))
}

func isMarkdownMetricPath(path string) bool {
	base := metricBasePath(path)
	ext := strings.ToLower(filepath.Ext(base))
	return ext == ".md" || ext == ".mdx" || ext == ".markdown"
}

func isTestLikeMetricPath(path string) bool {
	path = normalizeMetricPath(path)
	base := metricBasePath(path)
	segments := strings.Split(base, "/")
	for _, segment := range segments {
		if segment == "test" || segment == "tests" || segment == "spec" || segment == "specs" || segment == "__tests__" {
			return true
		}
	}
	name := filepath.Base(base)
	return strings.Contains(name, "_test.") ||
		strings.Contains(name, ".test.") ||
		strings.Contains(name, "-test.") ||
		strings.Contains(name, "_spec.") ||
		strings.Contains(name, ".spec.") ||
		strings.Contains(name, "-spec.") ||
		strings.HasPrefix(name, "test_") ||
		strings.HasPrefix(name, "spec_")
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func meanInts(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum int
	for _, value := range values {
		sum += value
	}
	return float64(sum) / float64(len(values))
}
