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

const (
	CanonicalLaneIntent        = "intent"
	CanonicalLaneModel         = "model"
	CanonicalLaneProtocol      = "protocol"
	CanonicalLaneTemplate      = "template"
	CanonicalLaneTrace         = "trace"
	CanonicalLaneSourceContext = "source_context"
	CanonicalLaneUnknown       = "unknown"
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
	Path          string  `json:"path"`
	Lane          string  `json:"lane"`
	CanonicalLane string  `json:"canonical_lane,omitempty"`
	Grade         string  `json:"grade"`
	Weight        float64 `json:"weight"`
	Exact         bool    `json:"exact"`
	SameCluster   bool    `json:"same_cluster,omitempty"`
	HardNegative  bool    `json:"hard_negative,omitempty"`
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
	sameCluster := sameClusterContext{
		lineExpectedBases: lineExpectedBases,
		expectedPaths:     expectedPathList(expected),
	}

	var positiveWeight, penalizedWeight float64
	for i, artifact := range cr.ArtifactsIncluded {
		norm := normalizeMetricPath(artifact)
		lane := classifyMetricLane(artifact, fileByPath[norm], reasonByPath[norm])
		canonicalLane := classifyCanonicalLane(artifact, fileByPath[norm], reasonByPath[norm])
		grade := gradeArtifactForAgentMetrics(norm, expected, hardNegatives, sameCluster)
		if grade.weight > 0 {
			positiveWeight += grade.weight
			penalizedWeight += grade.weight
			if cr.AgentMetrics.FirstUsefulRank == 0 {
				cr.AgentMetrics.FirstUsefulRank = i + 1
			}
		} else {
			penalizedWeight += grade.weight
		}
		if evalArtifactPathInSetByIdentity(norm, must) && cr.AgentMetrics.FirstMustRank == 0 {
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
			Path:          artifact,
			Lane:          lane,
			CanonicalLane: canonicalLane,
			Grade:         grade.grade,
			Weight:        grade.weight,
			Exact:         grade.exact,
			SameCluster:   grade.sameCluster,
			HardNegative:  grade.hardNegative,
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

func summarizeCanonicalLaneMetrics(cases []CaseResult) []LaneMetric {
	lanes := []string{
		CanonicalLaneIntent,
		CanonicalLaneModel,
		CanonicalLaneProtocol,
		CanonicalLaneTemplate,
		CanonicalLaneTrace,
		CanonicalLaneSourceContext,
		CanonicalLaneUnknown,
	}
	accs := make(map[string]*laneAccumulator, len(lanes))
	for _, lane := range lanes {
		accs[lane] = &laneAccumulator{LaneMetric: LaneMetric{Lane: lane}}
	}
	for _, c := range cases {
		for _, lane := range lanes {
			accs[lane].Cases++
		}
		expectedByLane := expectedCanonicalLaneCountsFromCase(c)
		for lane, count := range expectedByLane {
			acc := canonicalLaneAccumulator(accs, lane)
			acc.ExpectedArtifacts += count
			if count > 0 {
				acc.CasesWithExpected++
			}
		}
		includedByLane := map[string]bool{}
		for _, grade := range c.ArtifactGrades {
			lane := grade.CanonicalLane
			if lane == "" {
				lane = CanonicalLaneUnknown
			}
			acc := canonicalLaneAccumulator(accs, lane)
			includedByLane[lane] = true
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
		for lane := range includedByLane {
			canonicalLaneAccumulator(accs, lane).CasesWithIncluded++
		}
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

func canonicalLaneAccumulator(accs map[string]*laneAccumulator, lane string) *laneAccumulator {
	if lane == "" {
		lane = CanonicalLaneUnknown
	}
	if accs[lane] == nil {
		accs[lane] = &laneAccumulator{LaneMetric: LaneMetric{Lane: lane}}
	}
	return accs[lane]
}

func expectedCanonicalLaneCountsFromCase(c CaseResult) map[string]int {
	out := map[string]int{}
	for _, path := range c.RelevantIncluded {
		out[classifyExpectedCanonicalLane(path)]++
	}
	for _, path := range c.MissedExpectedRelevant {
		out[classifyExpectedCanonicalLane(path)]++
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
		LaneDocsPlans:              "Markdown artifacts from artifacts_included.",
		LaneTestCase:               "Artifacts with test-case metadata, test-case behavior reasons, or test-like paths.",
		LaneCodeComment:            "Artifacts with code-comment metadata or code-comment reason text.",
		LaneSourceContextOther:     "Non-markdown artifacts left after test_case and code_comment classification.",
		LanePackedSections:         "Overlay lane from packed_section_artifacts; it may overlap docs_plans and does not participate in source-lane totals.",
		"same_cluster":             "Unlabeled line-ref artifact in the same source/test file as an expected line-ref artifact.",
		CanonicalLaneIntent:        "Canonical document lane for plans, specs, ADRs, proposals, requirements, designs, and ordinary markdown written-record docs.",
		CanonicalLaneModel:         "Canonical document lane for API contracts, schemas, configuration, and workflow/model artifacts.",
		CanonicalLaneProtocol:      "Canonical document lane for agent instructions, skills, policies, runbooks, procedures, and standards.",
		CanonicalLaneTemplate:      "Canonical document lane for prompt, document, issue, and pull-request templates.",
		CanonicalLaneTrace:         "Canonical lane for trace/work-record artifacts when explicitly classified as trace.",
		CanonicalLaneSourceContext: "Non-document bucket for source, test, and code-comment context; not a canonical document lane.",
		CanonicalLaneUnknown:       "Fallback only when no classifier, kind/subtype, source, or path signal can assign a lane.",
	}
}

func classifyCanonicalLane(path string, f File, reasonText string) string {
	if lane := canonicalLaneFromSourceContext(path, f, reasonText); lane != "" {
		return lane
	}
	if lane := canonicalLaneFromMetadata(f.Metadata); lane != "" {
		return lane
	}
	if lane := canonicalLaneFromKindSubtype(f.Kind, f.Subtype); lane != "" {
		return lane
	}
	return classifyExpectedCanonicalLane(path)
}

func canonicalLaneFromMetadata(metadata map[string]string) string {
	if metadata == nil {
		return ""
	}
	for _, key := range []string{"classifier_mode", "mode"} {
		if lane := normalizeCanonicalLane(metadata[key]); lane != "" {
			return lane
		}
	}
	if lane := canonicalLaneFromKindSubtype(metadata["classifier_kind"], metadata["classifier_subtype"]); lane != "" {
		return lane
	}
	model := strings.ToLower(strings.TrimSpace(metadata["classifier_model"]))
	switch model {
	case "adr", "openspec", "plan", "prd", "rfc", "agent_note":
		return CanonicalLaneIntent
	case "model":
		return CanonicalLaneModel
	case "protocol":
		return CanonicalLaneProtocol
	case "template":
		return CanonicalLaneTemplate
	case "trace":
		return CanonicalLaneTrace
	default:
		return ""
	}
}

func normalizeCanonicalLane(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case CanonicalLaneIntent:
		return CanonicalLaneIntent
	case CanonicalLaneModel:
		return CanonicalLaneModel
	case CanonicalLaneProtocol:
		return CanonicalLaneProtocol
	case CanonicalLaneTemplate:
		return CanonicalLaneTemplate
	case CanonicalLaneTrace:
		return CanonicalLaneTrace
	default:
		return ""
	}
}

func canonicalLaneFromKindSubtype(kind, subtype string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	subtype = strings.ToLower(strings.TrimSpace(subtype))
	switch {
	case kind == "source_context":
		return CanonicalLaneSourceContext
	case isProtocolCanonicalSubtype(subtype):
		return CanonicalLaneProtocol
	case isTemplateCanonicalSubtype(subtype):
		return CanonicalLaneTemplate
	case isModelCanonicalSubtype(subtype):
		return CanonicalLaneModel
	case kind == "contract":
		return CanonicalLaneModel
	case kind == "plan" || kind == "spec" || kind == "requirements" || kind == "design" || kind == "decision":
		return CanonicalLaneIntent
	case subtype == "adr" || subtype == "prd" || strings.HasPrefix(subtype, "openspec_"):
		return CanonicalLaneIntent
	default:
		return ""
	}
}

func canonicalLaneFromSourceContext(path string, f File, reasonText string) string {
	if strings.EqualFold(strings.TrimSpace(f.Kind), "source_context") {
		return CanonicalLaneSourceContext
	}
	if isTestCaseFile(f) || isCodeCommentFile(f) || isTestLikeMetricPath(path) {
		return CanonicalLaneSourceContext
	}
	if f.Metadata != nil {
		switch strings.ToLower(strings.TrimSpace(f.Metadata["source_type"])) {
		case "test_case", "code_comment", "source_context":
			return CanonicalLaneSourceContext
		}
	}
	reasonText = strings.ToLower(reasonText)
	if strings.Contains(reasonText, "test-case behavior signal") ||
		strings.Contains(reasonText, "code comment") ||
		strings.Contains(reasonText, "code-comment") ||
		strings.Contains(reasonText, "comment signal") {
		return CanonicalLaneSourceContext
	}
	if isSourceLikeCanonicalPath(path) {
		return CanonicalLaneSourceContext
	}
	return ""
}

func classifyExpectedCanonicalLane(path string) string {
	path = strings.ToLower(filepath.ToSlash(metricBasePath(path)))
	switch {
	case path == "":
		return CanonicalLaneUnknown
	case isProtocolCanonicalPath(path):
		return CanonicalLaneProtocol
	case isTemplateCanonicalPath(path):
		return CanonicalLaneTemplate
	case isModelCanonicalPath(path):
		return CanonicalLaneModel
	case isMarkdownMetricPath(path):
		return CanonicalLaneIntent
	case isTestLikeMetricPath(path) || isSourceLikeCanonicalPath(path):
		return CanonicalLaneSourceContext
	default:
		return CanonicalLaneUnknown
	}
}

func isProtocolCanonicalSubtype(subtype string) bool {
	switch subtype {
	case "agent_instruction", "skill", "maintainer_policy", "ownership_policy",
		"governance_policy", "contribution_policy", "security_policy",
		"procedure", "runbook", "standard":
		return true
	default:
		return false
	}
}

func isTemplateCanonicalSubtype(subtype string) bool {
	switch subtype {
	case "document_template", "prompt_template", "issue_template", "pull_request_template":
		return true
	default:
		return false
	}
}

func isModelCanonicalSubtype(subtype string) bool {
	switch subtype {
	case "api_contract", "schema_model", "configuration", "workflow_definition":
		return true
	default:
		return false
	}
}

func isProtocolCanonicalPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	switch base {
	case "agents.md", "agent.md", "claude.md", "codex.md", "gemini.md", "memento.md", "contributing.md", "security.md", "governance.md", "maintainers.md", ".cursorrules":
		return true
	}
	if stem == "skill" || stem == "runbook" || stem == "procedure" || stem == "standard" {
		return true
	}
	return strings.Contains(path, "/agents/") ||
		strings.Contains(path, "/skills/") ||
		strings.Contains(path, "/runbooks/") ||
		strings.Contains(path, "/procedures/") ||
		strings.Contains(path, "/policies/") ||
		strings.Contains(path, "/standards/") ||
		strings.Contains(path, "/.github/instructions/") ||
		strings.HasSuffix(stem, ".agent") ||
		strings.HasSuffix(stem, ".instructions")
}

func isTemplateCanonicalPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	return strings.Contains(path, "/templates/") ||
		strings.Contains(path, "/template/") ||
		strings.Contains(base, "template") ||
		strings.Contains(path, ".github/issue_template") ||
		strings.Contains(path, ".github/pull_request_template")
}

func isModelCanonicalPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.Contains(path, "/schemas/") ||
		strings.Contains(path, "/schema/") ||
		strings.Contains(path, "/openapi/") ||
		strings.Contains(path, "/api/") ||
		strings.Contains(path, "/configs/") ||
		strings.Contains(path, "/config/") ||
		strings.Contains(path, "/workflows/") ||
		strings.Contains(path, "/workflow/") ||
		strings.Contains(path, "/.github/workflows/") {
		return true
	}
	return strings.Contains(stem, "schema") ||
		strings.Contains(stem, "openapi") ||
		strings.Contains(stem, "config") ||
		strings.Contains(stem, "workflow") ||
		strings.Contains(stem, "contract")
}

func isSourceLikeCanonicalPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(metricBasePath(path)))
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php", ".java", ".kt", ".kts", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp", ".cs", ".swift", ".scala", ".sql", ".sh", ".bash", ".zsh", ".ps1":
		return true
	default:
		return false
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

type sameClusterContext struct {
	lineExpectedBases map[string]bool
	expectedPaths     []string
}

func gradeArtifactForAgentMetrics(path string, expected map[string]string, hardNegatives map[string]bool, sameCluster sameClusterContext) artifactGradeResult {
	if importance, ok := expected[path]; ok {
		return artifactGradeResult{grade: importance, weight: gradeWeight(importance), exact: true}
	}
	base := metricBasePath(path)
	if hardNegatives[path] || hardNegatives[base] || evalArtifactPathInSetByIdentity(path, hardNegatives) {
		return artifactGradeResult{grade: "hard_negative", weight: -1.0, hardNegative: true}
	}
	for expectedPath, importance := range expected {
		if evalArtifactIdentityMatch(path, expectedPath) {
			return artifactGradeResult{grade: importance, weight: gradeWeight(importance), exact: true}
		}
	}
	if hasMetricLineRef(path) && sameCluster.lineExpectedBases[base] {
		return artifactGradeResult{grade: "same_cluster", weight: 0.5, sameCluster: true}
	}
	if sameClusterMetricPath(path, sameCluster.expectedPaths) {
		return artifactGradeResult{grade: "same_cluster", weight: 0.5, sameCluster: true}
	}
	return artifactGradeResult{grade: "unlabeled"}
}

func expectedPathList(expected map[string]string) []string {
	out := make([]string, 0, len(expected))
	for path := range expected {
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

func sameClusterMetricPath(path string, expectedPaths []string) bool {
	path = normalizeMetricPath(path)
	if path == "" || agentProtocolMetricPath(path) || !isMarkdownMetricPath(path) {
		return false
	}
	for _, expected := range expectedPaths {
		expected = normalizeMetricPath(expected)
		if expected == "" || agentProtocolMetricPath(expected) || !isMarkdownMetricPath(expected) {
			continue
		}
		if metricBasePath(path) == metricBasePath(expected) {
			return true
		}
		if sameMarkdownStemFamily(path, expected) {
			return true
		}
		if sameOpenSpecMetricFamily(path, expected) {
			return true
		}
		pathFamily := metricIntentFamilyDir(path)
		if pathFamily == "" {
			continue
		}
		if expectedFamily := metricIntentFamilyDir(expected); expectedFamily != "" && expectedFamily == pathFamily {
			return true
		}
	}
	return false
}

func sameMarkdownStemFamily(a, b string) bool {
	aBase := metricBasePath(a)
	bBase := metricBasePath(b)
	if localeNeutralMetricDir(filepath.Dir(aBase)) != localeNeutralMetricDir(filepath.Dir(bBase)) {
		return false
	}
	aTokens := stemMetricTokens(filepath.Base(aBase))
	bTokens := stemMetricTokens(filepath.Base(bBase))
	if len(aTokens) < 2 || len(bTokens) < 2 {
		return false
	}
	common := commonPrefixTokens(aTokens, bTokens)
	if len(common) < 2 {
		return false
	}
	for _, token := range common {
		if sameClusterStemSignalToken(token) {
			return true
		}
	}
	return false
}

func stemMetricTokens(name string) []string {
	name = strings.TrimSuffix(strings.ToLower(name), strings.ToLower(filepath.Ext(name)))
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ' '
	})
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) >= 3 {
			out = append(out, part)
		}
	}
	return out
}

