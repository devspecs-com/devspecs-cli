package classify

import (
	"path/filepath"
	"testing"
)

func TestRunEval_AgenticSaaSClassifierGoldens(t *testing.T) {
	result, err := RunEval(filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Evaluator != EvaluatorDeclarativeDocumentModelsV0 {
		t.Fatalf("evaluator = %q", result.Evaluator)
	}
	if result.ClassifierProfile != ProfileBuiltinIntentDocsV1 {
		t.Fatalf("profile = %q", result.ClassifierProfile)
	}
	if result.FixtureVersion != "agentic-saas-fragmented-v1" {
		t.Fatalf("fixture version = %q", result.FixtureVersion)
	}
	if result.EvalStage != "seed_smoke" {
		t.Fatalf("eval stage = %q", result.EvalStage)
	}
	if result.Summary.Cases < 6 {
		t.Fatalf("cases = %d", result.Summary.Cases)
	}
	if result.Summary.FixturePathCoverage != 1 {
		t.Fatalf("fixture coverage = %.3f", result.Summary.FixturePathCoverage)
	}
	if result.Summary.Accuracy == 0 {
		t.Fatal("expected non-zero classifier accuracy")
	}
	if result.Summary.ReasonCoverageCases == 0 {
		t.Fatal("expected reason coverage cases")
	}
	if result.Summary.GenericFallbackCases == 0 {
		t.Fatal("expected at least one generic fallback case")
	}
	if result.Summary.ChildCandidateExpected == 0 || result.Summary.ChildCandidateCoverage != 1 {
		t.Fatalf("child candidate coverage = %#v", result.Summary)
	}
	if len(result.Models) == 0 {
		t.Fatal("expected model summaries")
	}
	for _, c := range result.Cases {
		if c.ExpectedClassifier == "" || c.ActualClassifier == "" {
			t.Fatalf("missing classifiers in case: %#v", c)
		}
		if len(c.PositiveReasons) == 0 {
			t.Fatalf("%s: expected positive reasons", c.ID)
		}
	}
}

func TestFormatEvalJSON(t *testing.T) {
	result, err := RunEval(filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), EvalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := FormatEvalJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected JSON")
	}
}
