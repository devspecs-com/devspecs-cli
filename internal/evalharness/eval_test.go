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
	if result.Summary.MeanMustHaveRecall == 0 {
		t.Fatal("expected must-have recall to be reported")
	}
	if result.Summary.ContextSufficiencyCases == 0 {
		t.Fatal("expected context sufficiency cases")
	}
	if result.Summary.Pareto.MeanMustHaveRecall != result.Summary.MeanMustHaveRecall {
		t.Fatalf("pareto must-have recall mismatch: %#v", result.Summary.Pareto)
	}
	if result.Summary.MeanArtifactPrecision >= 0.95 {
		t.Fatalf("seed eval should expose distractor precision gaps, got %.3f", result.Summary.MeanArtifactPrecision)
	}
	if result.Summary.MedianTokenReductionVsFullPlanning <= 0 {
		t.Fatalf("expected positive full-planning reduction, got %.3f", result.Summary.MedianTokenReductionVsFullPlanning)
	}

	sufficiencyPasses := 0
	sufficiencyFailures := 0
	weightedCaseSeen := false
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
		if c.MustExpectedCount > 0 && c.MustExpectedCount != c.ExpectedRelevantCount {
			weightedCaseSeen = true
		}
		if c.ContextSufficiency.Configured {
			if c.ContextSufficiency.Passed {
				sufficiencyPasses++
			} else {
				sufficiencyFailures++
			}
		}
		if len(c.ArtifactReasons) != len(c.ArtifactsIncluded) {
			t.Fatalf("%s: artifact reason count mismatch", c.ID)
		}
		for _, reason := range c.ArtifactReasons {
			if reason.Path == "" || len(reason.Reasons) == 0 {
				t.Fatalf("%s: missing artifact reason: %#v", c.ID, reason)
			}
		}
	}
	if !weightedCaseSeen {
		t.Fatal("expected at least one case with helpful/background relevance")
	}
	if sufficiencyPasses == 0 || sufficiencyFailures == 0 {
		t.Fatalf("expected sufficiency passes and failures, got pass=%d fail=%d", sufficiencyPasses, sufficiencyFailures)
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
