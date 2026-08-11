package evalharness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")
}

func TestRun_AgenticSaaSFixture(t *testing.T) {
	result, err := Run(fixturePath(t), Options{CorpusSource: CorpusSourceFilesystemFixture})
	require.NoError(t, err)
	require.Equal(t, "eval_weighted_files_v0", result.Retriever,
		"retriever label = %q", result.Retriever)
	require.Equal(t, "approx_chars_div_4", result.TokenCounter,
		"token counter = %q", result.TokenCounter)
	require.Equal(t, "ceil(chars / 4.0)", result.TokenizerProfile.Approximation,
		"missing tokenizer approximation profile: %#v", result.TokenizerProfile)
	require.Equal(t, "none", result.PricingProfile.Name,
		"pricing profile = %#v", result.PricingProfile)
	require.Equal(t, "agentic-saas-fragmented-v1", result.FixtureVersion,
		"fixture version = %q", result.FixtureVersion)
	require.Equal(t, "seed_smoke", result.EvalStage,
		"eval stage = %q", result.EvalStage)
	require.Equal(t, CorpusSourceFilesystemFixture, result.CorpusSource,
		"corpus source = %q", result.CorpusSource)
	require.Equal(t, ProductPathLabOnly, result.ProductPath,
		"product path = %q", result.ProductPath)
	require.GreaterOrEqual(t, len(result.Cases), 8,
		"cases = %d", len(result.Cases))
	assert.Positive(t, result.Corpus.PlanningArtifacts.Files)
	assert.Positive(t, result.Corpus.FullCandidateCorpus.Tokens)
	require.GreaterOrEqual(t, result.Corpus.FullCandidateCorpus.Tokens, 20000,
		"fixture corpus too small: %d tokens", result.Corpus.FullCandidateCorpus.Tokens)
	require.GreaterOrEqual(t, result.Corpus.PlanningArtifacts.Tokens, 20000,
		"planning corpus too small: %d tokens", result.Corpus.PlanningArtifacts.Tokens)
	require.GreaterOrEqual(t, result.Summary.MeanArtifactRecall, 0.5,
		"mean recall too low: %.3f", result.Summary.MeanArtifactRecall)
	assert.NotZero(t, result.Summary.MeanMustHaveRecall,
		"expected must-have recall to be reported")
	require.NotEqual(t, 0, result.Summary.ContextSufficiencyCases,
		"expected context sufficiency cases")
	require.Equal(t, result.Summary.MeanMustHaveRecall, result.Summary.Pareto.MeanMustHaveRecall,
		"pareto must-have recall mismatch: %#v", result.Summary.Pareto)
	require.Less(t, result.Summary.MeanArtifactPrecision, 0.95,
		"seed eval should expose distractor precision gaps, got %.3f", result.Summary.MeanArtifactPrecision)
	assert.NotZero(t, result.Summary.MeanTokenReductionVsQueryFileBaseline,
		"expected mean query-baseline token reduction to be reported")
	assert.Positive(t, result.Summary.MeanGradedPrecision)
	assert.LessOrEqual(t, result.Summary.MeanGradedPrecision, 1.0)
	assert.GreaterOrEqual(t, result.Summary.MeanPenalizedUtilityPrecision, 0.0)
	assert.LessOrEqual(t, result.Summary.MeanPenalizedUtilityPrecision, result.Summary.MeanGradedPrecision)
	assert.Positive(t, result.Summary.GradeCounts.Must)
	assert.Positive(t, result.Summary.GradeCounts.Unlabeled)
	assert.Positive(t, result.Summary.MedianTokenReductionVsFullPlanning,
		"expected positive full-planning reduction, got %.3f", result.Summary.MedianTokenReductionVsFullPlanning)
	require.NotEqual(t, 0, result.Diagnostics.ExpectedRelevantCount,
		"expected eval diagnostics: %#v", result.Diagnostics)
	assert.InDelta(t, 1, result.Diagnostics.DiscoveryCoverage, 0,
		"filesystem fixture should expose all expected artifacts, got discovery coverage %.3f: %#v", result.Diagnostics.DiscoveryCoverage, result.Diagnostics)
	require.NotEmpty(t, result.Diagnostics.RoleSummaries,
		"expected diagnostic role summaries: %#v", result.Diagnostics)
	require.NotEmpty(t, result.Diagnostics.FalsePositiveSummaries,
		"expected primary false-positive summaries: %#v", result.Diagnostics)
	require.NotEmpty(t, result.Diagnostics.ExtensionSummaries,
		"expected extension summaries: %#v", result.Diagnostics)
	require.NotEqual(t, 0, result.AgentMetrics.MustHitAt3,
		"expected agent must-hit@3 metrics: %#v", result.AgentMetrics)
	require.Len(t, result.AgentMetrics.ContextSufficiencyAtTokenBudget, 4,
		"expected token-budget sufficiency metrics: %#v", result.AgentMetrics.ContextSufficiencyAtTokenBudget)
	require.Equal(t, result.AgentMetrics.MustHitAt3, result.Summary.AgentMetrics.MustHitAt3,
		"summary agent metrics mismatch: %#v vs %#v", result.Summary.AgentMetrics, result.AgentMetrics)
	require.Len(t, result.LaneMetrics, 5,
		"expected five lane metrics: %#v", result.LaneMetrics)
	require.Len(t, result.CanonicalLanes, 7,
		"expected seven canonical lane metrics: %#v", result.CanonicalLanes)
	require.NotEqual(t, "", result.MetricNotes[LanePackedSections],
		"expected metric notes: %#v", result.MetricNotes)
	require.NotEqual(t, "", result.MetricNotes[CanonicalLaneIntent],
		"expected canonical lane metric notes: %#v", result.MetricNotes)

	sufficiencyPasses := 0
	sufficiencyFailures := 0
	weightedCaseSeen := false
	for _, c := range result.Cases {
		require.Greater(t, c.DevSpecsTokens, 0,
			"%s: expected devspecs tokens", c.ID)
		assert.Positive(t, c.FullPlanningTokens, "%s: full planning tokens", c.ID)
		assert.Positive(t, c.QueryFileBaselineTokens, "%s: query baseline tokens", c.ID)
		if len(c.MissedExpectedRelevant) == 0 {
			assert.Equal(t, 1.0, c.ArtifactRecall, "%s: recall", c.ID)
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
		require.Len(t, c.ArtifactReasons, len(c.ArtifactsIncluded),
			"%s: artifact reason count mismatch", c.ID)
		require.Len(t, c.ArtifactGrades, len(c.ArtifactsIncluded),
			"%s: artifact grade count mismatch", c.ID)

		for _, grade := range c.ArtifactGrades {
			require.NotEqual(t, "", grade.CanonicalLane,
				"%s: missing canonical lane on artifact grade: %#v", c.ID, grade)

		}
		for _, fp := range c.PrimaryFalsePositiveDiagnostics {
			assert.Equal(t, c.ID, fp.CaseID)
			assert.NotEmpty(t, fp.Path)
			assert.Positive(t, fp.Position)
			assert.NotEmpty(t, fp.Lane)
			assert.NotEmpty(t, fp.Role)
			assert.NotEmpty(t, fp.Grade)
			assert.NotEmpty(t, fp.ReasonClass)

		}
		require.Equal(t, len(c.ArtifactsIncluded), c.AgentMetrics.IncludedArtifacts,
			"%s: agent included count mismatch", c.ID)
		require.Equal(t, c.ArtifactPrecision, c.AgentMetrics.StrictPrecision,
			"%s: strict precision mismatch", c.ID)
		assert.NotZero(t, c.DiscoveryCoverage,
			"%s: expected case discovery diagnostics", c.ID)

		for _, reason := range c.ArtifactReasons {
			assert.NotEmpty(t, reason.Path)
			assert.NotEmpty(t, reason.Reasons)

		}
	}
	require.True(t, weightedCaseSeen,
		"expected at least one case with helpful/background relevance")
	assert.Positive(t, sufficiencyPasses)
	assert.Positive(t, sufficiencyFailures)

}

func TestRun_PackDiagnostics(t *testing.T) {
	result, err := Run(fixturePath(t), Options{CorpusSource: CorpusSourceFilesystemFixture, PackDiagnostics: true})
	require.NoError(t, err)
	require.NotEmpty(t, result.Cases,
		"expected eval cases")
	require.NotNil(t, result.Cases[0].PackDiagnostics,
		"expected per-case pack diagnostics")
	require.Equal(t, "role_grouped_pack_v0", result.Cases[0].PackDiagnostics.Mode,
		"pack diagnostics mode = %q", result.Cases[0].PackDiagnostics.Mode)
	require.NotEmpty(t, result.Cases[0].PackDiagnostics.Groups,
		"expected grouped pack diagnostics: %#v", result.Cases[0].PackDiagnostics)
	require.NotNil(t, result.Cases[0].PackSummary)
	assert.Positive(t, result.Cases[0].PackSummary.IncludedCount)
	require.Equal(t, len(result.Cases), result.Summary.PackDiagnosticCases,
		"pack diagnostic cases = %d, want %d", result.Summary.PackDiagnosticCases, len(result.Cases))
	assert.Positive(t, result.Summary.MeanPackIncludedArtifacts)
	assert.Positive(t, result.Summary.MeanPackRoleDiversity)

}

func TestApplyGraphContextMetricsSeparatesGraphAssistedHits(t *testing.T) {
	cr := CaseResult{
		ArtifactsIncluded: []string{"src/auth/session.go"},
	}
	spec := CaseSpec{
		ExpectedRelevant: []ExpectedArtifact{
			{Path: "src/auth/session.go", Importance: "must"},
			{Path: "src/auth/session_test.go", Importance: "helpful"},
		},
	}
	graphFiles := []File{
		{Path: "src/auth/session_test.go", Kind: "source_context", Subtype: "test_case", Title: "TestSession"},
	}
	graphReasons := []ArtifactReason{{Path: "src/auth/session_test.go", Reasons: []string{"graph edge: tests_source"}}}

	applyGraphContextMetrics(&cr, spec, graphFiles, graphReasons)
	require.Len(t, cr.GraphContextRelevantIncluded, 1)
	assert.Equal(t, "src/auth/session_test.go", cr.GraphContextRelevantIncluded[0])
	require.Len(t, cr.GraphAssistedRelevantIncluded, 1)
	assert.Equal(t, "src/auth/session_test.go", cr.GraphAssistedRelevantIncluded[0])
	assert.InDelta(t, 1, cr.GraphContextArtifactPrecision, 0,
		"graph context precision = %.3f", cr.GraphContextArtifactPrecision)
	require.Len(t, cr.GraphContextArtifactGrades, 1)
	assert.Equal(t, "helpful", cr.GraphContextArtifactGrades[0].Grade)

}

func TestDiagnosticRole_WithAsciiDocPath_ReturnsAsciiDoc(t *testing.T) {
	got := diagnosticRole("docs/architecture/runtime.adoc")

	assert.Equal(t, "asciidoc", got)
}

func TestDiagnosticExtensionRole_WithAsciiDocPath_ReturnsExtensionAndRole(t *testing.T) {
	ext, role := diagnosticExtensionRole("docs/architecture/runtime.adoc")

	assert.Equal(t, ".adoc", ext)
	assert.Equal(t, "asciidoc", role)
}

func TestSummarizeUnindexedDocuments_WithAsciiDoc_ReturnsAsciiDocSummary(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs", "architecture"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "architecture", "runtime.adoc"), []byte("= Runtime\n"), 0o644))

	summaries := summarizeUnindexedDocuments(tmp, nil)
	require.Len(t, summaries, 1)
	assert.Equal(t, ".adoc", summaries[0].Extension)
	assert.Equal(t, "asciidoc", summaries[0].Role)
	assert.Equal(t, 1, summaries[0].Count)
}

func TestApplyArtifactMetricsMatchesFileExpectedToLineArtifact(t *testing.T) {
	cr := CaseResult{ArtifactsIncluded: []string{"src/auth/session.go#L24-L39"}}
	spec := CaseSpec{ExpectedRelevant: []ExpectedArtifact{{Path: "src/auth/session.go", Importance: "must"}}}

	applyArtifactMetrics(&cr, spec)
	assert.Equal(t, 1.0, cr.ArtifactRecall)
	assert.Equal(t, 1.0, cr.MustHaveRecall)
	require.Len(t, cr.RelevantIncluded, 1)
	assert.Equal(t, "src/auth/session.go#L24-L39", cr.RelevantIncluded[0])
	require.Empty(t, cr.MissedExpectedRelevant,
		"missed = %#v", cr.MissedExpectedRelevant)

}

func TestApplyArtifactMetricsMatchesLineExpectedToFileArtifact(t *testing.T) {
	cr := CaseResult{ArtifactsIncluded: []string{"src/auth/session.go"}}
	spec := CaseSpec{ExpectedRelevant: []ExpectedArtifact{{Path: "src/auth/session.go#L24-L39", Importance: "must"}}}

	applyArtifactMetrics(&cr, spec)
	assert.Equal(t, 1.0, cr.ArtifactRecall)
	assert.Equal(t, 1.0, cr.MustHaveRecall)
	require.Len(t, cr.RelevantIncluded, 1)
	assert.Equal(t, "src/auth/session.go", cr.RelevantIncluded[0])
	require.Empty(t, cr.MissedExpectedRelevant,
		"missed = %#v", cr.MissedExpectedRelevant)

}

func TestApplyArtifactMetricsDoesNotExactMatchDifferentLineRefs(t *testing.T) {
	cr := CaseResult{ArtifactsIncluded: []string{"src/auth/session.go#L60"}}
	spec := CaseSpec{ExpectedRelevant: []ExpectedArtifact{{Path: "src/auth/session.go#L24-L39", Importance: "must"}}}

	applyArtifactMetrics(&cr, spec)
	assert.Zero(t, cr.ArtifactRecall)
	assert.Zero(t, cr.MustHaveRecall)
	require.Len(t, cr.MissedExpectedRelevant, 1)
	assert.Equal(t, "src/auth/session.go#L24-L39", cr.MissedExpectedRelevant[0])

}

func TestApplyDiscoveryDiagnosticsUsesFileLineIdentity(t *testing.T) {
	cr := CaseResult{ArtifactsIncluded: []string{"src/auth/session.go"}}
	spec := CaseSpec{ExpectedRelevant: []ExpectedArtifact{{Path: "src/auth/session.go#L24-L39", Importance: "must"}}}
	corpusPaths := map[string]bool{"src/auth/session.go": true}

	applyArtifactMetrics(&cr, spec)
	applyDiscoveryDiagnostics(&cr, spec, corpusPaths)
	assert.Equal(t, 1, cr.ExpectedAvailableCount)
	assert.Empty(t, cr.ExpectedMissingFromCorpus)
	assert.Empty(t, cr.MissedAfterDiscovery)

}

func TestRun_DefaultUsesIndexedCorpus(t *testing.T) {
	result, err := Run(fixturePath(t), Options{})
	require.NoError(t, err)
	require.Equal(t, CorpusSourceSQLiteIndex, result.CorpusSource,
		"corpus source = %q", result.CorpusSource)
	require.Equal(t, ProductPathIndexedHarness, result.ProductPath,
		"product path = %q", result.ProductPath)
	require.NotEqual(t, 0, result.Corpus.PlanningArtifacts.Files,
		"expected indexed planning artifacts: %#v", result.Corpus)
	require.Equal(t, 0, result.Diagnostics.ExpectedMissingFromCorpusCount,
		"expected indexed corpus coverage gaps to be closed, got %#v", result.Diagnostics)
	assert.InDelta(t, 1, result.Diagnostics.DiscoveryCoverage, 0,
		"expected complete indexed discovery coverage, got %.3f", result.Diagnostics.DiscoveryCoverage)
	require.NotEqual(t, 0, result.Diagnostics.MissedAfterDiscoveryCount,
		"expected remaining retrieval gaps after discovery: %#v", result.Diagnostics)
	require.NotNil(t, result.Diagnostics.OpenSpec,
		"expected OpenSpec structural diagnostics: %#v", result.Diagnostics)
	assert.InDelta(t, 1, result.Diagnostics.OpenSpec.BundleRecall, 0,
		"expected complete OpenSpec bundle recall, got %#v", result.Diagnostics.OpenSpec)
	assert.InDelta(t, 1, result.Diagnostics.OpenSpec.ChildRoleRecall, 0,
		"expected complete OpenSpec child-role recall, got %#v", result.Diagnostics.OpenSpec)
	require.Equal(t, 0, result.Diagnostics.OpenSpec.MarkdownLeakage,
		"expected no OpenSpec markdown leakage, got %#v", result.Diagnostics.OpenSpec)

}

func TestRun_IndexedEvalUsesSectionAwareRetrieval(t *testing.T) {
	root := t.TempDir()
	writeSectionEvalFixture(t, root)

	result, err := Run(root, Options{})
	require.NoError(t, err)
	assert.Equal(t, "eval_weighted_files_v0", result.Retriever)
	require.Len(t, result.Cases, 1)
	assert.Positive(t, result.Cases[0].SectionSelectedCount)
}

func TestRun_IndexedEvalWithSectionAwareRetrievalDisabledSkipsSections(t *testing.T) {
	root := t.TempDir()
	writeSectionEvalFixture(t, root)

	result, err := Run(root, Options{DisableSectionAwareRetrieval: true})
	require.NoError(t, err)
	assert.Equal(t, "eval_weighted_files_v0_no_section_retrieval", result.Retriever)
	require.Len(t, result.Cases, 1)
	assert.Zero(t, result.Cases[0].SectionSelectedCount)
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
	require.NoError(t, err)
	require.Len(t, result.Cases, 1,
		"cases = %d", len(result.Cases))

	c := result.Cases[0]
	require.Equal(t, 650, c.ContextTokenBudget,
		"expected context budget metadata, got %#v", c)
	require.Greater(t, c.PreBudgetDevSpecsTokens, c.DevSpecsTokens,
		"expected pre-budget tokens to exceed packed tokens, pre=%d post=%d", c.PreBudgetDevSpecsTokens, c.DevSpecsTokens)
	require.LessOrEqual(t, c.DevSpecsTokens, c.ContextTokenBudget,
		"expected context to fit budget, got %d > %d", c.DevSpecsTokens, c.ContextTokenBudget)
	require.NotEmpty(t, c.ContextBudgetDroppedArtifacts,
		"expected dropped artifacts: %#v", c)
	require.False(t, c.ContextSufficiency.Passed,
		"budgeted pack should expose sufficiency tradeoff when required context is dropped")

}

func TestRun_IndexedCacheOnColdRunWritesCacheAndTelemetry(t *testing.T) {
	root := t.TempDir()
	writeTinyEvalFixture(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")

	result, err := Run(root, Options{
		IndexCacheDir:        cacheDir,
		MaxSourceFiles:       1,
		MaxTestCaseArtifacts: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, result.IndexCache)
	assert.True(t, result.IndexCache.Enabled)
	assert.False(t, result.IndexCache.Hit)
	assert.NotEmpty(t, result.IndexCache.CorpusFingerprint)
	assert.NotEmpty(t, result.IndexCache.ProvenanceFingerprint)
	assert.NotEmpty(t, result.PhaseTelemetry)
	assert.True(t, hasPhase(result.PhaseTelemetry, "index_or_load_corpus"))
	assert.True(t, hasPhase(result.PhaseTelemetry, "sqlite_scan"))
	assert.True(t, hasPhase(result.PhaseTelemetry, "sqlite_readback"))
	assert.True(t, hasPhase(result.PhaseTelemetry, "index_cache_write"))
	assert.True(t, hasPhase(result.PhaseTelemetry, "case"))

	cacheData, err := os.ReadFile(filepath.FromSlash(result.IndexCache.Path))
	require.NoError(t, err)
	assert.NotContains(t, string(cacheData), "\n")
}

func TestRun_IndexedCacheOnWarmRunReadsSameCacheKey(t *testing.T) {
	root := t.TempDir()
	writeTinyEvalFixture(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	options := Options{IndexCacheDir: cacheDir, MaxSourceFiles: 1, MaxTestCaseArtifacts: 1}
	key, err := indexedCorpusCacheKey(root, options)
	require.NoError(t, err)
	cachePath := filepath.Join(cacheDir, key+".json")
	require.NoError(t, writeIndexedCorpusCache(cachePath, indexedCorpusCacheFile{
		SchemaVersion: evalIndexCacheSchemaVersion,
		Key:           key,
		CreatedAt:     "2026-08-11T00:00:00Z",
		Root:          filepath.ToSlash(root),
		Files: []File{{
			Path: "docs/plans/alpha.md",
			Body: "# Alpha plan\n\nThis plan covers billing retry behavior.\n",
		}},
	}))

	result, err := Run(root, options)
	require.NoError(t, err)
	require.NotNil(t, result.IndexCache)
	assert.True(t, result.IndexCache.Hit)
	assert.Equal(t, key, result.IndexCache.Key)
	assert.True(t, hasPhase(result.PhaseTelemetry, "index_cache_read"))
}

func TestIndexedCorpusCacheKey_WithTestArtifacts_DiffersFromDefault(t *testing.T) {
	root := t.TempDir()
	writeTinyEvalFixture(t, root)

	withoutTests, err := indexedCorpusCacheKey(root, Options{})
	require.NoError(t, err)
	withTests, err := indexedCorpusCacheKey(root, Options{TestCaseArtifacts: true})
	require.NoError(t, err)

	assert.NotEqual(t, withoutTests, withTests)
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
	}, newPhaseRecorder())
	require.NoError(t, err)

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
	require.LessOrEqual(t, sourceFiles, 1,
		"source files should be capped before eval corpus use, got %d in %#v", sourceFiles, files)
	require.LessOrEqual(t, testCases, 1,
		"test cases should be capped before eval corpus use, got %d in %#v", testCases, files)
	require.LessOrEqual(t, codeComments, 1,
		"code comments should be capped before eval corpus use, got %d in %#v", codeComments, files)

}

func TestIndexedCorpusFingerprintIgnoresRetrievalOnlySource(t *testing.T) {
	root := t.TempDir()
	writeFingerprintSource(t, root, filepath.Join("internal", "adapters", "adapter.go"), "package adapters\nconst AdapterVersion = 1\n")
	writeFingerprintSource(t, root, filepath.Join("internal", "retrieval", "retrieval.go"), "package retrieval\nconst RetrievalVersion = 1\n")

	initial, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	require.NoError(t, err)

	writeFingerprintSource(t, root, filepath.Join("internal", "retrieval", "retrieval.go"), "package retrieval\nconst RetrievalVersion = 2\n")
	retrievalOnly, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	require.NoError(t, err)

	assert.Equal(t, initial, retrievalOnly,
		"retrieval-only change should not affect indexed corpus fingerprint: initial=%s retrieval=%s", initial, retrievalOnly)
}

func TestIndexedCorpusFingerprintChangesForAdapterSource(t *testing.T) {
	root := t.TempDir()
	writeFingerprintSource(t, root, filepath.Join("internal", "adapters", "adapter.go"), "package adapters\nconst AdapterVersion = 1\n")
	writeFingerprintSource(t, root, filepath.Join("internal", "retrieval", "retrieval.go"), "package retrieval\nconst RetrievalVersion = 1\n")
	initial, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	require.NoError(t, err)

	writeFingerprintSource(t, root, filepath.Join("internal", "adapters", "adapter.go"), "package adapters\nconst AdapterVersion = 2\n")
	changed, err := sourceTreeDigest(root, indexedCorpusFingerprintPaths())
	require.NoError(t, err)

	assert.NotEqual(t, initial, changed,
		"adapter change should affect indexed corpus fingerprint: %s", changed)

}

func writeFingerprintSource(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}

func TestRun_WithRecallThresholdAboveOneReportsFailure(t *testing.T) {
	minRecall := 1.01
	result, err := Run(fixturePath(t), Options{MinRecall: &minRecall, CorpusSource: CorpusSourceFilesystemFixture})
	require.NoError(t, err)
	assert.Positive(t, result.Summary.FailedThresholdCount)
}

func TestRun_WithMeanRecallThresholdAboveOneReportsFailure(t *testing.T) {
	minMeanRecall := 1.01
	result, err := Run(fixturePath(t), Options{MinMeanRecall: &minMeanRecall, CorpusSource: CorpusSourceFilesystemFixture})
	require.NoError(t, err)
	assert.Positive(t, result.Summary.FailedThresholdCount)
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
	{
		err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(root, "docs", "plans", "alpha.md"), []byte("# Alpha plan\n\nThis plan covers billing retry behavior.\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll(filepath.Join(root, "src"), 0o755)
		require.NoError(t, err)
	}

	source := "// TODO because retry behavior must stay compatible with legacy billing callers.\nexport function retryBilling() { return true }\n"
	{
		err := os.WriteFile(filepath.Join(root, "src", "billing.ts"), []byte(source), 0o644)
		require.NoError(t, err)
	}

	testSource := "describe(\"billing retries\", () => {\n  it(\"rejects duplicate replay\", () => {\n    expect(retryBilling()).toBe(true)\n  })\n  it(\"keeps legacy compatibility\", () => {\n    expect(retryBilling()).toBe(true)\n  })\n})\n"
	{
		err := os.WriteFile(filepath.Join(root, "src", "billing.test.ts"), []byte(testSource), 0o644)
		require.NoError(t, err)
	}

	cases := "fixture_version: tiny-v0\n" +
		"eval_stage: tiny_eval\n\n" +
		"cases:\n" +
		"  - id: alpha\n" +
		"    query: \"billing retry behavior\"\n" +
		"    expected_relevant:\n" +
		"      - path: docs/plans/alpha.md\n" +
		"        importance: must\n"
	{
		err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644)
		require.NoError(t, err)
	}

}

func writeSectionEvalFixture(t *testing.T, root string) {
	t.Helper()
	{
		err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755)
		require.NoError(t, err)
	}

	doc := "# Broad Plan\n\n" +
		strings.Repeat("General engineering notes without the target terms.\n", 80) +
		"\n## Replay Boundary\n\n" +
		"stripe_event_id idempotency protects webhook replay behavior.\n"
	{
		err := os.WriteFile(filepath.Join(root, "docs", "plans", "broad.md"), []byte(doc), 0o644)
		require.NoError(t, err)
	}

	cases := "fixture_version: section-v0\n" +
		"eval_stage: section_eval\n\n" +
		"cases:\n" +
		"  - id: section-replay\n" +
		"    query: \"stripe_event_id idempotency\"\n" +
		"    expected_relevant:\n" +
		"      - path: docs/plans/broad.md\n" +
		"        importance: must\n"
	{
		err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644)
		require.NoError(t, err)
	}

}

func writeBudgetedPackingFixture(t *testing.T, root string) {
	t.Helper()
	{
		err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755)
		require.NoError(t, err)
	}

	one := "# Billing Retry Plan\n\n" +
		"billing retry behavior stripe_event_id idempotency primary context.\n\n" +
		strings.Repeat("primary filler context keeps this artifact moderate.\n", 20)
	two := "# Billing Retry Appendix\n\n" +
		"billing retry behavior webhook replay secondary context.\n\n" +
		strings.Repeat("secondary filler context makes this artifact droppable.\n", 50)
	{
		err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing-retry.md"), []byte(one), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing-retry-appendix.md"), []byte(two), 0o644)
		require.NoError(t, err)
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
	{
		err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644)
		require.NoError(t, err)
	}

}
