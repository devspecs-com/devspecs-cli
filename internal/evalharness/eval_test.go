package evalharness

import (
	"path/filepath"
	"testing"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")
}

func TestRun_AgenticSaaSFixture(t *testing.T) {
	result, err := Run(fixturePath(t), Options{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Retriever != "eval_weighted_files_v0" {
		t.Fatalf("retriever label = %q", result.Retriever)
	}
	if result.TokenCounter != "approx_chars_div_4" {
		t.Fatalf("token counter = %q", result.TokenCounter)
	}
	if result.TokenizerProfile.Approximation != "ceil(chars / 4.0)" {
		t.Fatalf("missing tokenizer approximation profile: %#v", result.TokenizerProfile)
	}
	if result.PricingProfile.Name != "none" {
		t.Fatalf("pricing profile = %#v", result.PricingProfile)
	}
	if result.FixtureVersion != "agentic-saas-fragmented-v1" {
		t.Fatalf("fixture version = %q", result.FixtureVersion)
	}
	if result.EvalStage != "seed_smoke" {
		t.Fatalf("eval stage = %q", result.EvalStage)
	}
	if len(result.Cases) < 8 {
		t.Fatalf("cases = %d", len(result.Cases))
	}
	if result.Corpus.PlanningArtifacts.Files == 0 || result.Corpus.FullCandidateCorpus.Tokens == 0 {
		t.Fatalf("missing corpus summary: %#v", result.Corpus)
	}
	if result.Corpus.FullCandidateCorpus.Tokens < 20000 {
		t.Fatalf("fixture corpus too small: %d tokens", result.Corpus.FullCandidateCorpus.Tokens)
	}
	if result.Corpus.PlanningArtifacts.Tokens < 20000 {
		t.Fatalf("planning corpus too small: %d tokens", result.Corpus.PlanningArtifacts.Tokens)
	}
	if result.Summary.MeanArtifactRecall < 0.5 {
		t.Fatalf("mean recall too low: %.3f", result.Summary.MeanArtifactRecall)
	}
	if result.Summary.MeanArtifactPrecision >= 0.95 {
		t.Fatalf("seed eval should expose distractor precision gaps, got %.3f", result.Summary.MeanArtifactPrecision)
	}
	if result.Summary.MedianTokenReductionVsFullPlanning <= 0 {
		t.Fatalf("expected positive full-planning reduction, got %.3f", result.Summary.MedianTokenReductionVsFullPlanning)
	}

	for _, c := range result.Cases {
		if c.DevSpecsTokens <= 0 {
			t.Fatalf("%s: expected devspecs tokens", c.ID)
		}
		if c.FullPlanningTokens <= 0 || c.QueryFileBaselineTokens <= 0 {
			t.Fatalf("%s: expected baseline tokens", c.ID)
		}
		if len(c.MissedExpectedRelevant) == 0 && c.ArtifactRecall != 1 {
			t.Fatalf("%s: recall/missed mismatch", c.ID)
		}
	}
}

func TestRun_ThresholdFailure(t *testing.T) {
	minRecall := 1.01
	result, err := Run(fixturePath(t), Options{MinRecall: &minRecall})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.FailedThresholdCount == 0 {
		t.Fatal("expected threshold failures")
	}

	minMeanRecall := 1.01
	result, err = Run(fixturePath(t), Options{MinMeanRecall: &minMeanRecall})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.FailedThresholdCount == 0 {
		t.Fatal("expected aggregate threshold failure")
	}
}
