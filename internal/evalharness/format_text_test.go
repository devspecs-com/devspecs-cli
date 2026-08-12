package evalharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/openspecmetrics"
	"github.com/stretchr/testify/assert"
)

func TestFormatJSON_WithResult_ReturnsIndentedMachineReadableReport(t *testing.T) {
	report := &Result{Fixture: "agentic-saas", Summary: Summary{Cases: 2}}

	data, err := FormatJSON(report)

	assert.NoError(t, err)
	assert.Contains(t, string(data), "\n  \"fixture\": \"agentic-saas\"")
	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "agentic-saas", decoded["fixture"])
}

func TestFormatText_WithCompleteReport_RendersOperationalDiagnostics(t *testing.T) {
	report := &Result{
		Fixture:          "agentic-saas",
		FixtureVersion:   "v2",
		EvalStage:        "release",
		CorpusSource:     "sqlite_index",
		ProductPath:      "ds find",
		CommandUnderTest: "ds find billing",
		FindRuntime:      "active",
		Retriever:        "weighted_files_v0",
		TokenCounter:     "tiktoken",
		TokenizerProfile: TokenizerProfile{Approximation: "cl100k compatible"},
		PricingProfile:   PricingProfile{Name: "standard"},
		ResultsFile:      "results/eval.json",
		IndexCache:       &IndexCacheReport{Enabled: true, Hit: true, Key: "1234567890abcdef"},
		Budgets: BudgetReport{Applied: []BudgetEvent{{
			Name: "source files", Before: 1200, After: 500,
		}}},
		Corpus: CorpusSummary{
			PlanningArtifacts:       CorpusSlice{Files: 10, Tokens: 12000},
			MarkdownFiles:           CorpusSlice{Files: 20, Tokens: 24000},
			SourceContextCandidates: CorpusSlice{Files: 30, Tokens: 36000},
			FullCandidateCorpus:     CorpusSlice{Files: 60, Tokens: 72000},
		},
		Summary: Summary{
			Cases:                                    1,
			MedianTokenReductionVsFullPlanning:       0.75,
			MeanTokenReductionVsFullPlanning:         0.70,
			MedianTokenReductionVsQueryFileBaseline:  0.50,
			MeanTokenReductionVsQueryFileBaseline:    0.45,
			MeanArtifactRecall:                       0.90,
			MeanMustHaveRecall:                       1,
			MeanHelpfulRecall:                        0.80,
			MeanBackgroundRecall:                     0.60,
			MeanArtifactPrecision:                    0.75,
			MeanGradedPrecision:                      0.85,
			MeanPenalizedUtilityPrecision:            0.72,
			RelatedArtifactCount:                     2,
			MeanRelatedArtifactPrecision:             0.50,
			MeanRelatedGradedPrecision:               0.75,
			GraphContextArtifactCount:                2,
			GraphAssistedRelevantCount:               1,
			MeanGraphContextArtifactPrecision:        0.50,
			MeanGraphContextGradedPrecision:          0.75,
			PackDiagnosticCases:                      1,
			MeanPackIncludedArtifacts:                4,
			MeanPackRoleDiversity:                    3,
			PackExcludedNoiseCount:                   2,
			ContextSufficiencyCases:                  1,
			ContextSufficiencyPassed:                 1,
			ContextSufficiencyPassRate:               1,
			CombinedTieredContextSufficiencyCases:    1,
			CombinedTieredContextSufficiencyPassed:   1,
			CombinedTieredContextSufficiencyPassRate: 1,
			FailedThresholdCount:                     1,
			Pareto: ParetoSummary{
				MeanTokenReductionVsFullPlanning: 0.70,
				MeanArtifactRecall:               0.90,
				MeanMustHaveRecall:               1,
				MeanArtifactPrecision:            0.75,
				MeanGradedPrecision:              0.85,
				ContextSufficiencyPassRate:       1,
			},
		},
		AgentMetrics: AgentMetrics{
			MustHitAt3:        1,
			MeanFirstMustRank: 1,
			ContextSufficiencyAtTokenBudget: []TokenBudgetSufficiency{{
				BudgetTokens: 2048, PassedCases: 1, EligibleCases: 1, PassRate: 1,
			}},
		},
		LaneMetrics: []LaneMetric{{
			Lane: "primary", StrictPrecision: 0.75, GradedPrecision: 0.85, Recall: 0.90,
			IncludedArtifacts: 4, ExactRelevantArtifacts: 3, SameClusterArtifacts: 1,
			HardNegativeArtifacts: 1, PackedSectionCount: 2,
		}},
		CanonicalLanes: []LaneMetric{{
			Lane: "intent", StrictPrecision: 1, GradedPrecision: 1, Recall: 1,
			IncludedArtifacts: 1, ExactRelevantArtifacts: 1, SameClusterArtifacts: 1,
			HardNegativeArtifacts: 1,
		}},
		Diagnostics: Diagnostics{
			ExpectedRelevantCount:         3,
			ExpectedAvailableCount:        2,
			MissedAfterDiscoveryCount:     1,
			DiscoveryCoverage:             0.67,
			RetrievalCoverageOfDiscovered: 0.50,
			ExpectedMissingFromCorpus:     []string{"docs/missing.md"},
			MissedAfterDiscovery:          []string{"docs/missed.md"},
			RoleSummaries: []RoleDiagnostic{{
				Role: "decision", Expected: 2, ExpectedAvailable: 1, Retrieved: 1,
				MissingFromCorpus: 1, MissedAfterDiscovery: 0, IrrelevantRetrieved: 1,
			}},
			MissClassSummaries: []MissClassDiagnostic{{
				Class: "not_indexed", Count: 1, Examples: []string{"docs/missing.md"},
			}},
			FalsePositiveSummaries: []FalsePositiveDiagnostic{{
				Class: "support_noise", Count: 1,
				GradeCounts: GradeCounts{SameCluster: 1, Unlabeled: 1, HardNegative: 1},
				Examples:    []FalsePositiveExample{{CaseID: "billing", Path: "README.md"}},
			}},
			ExtensionSummaries: []ExtensionDiagnostic{{
				Extension: ".md", Role: "decision", Expected: 2, ExactRetrieved: 1,
				MissingFromCorpus: 1, MissedAfterDiscovery: 0, PrimaryFalsePositive: 1,
			}},
			UnindexedDocumentSummaries: []UnindexedDocumentDiagnostic{{
				Extension: ".rst", Role: "support", Count: 2,
				Examples: []string{"docs/one.rst", "docs/two.rst", "docs/three.rst", "docs/four.rst"},
			}},
			OpenSpec: &openspecmetrics.Metrics{
				ExpectedBundles: 2, BundleRecall: 0.5, MissingBundles: []string{"old-change"},
				ExpectedChildArtifacts: 4, ChildRoleRecall: 0.75, MissingChildRoles: []string{"design"},
				DuplicatePressure: 2.5, MarkdownLeakage: 1,
			},
		},
		Cases: []CaseResult{{
			ID: "billing", DevSpecsTokens: 1200, PreBudgetDevSpecsTokens: 1800,
			ContextTokenBudget: 1500, ContextBudgetDroppedCount: 1,
			ContextBudgetDroppedArtifacts: []string{"docs/noise.md"},
			FullPlanningTokens:            5000, AllMarkdownTokens: 7000, FullCandidateCorpusTokens: 12000,
			QueryFileBaselineTokens: 2500, TokenReductionVsFullPlanning: 0.76,
			TokenReductionVsAllMarkdown: 0.83, TokenReductionVsFullCandidate: 0.90,
			TokenReductionVsQueryFile: 0.52, RelevantRetrieved: 2, ExpectedRelevantCount: 3,
			ArtifactRecall: 0.67, MustRelevantRetrieved: 1, MustExpectedCount: 1, MustHaveRecall: 1,
			HelpfulRelevantRetrieved: 1, HelpfulExpectedCount: 1, HelpfulRecall: 1,
			BackgroundRelevantRetrieved: 0, BackgroundExpectedCount: 1, BackgroundRecall: 0,
			ArtifactsIncluded: []string{"decision.md", "service.go", "README.md"},
			RelevantIncluded:  []string{"decision.md", "service.go"}, IrrelevantIncluded: []string{"README.md"},
			ArtifactPrecision: 0.67, RelatedArtifacts: []string{"runbook.md"},
			RelatedRelevantIncluded: []string{"runbook.md"}, RelatedArtifactPrecision: 1,
			CombinedTieredContextSufficiency: SufficiencyResult{Configured: true, Passed: true},
			GraphContextArtifacts:            []string{"schema.md"}, GraphContextRelevantIncluded: []string{"schema.md"},
			GraphAssistedRelevantIncluded: []string{"schema.md"}, GraphContextArtifactPrecision: 1,
			AgentMetrics:       CaseAgentMetrics{GradedPrecision: 0.8, FirstMustRank: 1},
			ContextSufficiency: SufficiencyResult{Configured: true, Passed: false, Failures: []string{"missing retry policy"}},
			PrimaryFalsePositiveDiagnostics: []FalsePositiveExample{{
				Position: 3, Path: "README.md", Lane: "primary", Role: "support", Grade: "hard_negative",
			}},
			MissedExpectedRelevant: []string{"retry.md"}, ExpectedAvailableCount: 2,
			DiscoveryCoverage: 0.67, RetrievalCoverageOfDiscovered: 0.50,
			ExpectedMissingFromCorpus: []string{"missing.md"}, MissedAfterDiscovery: []string{"retry.md"},
			UnexpectedExcludedHits: []string{"generated.md"}, ThresholdFailures: []string{"recall below floor"},
		}},
	}

	output := FormatText(report)

	assert.True(t, strings.HasPrefix(output, "DevSpecs Eval: agentic-saas"))
	assert.Contains(t, output, "Index cache: hit=true key=1234567890ab")
	assert.Contains(t, output, "Graph context: 2 artifacts / 1 graph-assisted relevant")
	assert.Contains(t, output, "OpenSpec:")
	assert.Contains(t, output, "Case: billing")
	assert.Contains(t, output, "Sufficiency failures: missing retry policy")
	assert.Contains(t, output, "Threshold failures: recall below floor")
}
