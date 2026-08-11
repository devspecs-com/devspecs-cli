package classify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatEvalText_WithFailedDetailedCase_RendersClassificationEvidence(t *testing.T) {
	report := &EvalResult{
		Fixture: "document-models", FixtureVersion: "v3", EvalStage: "release",
		Evaluator: EvaluatorDeclarativeDocumentModelsV0, ClassifierProfile: "default",
		ConfigVersion: 2, ResultsFile: "results/classifier.json",
		Summary: EvalSummary{
			Cases: 2, PassedCases: 1, Accuracy: 0.5,
			SubformatFamilyCases: 2, SubformatFamilyPassed: 1, SubformatFamilyAccuracy: 0.5,
			DiscoveryCoverage: 0.75, AmbiguousCases: 1, AmbiguityRate: 0.5,
			GenericFallbackCases: 1, GenericFallbackRate: 0.5, RejectedCases: 1, RejectRate: 0.5,
			ReasonCoverageCases: 2, ReasonCoveragePassed: 1, ReasonCoverageRate: 0.5,
			ChildCandidateExpected: 2, ChildCandidateMatched: 1, ChildCandidateCoverage: 0.5,
		},
		Models:     []EvalModelSummary{{Model: "adr", Expected: 1, Predicted: 2, Precision: 0.5, Recall: 1}},
		Confusions: []EvalConfusion{{Expected: "openspec", Actual: "markdown", Count: 1}},
		Cases: []EvalCaseResult{{
			ID: "openspec-proposal", Path: "openspec/changes/login/proposal.md",
			ExpectedClassifier: "openspec", ActualClassifier: "markdown", Confidence: 0.42,
			ExpectedSubformat: "proposal", ExpectedFamily: "openspec", ActualFamily: "generic",
			Ambiguous: true, FallbackGeneric: true,
			MissingRequiredReasons:  []ReasonCode{"path_openspec"},
			ForbiddenClassifierHits: []string{"markdown"}, MissingChildCandidates: []string{"tasks.md"},
		}},
	}

	output := FormatEvalText(report)

	assert.True(t, strings.HasPrefix(output, "DevSpecs Classifier Eval: document-models"))
	assert.Contains(t, output, "Child candidate coverage: 1/2 = 50.0%")
	assert.Contains(t, output, "openspec -> markdown: 1")
	assert.Contains(t, output, "Result: fail")
	assert.Contains(t, output, "Subformat: proposal -> none")
	assert.Contains(t, output, "Missing required reasons: path_openspec")
	assert.Contains(t, output, "Missing child candidates: tasks.md")
}
