package classify

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunEval_AgenticSaaSClassifierGoldens(t *testing.T) {
	result, err := RunEval(filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), EvalOptions{})
	require.NoError(t, err)

	require.Equal(t, EvaluatorDeclarativeDocumentModelsV0, result.Evaluator,
		"evaluator = %q", result.Evaluator)
	require.Equal(t, ProfileBuiltinIntentDocsV1, result.ClassifierProfile,
		"profile = %q", result.ClassifierProfile)
	require.Equal(t, "agentic-saas-fragmented-v1", result.FixtureVersion,
		"fixture version = %q", result.FixtureVersion)
	require.Equal(t, "seed_smoke", result.EvalStage,
		"eval stage = %q", result.EvalStage)
	require.False(t, result.Summary.Cases < 6,
		"cases = %d", result.Summary.Cases)
	require.InDelta(t, 1.0, result.Summary.FixturePathCoverage, 0,
		"fixture coverage = %.3f", result.Summary.FixturePathCoverage)
	require.NotEqual(t, 0, result.Summary.Accuracy,
		"expected non-zero classifier accuracy")
	require.NotEqual(t, 0, result.Summary.ReasonCoverageCases,
		"expected reason coverage cases")
	require.NotEqual(t, 0, result.Summary.GenericFallbackCases,
		"expected at least one generic fallback case")
	require.NotZero(t, result.Summary.ChildCandidateExpected,
		"child candidate coverage = %#v", result.Summary)
	require.InDelta(t, 1.0, result.Summary.ChildCandidateCoverage, 0,
		"child candidate coverage = %#v", result.Summary)
	require.NotEmpty(t, result.Models,
		"expected model summaries")

	for _, c := range result.Cases {
		assert.NotEmpty(t, c.ExpectedClassifier, c.ID)
		assert.NotEmpty(t, c.ActualClassifier, c.ID)
		require.NotEmpty(t, c.PositiveReasons,
			"%s: expected positive reasons", c.ID)

	}
}

func TestFormatEvalJSON(t *testing.T) {
	result, err := RunEval(filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), EvalOptions{})
	require.NoError(t, err)

	data, err := FormatEvalJSON(result)
	require.NoError(t, err)

	require.NotEmpty(t, data,
		"expected JSON")

}
