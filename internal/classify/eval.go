package classify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const EvaluatorDeclarativeDocumentModelsV0 = "declarative_document_models_v0"

type EvalOptions struct {
	Config PipelineConfig
}

type EvalResult struct {
	Fixture           string             `json:"fixture"`
	FixtureVersion    string             `json:"fixture_version,omitempty"`
	EvalStage         string             `json:"eval_stage,omitempty"`
	Evaluator         string             `json:"evaluator"`
	ClassifierProfile string             `json:"classifier_profile"`
	ConfigVersion     int                `json:"config_version"`
	ResultsFile       string             `json:"results_file,omitempty"`
	Summary           EvalSummary        `json:"summary"`
	Models            []EvalModelSummary `json:"models"`
	Confusions        []EvalConfusion    `json:"confusions,omitempty"`
	Cases             []EvalCaseResult   `json:"cases"`
}

type EvalSummary struct {
	Cases                    int     `json:"cases"`
	PassedCases              int     `json:"passed_cases"`
	Accuracy                 float64 `json:"accuracy"`
	SubformatFamilyCases     int     `json:"subformat_family_cases"`
	SubformatFamilyPassed    int     `json:"subformat_family_passed"`
	SubformatFamilyAccuracy  float64 `json:"subformat_family_accuracy"`
	DiscoveryCoverage        float64 `json:"discovery_coverage"`
	FixturePathCoverage      float64 `json:"fixture_path_coverage"`
	MissingFixturePaths      int     `json:"missing_fixture_paths"`
	AmbiguousCases           int     `json:"ambiguous_cases"`
	AmbiguityRate            float64 `json:"ambiguity_rate"`
	GenericFallbackCases     int     `json:"generic_fallback_cases"`
	GenericFallbackRate      float64 `json:"generic_fallback_rate"`
	RejectedCases            int     `json:"rejected_cases"`
	RejectRate               float64 `json:"reject_rate"`
	ReasonCoverageCases      int     `json:"reason_coverage_cases"`
	ReasonCoveragePassed     int     `json:"reason_coverage_passed"`
	ReasonCoverageRate       float64 `json:"reason_coverage_rate"`
	CasesWithNegativeReasons int     `json:"cases_with_negative_reasons"`
	ChildCandidateExpected   int     `json:"child_candidate_expected"`
	ChildCandidateMatched    int     `json:"child_candidate_matched"`
	ChildCandidateCoverage   float64 `json:"child_candidate_coverage"`
}

