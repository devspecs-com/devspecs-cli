package evalharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")
}

func TestRun_AgenticSaaSFixture(t *testing.T) {
	result, err := Run(fixturePath(t), Options{CorpusSource: CorpusSourceFilesystemFixture})
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
	if result.CorpusSource != CorpusSourceFilesystemFixture {
		t.Fatalf("corpus source = %q", result.CorpusSource)
	}
	if result.ProductPath != ProductPathLabOnly {
		t.Fatalf("product path = %q", result.ProductPath)
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
	if result.Summary.MeanTokenReductionVsQueryFileBaseline == 0 {
		t.Fatalf("expected mean query-baseline token reduction to be reported")
	}
	if result.Summary.MeanGradedPrecision <= 0 || result.Summary.MeanGradedPrecision > 1 {
		t.Fatalf("expected bounded graded precision, exact=%.3f graded=%.3f", result.Summary.MeanArtifactPrecision, result.Summary.MeanGradedPrecision)
	}
	if result.Summary.MeanPenalizedUtilityPrecision < 0 || result.Summary.MeanPenalizedUtilityPrecision > result.Summary.MeanGradedPrecision {
		t.Fatalf("expected bounded penalized utility precision, graded=%.3f penalized=%.3f", result.Summary.MeanGradedPrecision, result.Summary.MeanPenalizedUtilityPrecision)
	}
	if result.Summary.GradeCounts.Must == 0 || result.Summary.GradeCounts.Unlabeled == 0 {
		t.Fatalf("expected summary grade counts: %#v", result.Summary.GradeCounts)
	}
	if result.Summary.MedianTokenReductionVsFullPlanning <= 0 {
		t.Fatalf("expected positive full-planning reduction, got %.3f", result.Summary.MedianTokenReductionVsFullPlanning)
	}
	if result.Diagnostics.ExpectedRelevantCount == 0 {
		t.Fatalf("expected eval diagnostics: %#v", result.Diagnostics)
	}
	if result.Diagnostics.DiscoveryCoverage != 1 {
		t.Fatalf("filesystem fixture should expose all expected artifacts, got discovery coverage %.3f: %#v", result.Diagnostics.DiscoveryCoverage, result.Diagnostics)
	}
	if len(result.Diagnostics.RoleSummaries) == 0 {
		t.Fatalf("expected diagnostic role summaries: %#v", result.Diagnostics)
	}
	if result.AgentMetrics.MustHitAt3 == 0 {
		t.Fatalf("expected agent must-hit@3 metrics: %#v", result.AgentMetrics)
	}
	if len(result.AgentMetrics.ContextSufficiencyAtTokenBudget) != 4 {
		t.Fatalf("expected token-budget sufficiency metrics: %#v", result.AgentMetrics.ContextSufficiencyAtTokenBudget)
	}
	if result.Summary.AgentMetrics.MustHitAt3 != result.AgentMetrics.MustHitAt3 {
		t.Fatalf("summary agent metrics mismatch: %#v vs %#v", result.Summary.AgentMetrics, result.AgentMetrics)
	}
	if len(result.LaneMetrics) != 5 {
		t.Fatalf("expected five lane metrics: %#v", result.LaneMetrics)
	}
	if result.MetricNotes[LanePackedSections] == "" {
		t.Fatalf("expected metric notes: %#v", result.MetricNotes)
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
		if len(c.ArtifactGrades) != len(c.ArtifactsIncluded) {
			t.Fatalf("%s: artifact grade count mismatch", c.ID)
		}
		if c.AgentMetrics.IncludedArtifacts != len(c.ArtifactsIncluded) {
			t.Fatalf("%s: agent included count mismatch", c.ID)
		}
		if c.AgentMetrics.StrictPrecision != c.ArtifactPrecision {
			t.Fatalf("%s: strict precision mismatch", c.ID)
		}
		if c.DiscoveryCoverage == 0 {
			t.Fatalf("%s: expected case discovery diagnostics", c.ID)
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

func TestRun_DefaultUsesIndexedCorpus(t *testing.T) {
	result, err := Run(fixturePath(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.CorpusSource != CorpusSourceSQLiteIndex {
		t.Fatalf("corpus source = %q", result.CorpusSource)
	}
	if result.ProductPath != ProductPathIndexedHarness {
		t.Fatalf("product path = %q", result.ProductPath)
	}
	if result.Corpus.PlanningArtifacts.Files == 0 {
		t.Fatalf("expected indexed planning artifacts: %#v", result.Corpus)
	}
	if result.Diagnostics.ExpectedMissingFromCorpusCount != 0 {
		t.Fatalf("expected indexed corpus coverage gaps to be closed, got %#v", result.Diagnostics)
	}
	if result.Diagnostics.DiscoveryCoverage != 1 {
		t.Fatalf("expected complete indexed discovery coverage, got %.3f", result.Diagnostics.DiscoveryCoverage)
	}
	if result.Diagnostics.MissedAfterDiscoveryCount == 0 {
		t.Fatalf("expected remaining retrieval gaps after discovery: %#v", result.Diagnostics)
	}
	if result.Diagnostics.OpenSpec == nil {
		t.Fatalf("expected OpenSpec structural diagnostics: %#v", result.Diagnostics)
	}
	if result.Diagnostics.OpenSpec.BundleRecall != 1 {
		t.Fatalf("expected complete OpenSpec bundle recall, got %#v", result.Diagnostics.OpenSpec)
	}
	if result.Diagnostics.OpenSpec.ChildRoleRecall != 1 {
		t.Fatalf("expected complete OpenSpec child-role recall, got %#v", result.Diagnostics.OpenSpec)
	}
	if result.Diagnostics.OpenSpec.MarkdownLeakage != 0 {
		t.Fatalf("expected no OpenSpec markdown leakage, got %#v", result.Diagnostics.OpenSpec)
	}
}

func TestRun_IndexedEvalExercisesSectionAwareRetrievalAndAblation(t *testing.T) {
	root := t.TempDir()
	writeSectionEvalFixture(t, root)

	result, err := Run(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Retriever != "eval_weighted_files_v0" {
		t.Fatalf("retriever = %q", result.Retriever)
	}
	if len(result.Cases) != 1 || result.Cases[0].SectionSelectedCount == 0 {
		t.Fatalf("expected section-selected artifact in default indexed eval: %#v", result.Cases)
	}

	disabled, err := Run(root, Options{DisableSectionAwareRetrieval: true})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Retriever != "eval_weighted_files_v0_no_section_retrieval" {
		t.Fatalf("retriever = %q", disabled.Retriever)
	}
	if len(disabled.Cases) != 1 || disabled.Cases[0].SectionSelectedCount != 0 {
		t.Fatalf("expected ablation to disable section-selected artifacts: %#v", disabled.Cases)
	}
}

func TestRun_BudgetedContextPackingTrimsAfterRanking(t *testing.T) {
	root := t.TempDir()
	writeBudgetedPackingFixture(t, root)

	result, err := Run(root, Options{
		CorpusSource:                 CorpusSourceFilesystemFixture,
		ExperimentalBalancedEvidence: true,
		ExperimentalBudgetedPacking:  true,
		ContextTokenBudget:           650,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cases) != 1 {
		t.Fatalf("cases = %d", len(result.Cases))
	}
	c := result.Cases[0]
	if c.ContextTokenBudget != 650 {
		t.Fatalf("expected context budget metadata, got %#v", c)
	}
	if c.PreBudgetDevSpecsTokens <= c.DevSpecsTokens {
		t.Fatalf("expected pre-budget tokens to exceed packed tokens, pre=%d post=%d", c.PreBudgetDevSpecsTokens, c.DevSpecsTokens)
	}
	if c.DevSpecsTokens > c.ContextTokenBudget {
		t.Fatalf("expected context to fit budget, got %d > %d", c.DevSpecsTokens, c.ContextTokenBudget)
	}
	if len(c.ContextBudgetDroppedArtifacts) == 0 {
		t.Fatalf("expected dropped artifacts: %#v", c)
	}
	if c.ContextSufficiency.Passed {
		t.Fatalf("budgeted pack should expose sufficiency tradeoff when required context is dropped")
	}
}

func TestRun_IndexedCacheTelemetryAndBudgets(t *testing.T) {
	root := t.TempDir()
	writeTinyEvalFixture(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")

	first, err := Run(root, Options{
		IndexCacheDir:        cacheDir,
		MaxSourceFiles:       1,
		MaxTestCaseArtifacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.IndexCache == nil || !first.IndexCache.Enabled || first.IndexCache.Hit {
		t.Fatalf("expected enabled cache miss on first run: %#v", first.IndexCache)
	}
	if first.IndexCache.CorpusFingerprint == "" || first.IndexCache.ProvenanceFingerprint == "" {
		t.Fatalf("expected cache provenance fingerprints: %#v", first.IndexCache)
	}
	if len(first.PhaseTelemetry) == 0 {
		t.Fatalf("expected phase telemetry")
	}
	if !hasPhase(first.PhaseTelemetry, "index_or_load_corpus") || !hasPhase(first.PhaseTelemetry, "case") {
		t.Fatalf("missing expected phases: %#v", first.PhaseTelemetry)
	}

	second, err := Run(root, Options{
		IndexCacheDir:        cacheDir,
		MaxSourceFiles:       1,
		MaxTestCaseArtifacts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.IndexCache == nil || !second.IndexCache.Hit || second.IndexCache.Key != first.IndexCache.Key {
		t.Fatalf("expected cache hit with same key: first=%#v second=%#v", first.IndexCache, second.IndexCache)
	}

	withTests, err := Run(root, Options{IndexCacheDir: cacheDir, TestCaseArtifacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if withTests.IndexCache == nil || withTests.IndexCache.Key == first.IndexCache.Key {
		t.Fatalf("include-tests should change cache key: first=%#v withTests=%#v", first.IndexCache, withTests.IndexCache)
	}
}

func TestCollectIndexedFiles_PreScanArtifactBudgets(t *testing.T) {
	root := t.TempDir()
	writeTinyEvalFixture(t, root)

	files, _, _, err := collectIndexedFiles(root, Options{
		TestCaseArtifacts:    true,
		CodeCommentArtifacts: true,
		MaxSourceFiles:       1,
		MaxTestCaseArtifacts: 1,
		MaxCodeComments:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var sourceFiles, testCases, codeComments int
	for _, f := range files {
		switch {
		case isTestCaseFile(f):
			testCases++
		case isCodeCommentFile(f):
			codeComments++
		case retrieval.IsSourceContextCandidate(f):
			sourceFiles++
		}
	}
	if sourceFiles > 1 {
		t.Fatalf("source files should be capped before eval corpus use, got %d in %#v", sourceFiles, files)
	}
	if testCases > 1 {
		t.Fatalf("test cases should be capped before eval corpus use, got %d in %#v", testCases, files)
	}
	if codeComments > 1 {
		t.Fatalf("code comments should be capped before eval corpus use, got %d in %#v", codeComments, files)
	}
}

func TestIndexedCorpusFingerprintIgnoresRetrievalOnlySource(t *testing.T) {
	root := t.TempDir()
	writeFingerprintSource(t, root, filepath.Join("internal", "adapters", "adapter.go"), "package adapters\nconst AdapterVersion = 1\n")
	writeFingerprintSource(t, root, filepath.Join("internal", "retrieval", "retrieval.go"), "package retrieval\nconst RetrievalVersion = 1\n")

	initial, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	if err != nil {
		t.Fatal(err)
	}
	writeFingerprintSource(t, root, filepath.Join("internal", "retrieval", "retrieval.go"), "package retrieval\nconst RetrievalVersion = 2\n")
	retrievalOnly, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	if err != nil {
		t.Fatal(err)
	}
	if retrievalOnly != initial {
		t.Fatalf("retrieval-only change should not affect indexed corpus fingerprint: initial=%s retrieval=%s", initial, retrievalOnly)
	}

	writeFingerprintSource(t, root, filepath.Join("internal", "adapters", "adapter.go"), "package adapters\nconst AdapterVersion = 2\n")
	adapterChange, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	if err != nil {
		t.Fatal(err)
	}
	if adapterChange == retrievalOnly {
		t.Fatalf("adapter change should affect indexed corpus fingerprint: %s", adapterChange)
	}
}

func writeFingerprintSource(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRun_ThresholdFailure(t *testing.T) {
	minRecall := 1.01
	result, err := Run(fixturePath(t), Options{MinRecall: &minRecall, CorpusSource: CorpusSourceFilesystemFixture})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.FailedThresholdCount == 0 {
		t.Fatal("expected threshold failures")
	}

	minMeanRecall := 1.01
	result, err = Run(fixturePath(t), Options{MinMeanRecall: &minMeanRecall, CorpusSource: CorpusSourceFilesystemFixture})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.FailedThresholdCount == 0 {
		t.Fatal("expected aggregate threshold failure")
	}
}

func hasPhase(phases []PhaseTelemetry, name string) bool {
	for _, phase := range phases {
		if phase.Name == name && phase.DurationMS >= 0 && phase.Status != "" {
			return true
		}
	}
	return false
}

func writeTinyEvalFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "alpha.md"), []byte("# Alpha plan\n\nThis plan covers billing retry behavior.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "// TODO because retry behavior must stay compatible with legacy billing callers.\nexport function retryBilling() { return true }\n"
	if err := os.WriteFile(filepath.Join(root, "src", "billing.ts"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	testSource := "describe(\"billing retries\", () => {\n  it(\"rejects duplicate replay\", () => {\n    expect(retryBilling()).toBe(true)\n  })\n  it(\"keeps legacy compatibility\", () => {\n    expect(retryBilling()).toBe(true)\n  })\n})\n"
	if err := os.WriteFile(filepath.Join(root, "src", "billing.test.ts"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := "fixture_version: tiny-v0\n" +
		"eval_stage: tiny_eval\n\n" +
		"cases:\n" +
		"  - id: alpha\n" +
		"    query: \"billing retry behavior\"\n" +
		"    expected_relevant:\n" +
		"      - path: docs/plans/alpha.md\n" +
		"        importance: must\n"
	if err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSectionEvalFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# Broad Plan\n\n" +
		strings.Repeat("General engineering notes without the target terms.\n", 80) +
		"\n## Replay Boundary\n\n" +
		"stripe_event_id idempotency protects webhook replay behavior.\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "broad.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := "fixture_version: section-v0\n" +
		"eval_stage: section_eval\n\n" +
		"cases:\n" +
		"  - id: section-replay\n" +
		"    query: \"stripe_event_id idempotency\"\n" +
		"    expected_relevant:\n" +
		"      - path: docs/plans/broad.md\n" +
		"        importance: must\n"
	if err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeBudgetedPackingFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	one := "# Billing Retry Plan\n\n" +
		"billing retry behavior stripe_event_id idempotency primary context.\n\n" +
		strings.Repeat("primary filler context keeps this artifact moderate.\n", 20)
	two := "# Billing Retry Appendix\n\n" +
		"billing retry behavior webhook replay secondary context.\n\n" +
		strings.Repeat("secondary filler context makes this artifact droppable.\n", 50)
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing-retry.md"), []byte(one), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing-retry-appendix.md"), []byte(two), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := "fixture_version: budget-v0\n" +
		"eval_stage: budget_eval\n\n" +
		"cases:\n" +
		"  - id: budget-billing\n" +
		"    query: \"billing retry behavior\"\n" +
		"    expected_relevant:\n" +
		"      - path: docs/plans/billing-retry.md\n" +
		"        importance: must\n" +
		"      - path: docs/plans/billing-retry-appendix.md\n" +
		"        importance: helpful\n" +
		"    success_criteria:\n" +
		"      must_contain_artifacts:\n" +
		"        - docs/plans/billing-retry.md\n" +
		"        - docs/plans/billing-retry-appendix.md\n"
	if err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
	}
}