func commonPrefixTokens(a, b []string) []string {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	var out []string
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			break
		}
		out = append(out, a[i])
	}
	return out
}

func sameClusterStemSignalToken(token string) bool {
	switch token {
	case "adr", "architecture", "decision", "design", "plan", "prd", "proposal", "prompt", "requirement", "requirements", "rfc", "roadmap", "spec":
		return true
	default:
		return false
	}
}

func metricIntentFamilyDir(path string) string {
	base := metricBasePath(path)
	dir := localeNeutralMetricDir(filepath.Dir(base))
	if dir == "." || dir == "" {
		return ""
	}
	if !intentFamilyDir(dir) {
		return ""
	}
	return dir
}

func localeNeutralMetricDir(dir string) string {
	parts := strings.Split(filepath.ToSlash(dir), "/")
	out := parts[:0]
	for _, part := range parts {
		if metricLocaleSegment(part) {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "/")
}

func metricLocaleSegment(segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	switch segment {
	case "en", "en-us", "en-gb", "zh", "zh-cn", "zh-tw", "cn", "ja", "jp", "ko", "kr", "fr", "de", "es", "pt", "br", "it":
		return true
	default:
		return false
	}
}

func intentFamilyDir(dir string) bool {
	for _, segment := range strings.Split(dir, "/") {
		segment = strings.ToLower(strings.TrimSpace(segment))
		switch segment {
		case "adr", "adrs", "architecture", "decisions", "design", "design-docs", "exec-plans", "plans", "planning", "product-specs", "product-requirements", "prd", "prds", "proposal", "proposals", "requirements", "rfc", "rfcs", "roadmap", "spec", "specs":
			return true
		}
		if strings.Contains(segment, "architecture") ||
			strings.Contains(segment, "design") ||
			strings.Contains(segment, "decision") ||
			strings.Contains(segment, "plan") ||
			strings.Contains(segment, "product-spec") ||
			strings.Contains(segment, "requirement") ||
			strings.Contains(segment, "rfc") ||
			strings.Contains(segment, "spec") {
			return true
		}
	}
	return false
}

func sameOpenSpecMetricFamily(a, b string) bool {
	aParts := strings.Split(metricBasePath(a), "/")
	bParts := strings.Split(metricBasePath(b), "/")
	aChange := openSpecChangeName(aParts)
	bChange := openSpecChangeName(bParts)
	if aChange != "" && aChange == bChange {
		return true
	}
	aSpec := openSpecSpecName(aParts)
	bSpec := openSpecSpecName(bParts)
	return aSpec != "" && aSpec == bSpec
}

func openSpecChangeName(parts []string) string {
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "openspec" && parts[i+1] == "changes" {
			return parts[i+2]
		}
	}
	return ""
}

func openSpecSpecName(parts []string) string {
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "openspec" && parts[i+1] == "specs" {
			return parts[i+2]
		}
	}
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "specs" && parts[len(parts)-1] == "spec.md" {
			return parts[i+1]
		}
	}
	return ""
}

func agentProtocolMetricPath(path string) bool {
	path = normalizeMetricPath(path)
	base := filepath.Base(metricBasePath(path))
	switch base {
	case "agents.md", "claude.md", "governance.md", "maintainers.md", "skill.md":
		return true
	}
	if strings.HasSuffix(base, ".agent.md") {
		return true
	}
	if strings.Contains(path, "/.claude/") || strings.Contains(path, "/.codex/skills/") || strings.Contains(path, "/.cursor/rules/") || strings.Contains(path, "/.github/issue_template/") {
		return true
	}
	return false
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
	return evalArtifactBasePath(path)
}

func hasMetricLineRef(path string) bool {
	return evalArtifactHasLineRef(path)
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