type EvalModelSummary struct {
	Model     string  `json:"model"`
	Expected  int     `json:"expected"`
	Predicted int     `json:"predicted"`
	TruePos   int     `json:"true_positive"`
	FalsePos  int     `json:"false_positive"`
	FalseNeg  int     `json:"false_negative"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}

type EvalConfusion struct {
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Count    int    `json:"count"`
}

type EvalCaseResult struct {
	ID                      string           `json:"id"`
	Path                    string           `json:"path"`
	Scope                   Scope            `json:"scope"`
	PathExists              bool             `json:"path_exists"`
	Passed                  bool             `json:"passed"`
	ExpectedClassifier      string           `json:"expected_classifier"`
	ActualClassifier        string           `json:"actual_classifier"`
	ExpectedSubformat       string           `json:"expected_subformat,omitempty"`
	ActualSubformat         string           `json:"actual_subformat,omitempty"`
	ExpectedFamily          string           `json:"expected_family,omitempty"`
	ActualFamily            string           `json:"actual_family,omitempty"`
	ExpectedKind            string           `json:"expected_kind,omitempty"`
	ActualKind              string           `json:"actual_kind,omitempty"`
	ExpectedSubtype         string           `json:"expected_subtype,omitempty"`
	ActualSubtype           string           `json:"actual_subtype,omitempty"`
	ExpectedStatus          string           `json:"expected_status,omitempty"`
	ActualStatus            string           `json:"actual_status,omitempty"`
	ExpectedAuthority       string           `json:"expected_authority,omitempty"`
	ActualAuthority         string           `json:"actual_authority,omitempty"`
	ExpectedFormatProfile   string           `json:"expected_format_profile,omitempty"`
	ActualFormatProfile     string           `json:"actual_format_profile,omitempty"`
	Confidence              float64          `json:"confidence"`
	Accepted                bool             `json:"accepted"`
	Ambiguous               bool             `json:"ambiguous"`
	FallbackGeneric         bool             `json:"fallback_generic"`
	MissingRequiredReasons  []ReasonCode     `json:"missing_required_reasons,omitempty"`
	ForbiddenClassifierHits []string         `json:"forbidden_classifier_hits,omitempty"`
	ExpectedChildCandidates []string         `json:"expected_child_candidates,omitempty"`
	ActualChildCandidates   []string         `json:"actual_child_candidates,omitempty"`
	MissingChildCandidates  []string         `json:"missing_child_candidates,omitempty"`
	PositiveReasons         []Reason         `json:"positive_reasons,omitempty"`
	NegativeReasons         []Reason         `json:"negative_reasons,omitempty"`
	Alternatives            []Classification `json:"alternatives,omitempty"`
}

func RunEval(fixture string, opts EvalOptions) (*EvalResult, error) {
	cfg := opts.Config
	if cfg.Version == 0 {
		cfg = DefaultPipelineConfig()
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	fixtureAbs, err := filepath.Abs(fixture)
	if err != nil {
		return nil, err
	}
	goldenPath := filepath.Join(fixtureAbs, "classifier_cases.yaml")
	golden, err := LoadGoldenFile(goldenPath)
	if err != nil {
		return nil, err
	}
	labels := loadFixtureEvalLabels(fixtureAbs)
	result := &EvalResult{
		Fixture:           filepath.ToSlash(fixture),
		FixtureVersion:    labels.FixtureVersion,
		EvalStage:         labels.EvalStage,
		Evaluator:         EvaluatorDeclarativeDocumentModelsV0,
		ClassifierProfile: cfg.Profile,
		ConfigVersion:     cfg.Version,
	}
	if result.FixtureVersion == "" {
		result.FixtureVersion = golden.Fixture
	}
	if result.EvalStage == "" {
		result.EvalStage = "seed_smoke"
	}

	for _, tc := range golden.ClassifierCases {
		caseResult, err := runEvalCase(fixtureAbs, tc, cfg)
		if err != nil {
			return nil, err
		}
		result.Cases = append(result.Cases, caseResult)
	}
	result.Summary = summarizeEvalCases(result.Cases)
	result.Models = summarizeEvalModels(result.Cases)
	result.Confusions = summarizeEvalConfusions(result.Cases)
	return result, nil
}

func runEvalCase(fixtureAbs string, tc GoldenCase, cfg PipelineConfig) (EvalCaseResult, error) {
	candidate, pathExists, err := evalCandidateFromGolden(fixtureAbs, tc)
	if err != nil {
		return EvalCaseResult{}, err
	}
	resolution := ClassifyCandidate(candidate, cfg)
	winner := resolution.Winner
	out := EvalCaseResult{
		ID:                    tc.ID,
		Path:                  filepath.ToSlash(tc.Path),
		Scope:                 tc.Scope,
		PathExists:            pathExists,
		ExpectedClassifier:    tc.Expected.Classifier,
		ActualClassifier:      winner.Classifier,
		ExpectedSubformat:     tc.Expected.Subformat,
		ActualSubformat:       winner.Subformat,
		ExpectedFamily:        tc.Expected.Family,
		ActualFamily:          winner.Family,
		ExpectedKind:          tc.Expected.Kind,
		ActualKind:            winner.Kind,
		ExpectedSubtype:       tc.Expected.Subtype,
		ActualSubtype:         winner.Subtype,
		ExpectedStatus:        tc.Expected.Status,
		ActualStatus:          winner.Status,
		ExpectedAuthority:     tc.Expected.Authority,
		ActualAuthority:       winner.Authority,
		ExpectedFormatProfile: tc.Expected.FormatProfile,
		ActualFormatProfile:   winner.FormatProfile,
		Confidence:            winner.Confidence,
		Accepted:              winner.Accepted,
		Ambiguous:             resolution.Ambiguous,
		FallbackGeneric:       resolution.FallbackGeneric,
		PositiveReasons:       winner.PositiveReasons,
		NegativeReasons:       winner.NegativeReasons,
		Alternatives:          resolution.Alternatives,
	}
	out.ExpectedChildCandidates = goldenChildPaths(tc.Expected.ChildCandidates)
	out.ActualChildCandidates = candidatePaths(winner.ChildCandidates)
	out.MissingChildCandidates = missingStrings(out.ExpectedChildCandidates, out.ActualChildCandidates)
	for _, reason := range tc.Expected.RequiredReasons {
		if !classificationHasReason(winner, reason) {
			out.MissingRequiredReasons = append(out.MissingRequiredReasons, reason)
		}
	}
	for _, forbidden := range tc.Expected.MustNotClassifyAs {
		if winner.Classifier == forbidden {
			out.ForbiddenClassifierHits = append(out.ForbiddenClassifierHits, forbidden)
		}
	}
	out.Passed = out.PathExists &&
		winner.Classifier == tc.Expected.Classifier &&
		matchesIfExpected(tc.Expected.Scope, winner.Scope) &&
		matchesIfExpectedString(tc.Expected.Subformat, winner.Subformat) &&
		matchesIfExpectedString(tc.Expected.Family, winner.Family) &&
		matchesIfExpectedString(tc.Expected.Kind, winner.Kind) &&
		matchesIfExpectedString(tc.Expected.Subtype, winner.Subtype) &&
		matchesIfExpectedString(tc.Expected.Status, winner.Status) &&
		matchesIfExpectedString(tc.Expected.Authority, winner.Authority) &&
		matchesIfExpectedString(tc.Expected.FormatProfile, winner.FormatProfile) &&
		len(out.MissingRequiredReasons) == 0 &&
		len(out.ForbiddenClassifierHits) == 0 &&
		len(out.MissingChildCandidates) == 0
	return out, nil
}

func evalCandidateFromGolden(fixtureAbs string, tc GoldenCase) (Candidate, bool, error) {
	path := filepath.ToSlash(tc.Path)
	fullPath := filepath.Join(fixtureAbs, filepath.FromSlash(path))
	candidate := Candidate{
		Path:  path,
		Scope: tc.Scope,
	}
	pathExists := false
	info, err := os.Stat(fullPath)
	if err == nil {
		pathExists = true
		if info.IsDir() {
			candidate.SizeBytes = 0
		} else {
			candidate.SizeBytes = info.Size()
			body, err := os.ReadFile(fullPath)
			if err != nil {
				return Candidate{}, false, fmt.Errorf("read classifier eval case %s: %w", tc.ID, err)
			}
			candidate.Body = string(body)
		}
	} else if !os.IsNotExist(err) {
		return Candidate{}, false, err
	}
	for _, child := range tc.Expected.ChildCandidates {
		childPath := filepath.ToSlash(child.Path)
		childCandidate := Candidate{
			Path:  childPath,
			Scope: ScopeDocument,
			Role:  child.Role,
		}
		if info, err := os.Stat(filepath.Join(fixtureAbs, filepath.FromSlash(childPath))); err == nil {
			childCandidate.SizeBytes = info.Size()
		}
		candidate.ChildCandidates = append(candidate.ChildCandidates, childCandidate)
	}
	return candidate, pathExists, nil
}

func summarizeEvalCases(cases []EvalCaseResult) EvalSummary {
	var out EvalSummary
	out.Cases = len(cases)
	if len(cases) == 0 {
		return out
	}
	for _, c := range cases {
		if c.Passed {
			out.PassedCases++
		}
		if !c.PathExists {
			out.MissingFixturePaths++
		}
		if c.ExpectedSubformat != "" || c.ExpectedFamily != "" {
			out.SubformatFamilyCases++
			if c.ExpectedSubformat == c.ActualSubformat && c.ExpectedFamily == c.ActualFamily {
				out.SubformatFamilyPassed++
			}
		}
		if c.Ambiguous {
			out.AmbiguousCases++
		}
		if c.FallbackGeneric || c.ActualClassifier == ModelGenericMarkdown {
			out.GenericFallbackCases++
		}
		if !c.Accepted {
			out.RejectedCases++
		}
		if len(c.MissingRequiredReasons) == 0 {
			out.ReasonCoveragePassed++
		}
		if len(c.PositiveReasons) > 0 || len(c.NegativeReasons) > 0 {
			out.ReasonCoverageCases++
		}
		if len(c.NegativeReasons) > 0 {
			out.CasesWithNegativeReasons++
		}
		out.ChildCandidateExpected += len(c.ExpectedChildCandidates)
		out.ChildCandidateMatched += len(c.ExpectedChildCandidates) - len(c.MissingChildCandidates)
	}
	total := float64(len(cases))
	out.Accuracy = float64(out.PassedCases) / total
	out.FixturePathCoverage = float64(len(cases)-out.MissingFixturePaths) / total
	out.DiscoveryCoverage = out.FixturePathCoverage
	out.AmbiguityRate = float64(out.AmbiguousCases) / total
	out.GenericFallbackRate = float64(out.GenericFallbackCases) / total
	out.RejectRate = float64(out.RejectedCases) / total
	if out.SubformatFamilyCases > 0 {
		out.SubformatFamilyAccuracy = float64(out.SubformatFamilyPassed) / float64(out.SubformatFamilyCases)
	}
	if out.ReasonCoverageCases > 0 {
		out.ReasonCoverageRate = float64(out.ReasonCoveragePassed) / float64(out.ReasonCoverageCases)
	}
	if out.ChildCandidateExpected > 0 {
		out.ChildCandidateCoverage = float64(out.ChildCandidateMatched) / float64(out.ChildCandidateExpected)
	}
	return out
}

func summarizeEvalModels(cases []EvalCaseResult) []EvalModelSummary {
	models := map[string]*EvalModelSummary{}
	ensure := func(model string) *EvalModelSummary {
		if models[model] == nil {
			models[model] = &EvalModelSummary{Model: model}
		}
		return models[model]
	}
	for _, c := range cases {
		expected := ensure(c.ExpectedClassifier)
		actual := ensure(c.ActualClassifier)
		expected.Expected++
		actual.Predicted++
		if c.ExpectedClassifier == c.ActualClassifier {
			expected.TruePos++
		} else {
			expected.FalseNeg++
			actual.FalsePos++
		}
	}
	out := make([]EvalModelSummary, 0, len(models))
	for _, model := range models {
		if model.Predicted > 0 {
			model.Precision = float64(model.TruePos) / float64(model.Predicted)
		}
		if model.Expected > 0 {
			model.Recall = float64(model.TruePos) / float64(model.Expected)
		}
		out = append(out, *model)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Model < out[j].Model
	})
	return out
}

func summarizeEvalConfusions(cases []EvalCaseResult) []EvalConfusion {
	counts := map[string]int{}
	for _, c := range cases {
		if c.ExpectedClassifier == c.ActualClassifier {
			continue
		}
		key := c.ExpectedClassifier + "\x00" + c.ActualClassifier
		counts[key]++
	}
	out := make([]EvalConfusion, 0, len(counts))
	for key, count := range counts {
		parts := strings.SplitN(key, "\x00", 2)
		out = append(out, EvalConfusion{Expected: parts[0], Actual: parts[1], Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].Expected == out[j].Expected {
				return out[i].Actual < out[j].Actual
			}
			return out[i].Expected < out[j].Expected
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func FormatEvalText(r *EvalResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DevSpecs Classifier Eval: %s\n\n", r.Fixture)
	fmt.Fprintf(&b, "Cases: %d\n", r.Summary.Cases)
	if r.FixtureVersion != "" {
		fmt.Fprintf(&b, "Fixture version: %s\n", r.FixtureVersion)
	}
	if r.EvalStage != "" {
		fmt.Fprintf(&b, "Eval stage: %s\n", r.EvalStage)
	}
	fmt.Fprintf(&b, "Evaluator: %s\n", r.Evaluator)
	fmt.Fprintf(&b, "Classifier profile: %s\n", r.ClassifierProfile)
	fmt.Fprintf(&b, "Config version: %d\n", r.ConfigVersion)
	if r.ResultsFile != "" {
		fmt.Fprintf(&b, "Results file: %s\n", r.ResultsFile)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Summary")
	fmt.Fprintf(&b, "- Model accuracy: %d/%d = %s\n", r.Summary.PassedCases, r.Summary.Cases, pct(r.Summary.Accuracy))
	fmt.Fprintf(&b, "- Subformat/family accuracy: %d/%d = %s\n", r.Summary.SubformatFamilyPassed, r.Summary.SubformatFamilyCases, pct(r.Summary.SubformatFamilyAccuracy))
	fmt.Fprintf(&b, "- Discovery coverage (golden fixture paths): %s\n", pct(r.Summary.DiscoveryCoverage))
	fmt.Fprintf(&b, "- Ambiguity rate: %d/%d = %s\n", r.Summary.AmbiguousCases, r.Summary.Cases, pct(r.Summary.AmbiguityRate))
	fmt.Fprintf(&b, "- Generic fallback rate: %d/%d = %s\n", r.Summary.GenericFallbackCases, r.Summary.Cases, pct(r.Summary.GenericFallbackRate))
	fmt.Fprintf(&b, "- Reject rate: %d/%d = %s\n", r.Summary.RejectedCases, r.Summary.Cases, pct(r.Summary.RejectRate))
	fmt.Fprintf(&b, "- Reason coverage: %d/%d = %s\n", r.Summary.ReasonCoveragePassed, r.Summary.ReasonCoverageCases, pct(r.Summary.ReasonCoverageRate))
	if r.Summary.ChildCandidateExpected > 0 {
		fmt.Fprintf(&b, "- Child candidate coverage: %d/%d = %s\n", r.Summary.ChildCandidateMatched, r.Summary.ChildCandidateExpected, pct(r.Summary.ChildCandidateCoverage))
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Models")
	for _, model := range r.Models {
		fmt.Fprintf(&b, "- %s: expected %d / predicted %d / precision %s / recall %s\n",
			model.Model, model.Expected, model.Predicted, pct(model.Precision), pct(model.Recall))
	}
	fmt.Fprintln(&b)
	if len(r.Confusions) > 0 {
		fmt.Fprintln(&b, "Confusions")
		for _, confusion := range r.Confusions {
			fmt.Fprintf(&b, "- %s -> %s: %d\n", confusion.Expected, confusion.Actual, confusion.Count)
		}
		fmt.Fprintln(&b)
	}
	for _, c := range r.Cases {
		fmt.Fprintf(&b, "Case: %s\n", c.ID)
		fmt.Fprintf(&b, "- Path: %s\n", c.Path)
		fmt.Fprintf(&b, "- Expected/actual: %s -> %s\n", c.ExpectedClassifier, c.ActualClassifier)
		fmt.Fprintf(&b, "- Confidence: %.3f\n", c.Confidence)
		fmt.Fprintf(&b, "- Result: %s\n", passFail(c.Passed))
		if c.ExpectedSubformat != "" || c.ActualSubformat != "" {
			fmt.Fprintf(&b, "- Subformat: %s -> %s\n", valueOrNone(c.ExpectedSubformat), valueOrNone(c.ActualSubformat))
		}
		if c.ExpectedFamily != "" || c.ActualFamily != "" {
			fmt.Fprintf(&b, "- Family: %s -> %s\n", valueOrNone(c.ExpectedFamily), valueOrNone(c.ActualFamily))
		}
		if c.Ambiguous {
			fmt.Fprintln(&b, "- Ambiguous: true")
		}
		if c.FallbackGeneric {
			fmt.Fprintln(&b, "- Fallback generic: true")
		}
		if len(c.MissingRequiredReasons) > 0 {
			fmt.Fprintf(&b, "- Missing required reasons: %s\n", reasonList(c.MissingRequiredReasons))
		}
		if len(c.ForbiddenClassifierHits) > 0 {
			fmt.Fprintf(&b, "- Forbidden classifier hits: %s\n", strings.Join(c.ForbiddenClassifierHits, ", "))
		}
		if len(c.MissingChildCandidates) > 0 {
			fmt.Fprintf(&b, "- Missing child candidates: %s\n", strings.Join(c.MissingChildCandidates, ", "))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func FormatEvalJSON(r *EvalResult) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func loadFixtureEvalLabels(fixtureAbs string) struct {
	FixtureVersion string `yaml:"fixture_version"`
	EvalStage      string `yaml:"eval_stage"`
} {
	var labels struct {
		FixtureVersion string `yaml:"fixture_version"`
		EvalStage      string `yaml:"eval_stage"`
	}
	data, err := os.ReadFile(filepath.Join(fixtureAbs, "cases.yaml"))
	if err != nil {
		return labels
	}
	_ = yaml.Unmarshal(data, &labels)
	return labels
}

func goldenChildPaths(children []GoldenChildCandidate) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		out = append(out, filepath.ToSlash(child.Path))
	}
	sort.Strings(out)
	return out
}

func candidatePaths(candidates []Candidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, filepath.ToSlash(candidate.Path))
	}
	sort.Strings(out)
	return out
}

func missingStrings(want, got []string) []string {
	gotSet := map[string]bool{}
	for _, item := range got {
		gotSet[filepath.ToSlash(item)] = true
	}
	var missing []string
	for _, item := range want {
		item = filepath.ToSlash(item)
		if !gotSet[item] {
			missing = append(missing, item)
		}
	}
	return missing
}

func classificationHasReason(cl Classification, want ReasonCode) bool {
	for _, reason := range cl.PositiveReasons {
		if reason.Code == want {
			return true
		}
	}
	for _, reason := range cl.NegativeReasons {
		if reason.Code == want {
			return true
		}
	}
	return false
}

func matchesIfExpected(expected, actual Scope) bool {
	return expected == "" || expected == actual
}

func matchesIfExpectedString(expected, actual string) bool {
	return strings.TrimSpace(expected) == "" || strings.TrimSpace(expected) == strings.TrimSpace(actual)
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func reasonList(reasons []ReasonCode) string {
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}

func pct(value float64) string {
	return fmt.Sprintf("%.1f%%", value*100)
}

func passFail(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}
