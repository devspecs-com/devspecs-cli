package retrieval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeightedFilesRetrieverV0_RetrievesAndExplainsCandidates(t *testing.T) {
	candidates := []Candidate{
		{
			Path: "openspec/changes/harden-entitlement-sync/design.md",
			Body: "Design for entitlement_sync and stripe_event_id idempotency.",
		},
		{
			Path: "scratch/old-billing-plan.md",
			Body: "Old billing scratch notes with customer portal tasks.",
		},
	}

	retriever := WeightedFilesRetrieverV0{}
	got := retriever.Retrieve(candidates, "stripe_event_id idempotency")
	name := retriever.Name()
	reasons := ExplainCandidates(got, "stripe_event_id idempotency")

	assert.Equal(t, "eval_weighted_files_v0", name,
		"retriever name = %q", name)
	require.Len(t, got, 1,
		"retrieved %d candidates, want 1: %#v", len(got), got)
	assert.Equal(t, "openspec/changes/harden-entitlement-sync/design.md", got[0].Path,
		"retrieved path = %q", got[0].Path)
	require.Len(t, reasons, 1)
	assert.Equal(t, got[0].Path, reasons[0].Path)
	assert.NotEmpty(t, reasons[0].Reasons)

}

func TestExplainCandidatesOrdersTermReasonsByEvidenceStrength(t *testing.T) {
	candidates := []Candidate{{
		Path:  "tests/e2e/tests/auth/mcp-oauth-settings-gate.sb.test.ts",
		Title: "settings gate",
		Body:  "fix dcr metadata handling",
	}}

	reasons := ExplainCandidates(candidates, "Fix MCP OAuth DCR metadata handling")
	require.Len(t, reasons, 1,
		"reasons = %#v", reasons)

	got := reasons[0].Reasons
	wantPrefix := []string{
		"query term match in path: mcp",
		"query term match in path: oauth",
		"query term match in body: dcr",
	}
	require.GreaterOrEqual(t, len(got), len(wantPrefix),
		"too few reasons: %#v", got)

	for i, want := range wantPrefix {
		require.Equal(t, want, got[i],
			"reason[%d] = %q, want %q; all=%#v", i, got[i], want, got)

	}
}

func TestArtifactRole_WithSkillSubtype_PrefersSubtypeOverClassifierFamily(t *testing.T) {
	candidate := Candidate{
		Path:    ".codex/skills/review/SKILL.md",
		Kind:    "markdown_artifact",
		Subtype: "skill",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
			"classifier_family":  "protocol.agent_instruction",
		},
	}

	actual := artifactRole(candidate)

	assert.Equal(t, "skill", actual)
}

func TestCandidateRole_WithSkillSubtype_PrefersSubtypeOverClassifierFamily(t *testing.T) {
	candidate := Candidate{
		Path:    ".codex/skills/review/SKILL.md",
		Kind:    "markdown_artifact",
		Subtype: "skill",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
			"classifier_family":  "protocol.agent_instruction",
		},
	}

	actual := candidateRole(candidate)

	assert.Equal(t, "skill", actual)
}

func TestArtifactRole_WithTemplateSubtype_PrefersSubtypeOverClassifierModel(t *testing.T) {
	candidate := Candidate{
		Path:    ".github/PULL_REQUEST_TEMPLATE.md",
		Kind:    "markdown_artifact",
		Subtype: "pull_request_template",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
		},
	}

	actual := artifactRole(candidate)

	assert.Equal(t, "template", actual)
}

func TestCandidateRole_WithTemplateSubtype_PrefersSubtypeOverClassifierModel(t *testing.T) {
	candidate := Candidate{
		Path:    ".github/PULL_REQUEST_TEMPLATE.md",
		Kind:    "markdown_artifact",
		Subtype: "pull_request_template",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
		},
	}

	actual := candidateRole(candidate)

	assert.Equal(t, "template", actual)
}

func TestArtifactRole_WithoutDetectedSubtype_UsesClassifierMetadata(t *testing.T) {
	candidate := Candidate{
		Path: "docs/process/repo-rules.md",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
		},
	}

	actual := artifactRole(candidate)

	assert.Equal(t, "agent_instruction", actual)
}

func TestCandidateRole_WithoutDetectedSubtype_UsesClassifierMetadata(t *testing.T) {
	candidate := Candidate{
		Path: "docs/process/repo-rules.md",
		Metadata: map[string]string{
			"classifier_model":   "protocol",
			"classifier_subtype": "agent_instruction",
		},
	}

	actual := candidateRole(candidate)

	assert.Equal(t, "agent_instruction", actual)
}

func TestArtifactRole_WithOpenSpecDesignPath_RefinesBroadSubtype(t *testing.T) {
	candidate := Candidate{
		Path:    "openspec/changes/harden-entitlement-sync/design.md",
		Kind:    "spec",
		Subtype: "openspec_child",
	}

	actual := artifactRole(candidate)

	assert.Equal(t, "openspec_design", actual)
}

func TestCandidateRole_WithOpenSpecDesignPath_RefinesBroadSubtype(t *testing.T) {
	candidate := Candidate{
		Path:    "openspec/changes/harden-entitlement-sync/design.md",
		Kind:    "spec",
		Subtype: "openspec_child",
	}

	actual := candidateRole(candidate)

	assert.Equal(t, "openspec_design", actual)
}

func TestNonIntentCandidateMode_WithTemplateSubtype_PrefersSubtype(t *testing.T) {
	candidate := Candidate{Subtype: "pull_request_template", Metadata: map[string]string{"classifier_model": "protocol"}}

	actual := nonIntentCandidateMode(candidate)

	assert.Equal(t, "template", actual)
}

func TestNonIntentCandidateMode_WithSkillSubtype_PrefersSubtype(t *testing.T) {
	candidate := Candidate{Subtype: "skill", Metadata: map[string]string{"classifier_model": "template"}}

	actual := nonIntentCandidateMode(candidate)

	assert.Equal(t, "protocol", actual)
}

func TestNonIntentCandidateMode_WithoutSubtype_UsesClassifierModel(t *testing.T) {
	candidate := Candidate{Metadata: map[string]string{"classifier_model": "protocol"}}

	actual := nonIntentCandidateMode(candidate)

	assert.Equal(t, "protocol", actual)
}

func TestNonIntentCandidateMode_WithSkillPath_UsesPathFallback(t *testing.T) {
	candidate := Candidate{Path: ".codex/skills/review/SKILL.md"}

	actual := nonIntentCandidateMode(candidate)

	assert.Equal(t, "protocol", actual)
}

func TestWeightedFilesRetrieverV0_UsesIndexedSectionEvidence(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/plans/broad.md",
			Title: "Broad Plan",
			Kind:  "plan",
			Body:  "# Broad Plan\n\n" + strings.Repeat("general implementation background.\n", 140) + "\nstripe_event_id idempotency protects webhook replay behavior.\n",
			Metadata: map[string]string{
				"indexed_section_retrieval_mode":      "section_aware",
				"indexed_section_match_count":         "1",
				"indexed_section_match_headings_json": mustJSONList(t, []string{"Requirements > Replay Boundary"}),
				"indexed_section_match_ranges_json":   mustJSONList(t, []string{"22-40"}),
				"indexed_section_match_bodies_json":   mustJSONList(t, []string{"stripe_event_id idempotency protects webhook replay behavior."}),
				"indexed_section_match_ids_json":      mustJSONList(t, []string{"sec_test"}),
				"indexed_section_total":               "5",
			},
		},
		{Path: "docs/plans/unrelated.md", Body: "general implementation background"},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "stripe_event_id idempotency")
	reasons := ExplainCandidates(got, "stripe_event_id idempotency")

	assert.True(t, containsCandidatePath(got, "docs/plans/broad.md"),
		"missing section-selected artifact: %#v", CandidatePaths(got))
	require.NotEmpty(t, got)
	assert.Equal(t, "section_aware", got[0].Metadata["indexed_section_retrieval_mode"],
		"expected indexed section match metadata, got %#v", got[0].Metadata)
	require.NotEmpty(t, reasons)
	assert.Contains(t, strings.Join(reasons[0].Reasons, "\n"), "indexed section match")

}

func TestWeightedFilesRetrieverV0_BalancedEvidenceOrdersAnchoredCandidate(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/notes/broad-sync-notes.md",
			Title: "Broad Notes",
			Body:  strings.Repeat("entitlement sync rollout background ", 12),
		},
		{
			Path:  "docs/plans/entitlement-sync-rollout.md",
			Title: "Entitlement Sync Rollout Plan",
			Kind:  "plan",
			Body:  "Plan for the entitlement sync rollout.",
		},
	}

	retriever := WeightedFilesRetrieverV0{EvidenceMode: EvidenceModeBalanced}
	got := retriever.Retrieve(candidates, "resume entitlement sync rollout plan")
	require.Equal(t, "eval_weighted_files_v0_evidence_balanced", retriever.Name(),
		"retriever name = %q", retriever.Name())
	require.NotEmpty(t, got)
	assert.Equal(t, "docs/plans/entitlement-sync-rollout.md", got[0].Path)
	require.Equal(t, EvidenceModeBalanced, got[0].Metadata["retrieval_evidence_mode"],
		"missing balanced evidence metadata: %#v", got[0].Metadata)

}

func TestWeightedFilesRetrieverV0_WithAttachedSectionsUsesSectionsForPacking(t *testing.T) {
	candidates := attachedSectionCandidates()

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "stripe_event_id idempotency")
	require.True(t, containsCandidatePath(got, "docs/plans/broad.md"),
		"missing section-selected artifact: %#v", CandidatePaths(got))
	require.Equal(t, "candidate_sections", got[0].Metadata["indexed_section_match_source"],
		"expected attached section metadata, got %#v", got[0].Metadata)
}

func TestWeightedFilesRetrieverV0_WithSectionAwareDisabledDoesNotAnnotateSectionEvidence(t *testing.T) {
	candidates := attachedSectionCandidates()

	got := (WeightedFilesRetrieverV0{DisableSectionAware: true}).Retrieve(candidates, "stripe_event_id idempotency")

	require.True(t, containsCandidatePath(got, "docs/plans/broad.md"))
	if got[0].Metadata != nil {
		assert.NotEqual(t, "section_aware", got[0].Metadata["indexed_section_retrieval_mode"])
	}
}

func TestEnrichCandidatesWithSectionMatchesRejectsGenericBodyOnlyMatches(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "generic",
			Path:  "docs/plans/generic.md",
			Title: "Generic Plan",
			Kind:  "plan",
			Body:  "# Generic Plan\n\nGeneral notes.",
			Sections: []IndexedSection{
				{
					ID:          "sec_generic",
					ArtifactID:  "generic",
					SourcePath:  "docs/plans/generic.md",
					HeadingPath: "Overview",
					Title:       "Overview",
					StartLine:   1,
					EndLine:     5,
					Body:        "This implementation plan document gives general context and background.",
				},
			},
		},
	}

	got := EnrichCandidatesWithSectionMatches(candidates, "implementation plan context")
	require.Len(t, got, 1)
	if got[0].Metadata != nil {
		assert.NotEqual(t, "section_aware", got[0].Metadata["indexed_section_retrieval_mode"])
	}

}

func TestEnrichCandidatesWithSectionMatchesWithoutRoadmapIntentDoesNotRescueRoadmap(t *testing.T) {
	candidates := roadmapSectionCandidates()

	got := EnrichCandidatesWithSectionMatches(candidates, "repository agent operating instructions and contributor guidance for AReaL")
	require.Len(t, got, 1)
	if got[0].Metadata != nil {
		assert.NotEqual(t, "section_aware", got[0].Metadata["indexed_section_retrieval_mode"])
	}
}

func TestEnrichCandidatesWithSectionMatchesWithRoadmapIntentRescuesRoadmap(t *testing.T) {
	candidates := roadmapSectionCandidates()

	got := EnrichCandidatesWithSectionMatches(candidates, "AReaL roadmap future work and milestones")

	require.Len(t, got, 1)
	require.NotNil(t, got[0].Metadata)
	assert.Equal(t, "section_aware", got[0].Metadata["indexed_section_retrieval_mode"])
}

func attachedSectionCandidates() []Candidate {
	return []Candidate{
		{
			ID:    "plan_broad",
			Path:  "docs/plans/broad.md",
			Title: "Broad Plan",
			Kind:  "plan",
			Body:  "# Broad Plan\n\n" + strings.Repeat("general implementation background.\n", 140) + "\nstripe_event_id idempotency protects webhook replay behavior.\n",
			Sections: []IndexedSection{{
				ID:           "sec_replay",
				ArtifactID:   "plan_broad",
				SourcePath:   "docs/plans/broad.md",
				HeadingPath:  "Requirements > Replay Boundary",
				Title:        "Replay Boundary",
				StartLine:    22,
				EndLine:      40,
				Body:         "stripe_event_id idempotency protects webhook replay behavior.",
				HeadingDepth: 2,
			}},
		},
		{ID: "unrelated", Path: "docs/plans/unrelated.md", Body: "general implementation background"},
	}
}

func roadmapSectionCandidates() []Candidate {
	return []Candidate{{
		ID:    "roadmap",
		Path:  "ROADMAP.md",
		Title: "AReaL Roadmap",
		Kind:  "plan",
		Body:  "# AReaL Roadmap\n\nGeneral future planning.",
		Sections: []IndexedSection{{
			ID:          "sec_roadmap",
			ArtifactID:  "roadmap",
			SourcePath:  "ROADMAP.md",
			HeadingPath: "AReaL Roadmap",
			Title:       "AReaL Roadmap",
			StartLine:   1,
			EndLine:     9,
			Body:        "AReaL project timeline and future planning notes.",
		}},
	}}
}

func mustJSONList(t *testing.T, values []string) string {
	t.Helper()
	b, err := json.Marshal(values)
	require.NoError(t, err)

	return string(b)
}

func TestQueryBaselineMatchesPathOrBody(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/plans/2026-05-01-entitlement-sync-plan.md", Body: "Implementation notes."},
		{Path: "docs/adr/0002-webhook-idempotency-boundary.md", Body: "stripe_event_id is the replay boundary."},
		{Path: "docs/prd/customer-portal-v2.md", Body: "Portal background."},
	}

	got := QueryBaseline(candidates, "stripe_event_id idempotency")
	paths := CandidatePaths(got)
	require.Len(t, paths, 1)
	assert.Equal(t, "docs/adr/0002-webhook-idempotency-boundary.md", paths[0])

}

func TestWeightedFilesRetrieverV0_UsesCandidateTitle(t *testing.T) {
	candidates := []Candidate{
		{Path: "plan.md", Title: "Golden Plan", Body: "Short body."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "Golden")
	reasons := ExplainCandidates(got, "Golden")

	require.Len(t, got, 1,
		"retrieved %d candidates, want 1", len(got))
	require.NotEmpty(t, reasons)
	require.NotEmpty(t, reasons[0].Reasons)
	assert.Equal(t, "query term match in title: golden", reasons[0].Reasons[0])

}

func TestWeightedFilesRetrieverV0_SourceIntentPrefersExactSourceFiles(t *testing.T) {
	candidates := []Candidate{
		{Path: "services/api/src/auth/session.ts", Body: "type Session = { customer_id?: string; authorization_details?: unknown }"},
		{Path: "services/api/src/billing/entitlements.ts", Body: "const authorization_details = await loadAuthorizationDetails(customer_id)"},
		{Path: "docs/prd/billing-entitlements-v1.md", Body: "Requirements mention `authorization_details` and `customer_id` for access checks."},
		{Path: "docs/adr/0005-auth-session-cookie-boundary.md", Body: "Decision: session cookies own customer_id lookup boundaries."},
		{Path: "openspec/changes/refactor-auth-session/design.md", Body: "Design: load authorization_details from the server session before token handoff."},
		{Path: "docs/plans/billing-ops-runbook.md", Body: "Known false positive: customer_id authorization_details source file billing support replay customer customer customer."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "authorization_details customer_id source file")
	require.True(t, containsCandidatePath(got, "services/api/src/auth/session.ts"),
		"missing session source file: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "services/api/src/billing/entitlements.ts"),
		"missing entitlements source file: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "docs/plans/billing-ops-runbook.md"),
		"broad runbook should not outrank exact source matches: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_RFCIntentUsesRFCAndCoreTerms(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/rfcs/0008-billing-webhook-replay-protection.md", Body: "Summary Motivation Proposal Drawbacks Alternatives stripe_event_id webhook_replay_protection replay ledger."},
		{Path: "docs/rfcs/0009-support-search-ranking.md", Body: "Summary Motivation Proposal Drawbacks Alternatives support search customer portal."},
		{Path: "openspec/changes/harden-entitlement-sync/design.md", Body: "webhook replay protection uses stripe_event_id."},
		{Path: "docs/adr/0002-webhook-idempotency-boundary.md", Body: "Decision for webhook idempotency boundary."},
		{Path: "docs/plans/2026-04-billing-ops-runbook.md", Body: "support replay webhook customer portal runbook alternatives."},
		{Path: "scratch/old-webhook-retry-investigation.md", Body: "old retry notes for replay."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "RFC for webhook replay protection alternatives")
	require.True(t, containsCandidatePath(got, "docs/rfcs/0008-billing-webhook-replay-protection.md"),
		"missing RFC candidate: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "docs/rfcs/0009-support-search-ranking.md"),
		"unrelated RFC should not be selected: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_GenericPlanNeedsCoreEvidence(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/plans/2026-05-01-entitlement-sync-plan.md", Body: "Current progress for entitlement_sync hardening and billing-webhook-hardening."},
		{Path: "docs/plans/generic-implementation-plan.md", Body: "Current progress next steps implementation notes without the requested feature words."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume entitlement sync hardening")
	require.True(t, containsCandidatePath(got, "docs/plans/2026-05-01-entitlement-sync-plan.md"),
		"missing specific plan: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "docs/plans/generic-implementation-plan.md"),
		"generic plan should not pass without core evidence: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_DemotesNonIntentLanesForOrdinaryPlanQuery(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/plans/auth-token-rollout.md",
			Body:  "Current progress for auth token rollout and migration tasks.",
			Title: "Auth Token Rollout Plan",
		},
		{
			Path:     "CLAUDE.md",
			Title:    "Claude Instructions",
			Subtype:  "agent_instruction",
			Body:     "Auth token rollout rules and instructions for contributors.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume auth token rollout")

	assert.True(t, containsCandidatePath(got, "docs/plans/auth-token-rollout.md"),
		"missing plan: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "CLAUDE.md"),
		"protocol instructions should not appear in ordinary plan retrieval: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_IncludesNonIntentLaneWhenExplicitlyRequested(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/plans/auth-token-rollout.md",
			Body:  "Current progress for auth token rollout and migration tasks.",
			Title: "Auth Token Rollout Plan",
		},
		{
			Path:     "CLAUDE.md",
			Title:    "Claude Instructions",
			Subtype:  "agent_instruction",
			Body:     "Auth token rollout rules and instructions for contributors.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "claude instructions auth token rollout")

	assert.True(t, containsCandidatePath(got, "CLAUDE.md"),
		"missing explicitly requested instructions: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_UsesTestCasesForBehaviorQueries(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "services/billing/webhook_test.go#L12",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestWebhookReplayProtection",
			Body:    "Test: TestWebhookReplayProtection\nSource: services/billing/webhook_test.go\nSymbols: stripe_event_id, idempotency, webhook\nAssertion vocabulary: require, error\n",
			Metadata: map[string]string{
				"source_type":       "test_case",
				"source_line_range": "12-24",
			},
		},
		{
			Path:  "docs/plans/billing-hardening.md",
			Title: "Billing Hardening Plan",
			Body:  "Plan for customer portal billing tasks unrelated to replay tests.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what regression tests protect stripe_event_id idempotency?")
	reasons := ExplainCandidates(got, "what regression tests protect stripe_event_id idempotency?")

	assert.True(t, containsCandidatePath(got, "services/billing/webhook_test.go#L12"),
		"missing test-case candidate: %#v", CandidatePaths(got))
	reasonIndex := reasonPathIndex(reasons, "services/billing/webhook_test.go#L12")
	require.GreaterOrEqual(t, reasonIndex, 0,
		"missing reasons for test candidate: %#v", reasons)
	assert.True(t, reasonContains(reasons[reasonIndex].Reasons, "test-case behavior signal"),
		"missing test behavior reason: %#v", reasons[reasonIndex].Reasons)

}

func TestWeightedFilesRetrieverV0_UsesTestsForImplementationTaskQueries(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "internal/costs/parser_minimax.go",
			Kind:  "source_context",
			Title: "internal/costs/parser_minimax.go (go)",
			Body:  "package costs\n\nfunc ParseMiniMaxUsage() {}\n",
		},
		{
			Path:    "internal/costs/parser_minimax_integration_test.go#L12",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestParseMiniMaxUsagePricing",
			Body:    "Test: TestParseMiniMaxUsagePricing\nSource: internal/costs/parser_minimax_integration_test.go\nSymbols: minimax, usage, pricing\nAssertion vocabulary: require\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "TestParseMiniMaxUsagePricing",
			},
		},
		{
			Path:  "docs/plans/minimax-cost-plan.md",
			Kind:  "plan",
			Title: "MiniMax Cost Plan",
			Body:  strings.Repeat("MiniMax cost collection planning notes. ", 8),
		},
	}

	query := "add Agent Deck cost collection support for MiniMax usage strings and MiniMax model pricing"
	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)
	require.True(t, containsCandidatePath(got, "internal/costs/parser_minimax.go"),
		"missing implementation source candidate: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "internal/costs/parser_minimax_integration_test.go#L12"),
		"implementation task should admit directly matching behavior test: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_AnchorsCamelCaseTestNames(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "internal/tools/tool_test.go#L42",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestPutAndGetExposedTool",
			Body:    "Test: TestPutAndGetExposedTool\nSource: internal/tools/tool_test.go\nAssertion vocabulary: require equal\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "TestPutAndGetExposedTool",
			},
		},
		{
			Path:    "internal/tools/tool_test.go#L90",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestDeleteHiddenTool",
			Body:    "Test: TestDeleteHiddenTool\nSource: internal/tools/tool_test.go\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "TestDeleteHiddenTool",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what tests cover TestPutAndGetExposedTool behavior?")
	reasons := ExplainCandidates(got, "what tests cover TestPutAndGetExposedTool behavior?")

	require.NotEmpty(t, got, "expected exact test-name anchor, got %#v", CandidatePaths(got))
	assert.Equal(t, "internal/tools/tool_test.go#L42", got[0].Path,
		"expected exact test-name anchor first, got %#v", CandidatePaths(got))

	require.NotEmpty(t, reasons, "missing exact test-name reasons")
	assert.True(t, reasonContains(reasons[0].Reasons, "exact test-name anchor"),
		"missing exact test-name reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_AnchorsNaturalLanguageTestNameParts(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "internal/tools/tool_test.go#L42",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestPutAndGetExposedTool",
			Body:    "Test: TestPutAndGetExposedTool\nSource: internal/tools/tool_test.go\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "TestPutAndGetExposedTool",
			},
		},
		{
			Path:    "internal/tools/tool_test.go#L90",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestPutHiddenTool",
			Body:    "Test: TestPutHiddenTool\nSource: internal/tools/tool_test.go\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "TestPutHiddenTool",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what tests cover put and get exposed tool behavior?")
	reasons := ExplainCandidates(got, "what tests cover put and get exposed tool behavior?")

	require.NotEmpty(t, got, "expected token test-name anchor, got %#v", CandidatePaths(got))
	assert.Equal(t, "internal/tools/tool_test.go#L42", got[0].Path,
		"expected token test-name anchor first, got %#v", CandidatePaths(got))

	require.NotEmpty(t, reasons, "missing token test-name reasons")
	assert.True(t, reasonContains(reasons[0].Reasons, "test-name token anchor"),
		"missing token test-name reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_AnchorsSnakeCaseTestNames(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "tests/tools_test.py#L12",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "test_put_and_get_exposed_tool",
			Body:    "Test: test_put_and_get_exposed_tool\nSource: tests/tools_test.py\n",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "test_put_and_get_exposed_tool",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what tests cover test_put_and_get_exposed_tool behavior?")
	require.NotEmpty(t, got, "expected snake-case test-name anchor, got %#v", CandidatePaths(got))
	assert.Equal(t, "tests/tools_test.py#L12", got[0].Path,
		"expected snake-case test-name anchor first, got %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_DoesNotUseTestCasesForOrdinaryRoadmapQueries(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/roadmap.md",
			Title: "Roadmap",
			Body:  "Roadmap for realtime multimodal voice agents and production readiness.",
		},
		{
			Path:    "tests/voice_test.py#L20",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "test_realtime_voice_agents",
			Body:    "Test: test_realtime_voice_agents\nSource: tests/voice_test.py\n",
			Metadata: map[string]string{
				"source_type": "test_case",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "roadmap for realtime multimodal voice agents")
	require.True(t, containsCandidatePath(got, "docs/roadmap.md"),
		"missing roadmap: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "tests/voice_test.py#L20"),
		"test case should not appear in ordinary roadmap retrieval: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_SuppressesRawTestFilesForOrdinaryPlanningQueries(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/plans/interactive-mode.md",
			Title: "Interactive Mode Plan",
			Body:  "Plan for mode active critical user flow and review state.",
		},
		{
			Path:  "packages/coding-agent/test/interactive-mode-plan-review.test.ts",
			Title: "interactive-mode-plan-review.test.ts",
			Body:  "mode active critical user flow review state.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "engineering context for mode active critical user flow")
	require.True(t, containsCandidatePath(got, "docs/plans/interactive-mode.md"),
		"missing planning doc: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "packages/coding-agent/test/interactive-mode-plan-review.test.ts"),
		"raw test file should not appear in non-test planning retrieval: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_PrefersTestUnitsOverRawTestFiles(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "services/billing/webhook_test.go",
			Title: "webhook_test.go",
			Body:  "TestWebhookReplayProtection stripe_event_id idempotency test file.",
		},
		{
			Path:    "services/billing/webhook_test.go#L12",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestWebhookReplayProtection",
			Body:    "Test: TestWebhookReplayProtection\nSource: services/billing/webhook_test.go\nSymbols: stripe_event_id, idempotency, webhook\n",
			Metadata: map[string]string{
				"source_type":       "test_case",
				"source_line_range": "12-24",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what tests cover TestWebhookReplayProtection stripe_event_id behavior")
	require.True(t, containsCandidatePath(got, "services/billing/webhook_test.go#L12"),
		"missing precise test unit: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "services/billing/webhook_test.go"),
		"raw test file should be suppressed when unit-level artifact exists: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_DoesNotRouteLegacyWordToCodeComments(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "processing/src/test/java/org/example/QuerySegmentSpecTest.java#L39",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "testSerializationLegacyString",
			Body:    "Test: testSerializationLegacyString\nSource: processing/src/test/java/org/example/QuerySegmentSpecTest.java\n",
			Metadata: map[string]string{
				"source_type": "test_case",
			},
		},
		{
			Path:    "processing/src/main/java/org/example/LegacyParser.java#L20",
			Kind:    "source_context",
			Subtype: "code_comment",
			Title:   "Compatibility: keep legacy parser path.",
			Body:    "Comment: Compatibility: keep legacy parser path.\nSource: processing/src/main/java/org/example/LegacyParser.java\n",
			Metadata: map[string]string{
				"source_type":  "code_comment",
				"comment_role": "compatibility",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "what tests cover testSerializationLegacyString behavior")
	require.True(t, containsCandidatePath(got, "processing/src/test/java/org/example/QuerySegmentSpecTest.java#L39"),
		"missing test unit: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "processing/src/main/java/org/example/LegacyParser.java#L20"),
		"legacy inside test name should not request code comments: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_DomainWordsDoNotRequestTests(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "docs/product-specs/course-visit-analytics.md",
			Kind:    "requirements",
			Subtype: "prd",
			Title:   "Course Visit Analytics",
			Body:    "Product spec for course visit analytics dashboards and operator reporting.",
			Metadata: map[string]string{
				"classifier_model": "prd",
			},
		},
		{
			Path:    "src/app/courseVisitTracking.test.ts#L11",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "tracks course visits",
			Body:    "Test: tracks course visits\nSource: src/app/courseVisitTracking.test.ts\nanalytics course visit tracking",
			Metadata: map[string]string{
				"source_type": "test_case",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "course visit analytics product spec")
	require.True(t, containsCandidatePath(got, "docs/product-specs/course-visit-analytics.md"),
		"missing product spec: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "src/app/courseVisitTracking.test.ts#L11"),
		"analytics domain word should not route tests without test intent: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_CodeTaskModePrefersSpecificEmbeddingSourceOverGenericModels(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "src/Appwrite/Utopia/Response/Model/User.php",
			Kind:  "source_context",
			Title: "User response model",
			Body:  "model endpoint update use project user response.",
		},
		{
			Path:  "src/Appwrite/Utopia/Response/Model/UsageUsers.php",
			Kind:  "source_context",
			Title: "Usage users response model",
			Body:  "model endpoint update use usage response.",
		},
		{
			Path:  "src/Appwrite/Utopia/Response/Model/Embedding.php",
			Kind:  "source_context",
			Title: "Embedding response model",
			Body:  "appwrite-embedding nomic embedding response model.",
		},
		{
			Path:  "src/Appwrite/Platform/Modules/Databases/Http/VectorsDB/Embeddings/Text/Create.php",
			Kind:  "source_context",
			Title: "Create embedding text endpoint",
			Body:  "nomic appwrite-embedding embedding endpoint.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTask}).Retrieve(candidates, "refactor: update embedding model and endpoint to use 'nomic' and 'appwrite-embedding'")
	reasons := ExplainCandidates(got, "refactor: update embedding model and endpoint to use 'nomic' and 'appwrite-embedding'")

	require.GreaterOrEqual(t, len(got), 2,
		"expected embedding results, got %#v", CandidatePaths(got))
	assert.Contains(t, []string{
		"src/Appwrite/Platform/Modules/Databases/Http/VectorsDB/Embeddings/Text/Create.php",
		"src/Appwrite/Utopia/Response/Model/Embedding.php",
	}, got[0].Path,
		"generic model won code-task ranking: %#v", CandidatePaths(got))

	require.NotEmpty(t, reasons, "missing source query ranking reasons")
	assert.True(t, reasonContainsPrefix(reasons[0].Reasons, "source query ranking:"),
		"missing source query ranking reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_CodeTaskModeDemotesAgentInstructionsForProductAgentTerm(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "AGENTS.md",
			Kind:    "markdown_artifact",
			Subtype: "agent_instruction",
			Title:   "Repository agent instructions",
			Body:    "Agents may use applications and secrets during command examples.",
		},
		{
			Path:  "pkg/cmd/secret/set/set.go",
			Kind:  "source_context",
			Title: "set secrets command",
			Body:  "Allow agents as application for secrets.",
		},
		{
			Path:  "pkg/cmd/secret/list/list.go",
			Kind:  "source_context",
			Title: "list secrets command",
			Body:  "List application secrets for agents.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTask}).Retrieve(candidates, "Allow agents as application for secrets")
	require.NotEmpty(t, got,
		"expected results")
	require.NotEqual(t, "AGENTS.md", got[0].Path,
		"agent instruction won product-agent code task: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got[:minInt(len(got), 2)], "pkg/cmd/secret/set/set.go"),
		"missing secret command source near top: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_CodeTaskFamilyPrefersCoreSourceFamilyOverDocsSrcTutorials(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs_src/pydantic_v1_in_v2/tutorial001.py",
			Kind:  "source_context",
			Title: "Pydantic v1 in v2 tutorial",
			Body:  "Update pydantic v2 code examples and deprecations tutorial.",
		},
		{
			Path:  "docs_src/pydantic_v1_in_v2/tutorial002.py",
			Kind:  "source_context",
			Title: "Pydantic v2 migration tutorial",
			Body:  "Pydantic v2 deprecations tutorial sample code.",
		},
		{
			Path:  "fastapi/encoders.py",
			Kind:  "source_context",
			Title: "jsonable encoder pydantic v2 compatibility",
			Body:  "Pydantic v2 deprecations model_dump serialization encoder implementation.",
		},
		{
			Path:  "fastapi/_compat.py",
			Kind:  "source_context",
			Title: "Pydantic compatibility helpers",
			Body:  "Pydantic v2 deprecations compatibility implementation.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTaskFamily}).Retrieve(candidates, "Update Pydantic v2 code to address deprecations")
	require.GreaterOrEqual(t, len(got), 2,
		"expected results, got %#v", CandidatePaths(got))

	assert.True(t, containsCandidatePath(got[:2], "fastapi/encoders.py"),
		"core source family should beat docs_src tutorials, got top=%#v all=%#v", CandidatePaths(got[:2]), CandidatePaths(got))
	assert.True(t, containsCandidatePath(got[:2], "fastapi/_compat.py"),
		"core source family should beat docs_src tutorials, got top=%#v all=%#v", CandidatePaths(got[:2]), CandidatePaths(got))
	assert.Equal(t, "core_runtime", got[0].Metadata["source_family_primary_role"],
		"expected core runtime family metadata, got %#v", got[0].Metadata)

}

func TestWeightedFilesRetrieverV0_CodeTaskFamilyProfilesMixedSourceTestDocsWithoutFlattening(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "services/context/permission.go",
			Kind:  "source_context",
			Title: "Token scope permission service",
			Body:  "enforce token scopes permission service context downloads attachments.",
		},
		{
			Path:    "services/context/permission_test.go",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "enforces token scopes for attachments",
			Body:    "test token scopes permission context attachment download.",
		},
		{
			Path:  "services/context/README.md",
			Kind:  "markdown_artifact",
			Title: "Context permissions",
			Body:  "Local reference for token scopes and permissions.",
		},
		{
			Path:  "docs/tutorial/token_scopes.md",
			Kind:  "markdown_artifact",
			Title: "Token scopes tutorial",
			Body:  "Tutorial for token scopes.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTaskFamily}).Retrieve(candidates, "enforce token scopes on attachment downloads")
	require.GreaterOrEqual(t, len(got), 2,
		"expected mixed family results, got %#v", CandidatePaths(got))
	require.Equal(t, "services/context/permission.go", got[0].Path,
		"implementation member should lead mixed family, got %#v", CandidatePaths(got))

	testIdx := candidatePathIndex(got, "services/context/permission_test.go")
	require.GreaterOrEqual(t, testIdx, 0,
		"missing colocated test family member: %#v", CandidatePaths(got))
	assert.Equal(t, "test_family", got[testIdx].Metadata["source_family_member_role"],
		"test member role was flattened: %#v", got[testIdx].Metadata)
	assert.Contains(t, got[testIdx].Metadata["source_family_member_roles_json"], "service_logic",
		"family profile should preserve service role: %#v", got[testIdx].Metadata)
	assert.Contains(t, got[testIdx].Metadata["source_family_member_roles_json"], "test_family",
		"family profile should preserve mixed member roles: %#v", got[testIdx].Metadata)

}

func TestWeightedFilesRetrieverV0_CodeTaskFamilyDemotesGeneratedClientSurfaceForRuntimeTask(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "pkg/client/applyconfiguration/monitoring/v1/prometheusrule.go",
			Kind:  "source_context",
			Title: "PrometheusRule apply configuration type",
			Body:  "prometheus rule operator generated client apply configuration type.",
		},
		{
			Path:  "pkg/operator/rules/rules.go",
			Kind:  "source_context",
			Title: "Prometheus operator rules reconciliation",
			Body:  "operator rules runtime validates prometheus rule groups and alert rules.",
		},
		{
			Path:    "pkg/operator/rules/rules_test.go",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "validates prometheus rule groups",
			Body:    "test operator rules validation behavior.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTaskFamily}).Retrieve(candidates, "support Prometheus operator rule validation")
	require.GreaterOrEqual(t, len(got), 2,
		"expected operator rule results, got %#v", CandidatePaths(got))
	require.Equal(t, "pkg/operator/rules/rules.go", got[0].Path,
		"runtime operator source should beat generated client surface: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got[:minInt(len(got), 2)], "pkg/client/applyconfiguration/monitoring/v1/prometheusrule.go"),
		"generated client surface should not be in top two for runtime task: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_CodeTaskFamilyV2LetsRareAnchorBeatGenericFlow(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "api/src/ai/tools/trigger-flow/index.ts",
			Kind:  "source_context",
			Title: "Trigger flow tool",
			Body:  "Improve automation flow execution and construct flow tree.",
		},
		{
			Path:  "api/src/utils/construct-flow-tree.ts",
			Kind:  "source_context",
			Title: "Construct flow tree",
			Body:  "Flow tree utility for automation flows.",
		},
		{
			Path:  "app/src/modules/settings/routes/flows/flow.vue",
			Kind:  "source_context",
			Title: "Flow settings route",
			Body:  "Flow route for settings and manual flow selection.",
		},
		{
			Path:  "app/src/modules/content/components/bookmark-add.vue",
			Kind:  "source_context",
			Title: "Bookmark add component",
			Body:  "Improve bookmark flow by adding bookmark handling.",
		},
		{
			Path:  "app/src/modules/content/components/bookmark-delete.vue",
			Kind:  "source_context",
			Title: "Bookmark delete component",
			Body:  "Improve bookmark deletion flow.",
		},
		{
			Path:  "app/src/modules/content/composables/use-delete-bookmark.ts",
			Kind:  "source_context",
			Title: "Use delete bookmark composable",
			Body:  "Bookmark flow delete implementation.",
		},
		{
			Path:    "app/src/modules/content/composables/use-delete-bookmark.test.ts",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "delete bookmark flow",
			Body:    "test bookmark delete behavior.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTaskFamilyV2}).Retrieve(candidates, "Improve bookmark flow")
	reasons := ExplainCandidates(got, "Improve bookmark flow")

	require.GreaterOrEqual(t, len(got), 3,
		"expected bookmark results, got %#v", CandidatePaths(got))

	topGroup := got[:minInt(len(got), 4)]
	assert.True(t, containsCandidatePath(topGroup, "app/src/modules/content/components/bookmark-add.vue"),
		"missing bookmark add from top group: %#v", CandidatePaths(topGroup))
	assert.True(t, containsCandidatePath(topGroup, "app/src/modules/content/components/bookmark-delete.vue"),
		"missing bookmark delete from top group: %#v", CandidatePaths(topGroup))
	assert.True(t, containsCandidatePath(topGroup, "app/src/modules/content/composables/use-delete-bookmark.ts"),
		"missing bookmark composable from top group: %#v", CandidatePaths(topGroup))
	assert.True(t, containsCandidatePath(topGroup, "app/src/modules/content/composables/use-delete-bookmark.test.ts"),
		"missing bookmark test from top group: %#v", CandidatePaths(topGroup))
	assert.False(t, containsCandidatePath(topGroup, "api/src/ai/tools/trigger-flow/index.ts"),
		"generic flow family should not lead bookmark query: %#v", CandidatePaths(got))

	require.NotEmpty(t, reasons, "missing query hygiene reasons")
	assert.True(t, reasonContainsPrefix(reasons[0].Reasons, "query hygiene:"),
		"missing query hygiene reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_CodeTaskFamilyV2PrefersSpecificPluginTestOverPlayground(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "packages/vite/src/node/plugins/importMetaGlob.ts",
			Kind:  "source_context",
			Title: "Import glob plugin",
			Body:  "match import glob common base by path segment correctly.",
		},
		{
			Path:    "packages/vite/src/node/__tests__/plugins/importGlob/utils.spec.ts",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "match import glob common base by path segment",
			Body:    "tests import glob common base path segment behavior.",
		},
		{
			Path:    "playground/glob-import/__tests__/glob-import.spec.ts",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "glob import playground",
			Body:    "playground import glob common base behavior.",
		},
		{
			Path:  "playground/glob-import/root/array-common-base/pattern1/a.js",
			Kind:  "source_context",
			Title: "Glob import common base fixture",
			Body:  "import glob common base fixture.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeCodeTaskFamilyV2}).Retrieve(candidates, "Match import glob common base by path segment correctly")
	require.GreaterOrEqual(t, len(got), 2,
		"expected import glob results, got %#v", CandidatePaths(got))
	require.Equal(t, "packages/vite/src/node/plugins/importMetaGlob.ts", got[0].Path,
		"plugin source should lead import glob query: %#v", CandidatePaths(got))
	require.False(t, strings.HasPrefix(got[0].Path, "playground/"),
		"playground fixtures should not lead implementation query: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_UsesCodeCommentsForRationaleQueries(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "services/billing/webhook.go#L18",
			Kind:    "source_context",
			Subtype: "code_comment",
			Title:   "Invariant: stripe_event_id must always be checked before applying credits.",
			Body:    "Comment: Invariant: stripe_event_id must always be checked before applying credits.\nSource: services/billing/webhook.go\nRole: invariant\n",
			Metadata: map[string]string{
				"source_type":  "code_comment",
				"comment_role": "invariant",
			},
		},
		{
			Path:  "docs/plans/billing-hardening.md",
			Title: "Billing Hardening Plan",
			Body:  "Plan for webhook processing and credit application.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "implementation rationale for stripe_event_id invariant in code")
	reasons := ExplainCandidates(got, "implementation rationale for stripe_event_id invariant in code")

	assert.True(t, containsCandidatePath(got, "services/billing/webhook.go#L18"),
		"missing code-comment candidate: %#v", CandidatePaths(got))
	reasonIndex := reasonPathIndex(reasons, "services/billing/webhook.go#L18")
	require.GreaterOrEqual(t, reasonIndex, 0,
		"missing reasons for code-comment candidate: %#v", reasons)
	assert.True(t, reasonContains(reasons[reasonIndex].Reasons, "code-comment rationale signal"),
		"missing code-comment reason: %#v", reasons[reasonIndex].Reasons)

}

func TestWeightedFilesRetrieverV0_DoesNotBackfillWeakBodyOnlyMarkdown(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/design-docs/langfuse-trace-association.md",
			Title: "Langfuse Trace Association",
			Body:  "Design for Langfuse trace association across ask and runtime generations.",
		},
		{
			Path:     "AGENTS.md",
			Title:    "Agent Instructions",
			Subtype:  "agent_instruction",
			Body:     "Langfuse trace association rules for ask runtime generations.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
		{
			Path:  "docs/billing-subscription-design.md",
			Title: "Billing Subscription Design",
			Body:  "Ask runtime workers share the same tree for unrelated billing subscription flows.",
		},
		{
			Path:  "docs/shared-admin-table-component.md",
			Title: "Shared Admin Table Component",
			Body:  "Runtime views share the same tree for admin tables.",
		},
		{
			Path:  "docs/engineering-baseline.md",
			Title: "Engineering Baseline",
			Body:  "Fix ask runtime defaults shared by unrelated engineering tasks.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "fix Langfuse trace association so ask and runtime generations share the same trace tree")
	require.True(t, containsCandidatePath(got, "docs/design-docs/langfuse-trace-association.md"),
		"missing anchored design doc: %#v", CandidatePaths(got))

	assert.False(t, containsCandidatePath(got, "AGENTS.md"),
		"agent instructions should not backfill body-only retrieval: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/billing-subscription-design.md"),
		"billing design should not backfill body-only retrieval: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/shared-admin-table-component.md"),
		"admin table design should not backfill body-only retrieval: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/engineering-baseline.md"),
		"engineering baseline should not backfill body-only retrieval: %#v", CandidatePaths(got))
}

func TestWeightedFilesRetrieverV0_KeepsSmallCandidateSets(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "ROADMAP.md",
			Title: "CloudNativePG Roadmap",
			Body:  "Roadmap and contributor prioritization.",
		},
		{
			Path:  "docs/src/architecture.md",
			Title: "Architecture",
			Body:  "The operator performs direct pod management without StatefulSets and coordinates instance manager failover.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "CloudNativePG operator architecture direct pod management no StatefulSets instance manager failover")
	require.True(t, containsCandidatePath(got, "docs/src/architecture.md"),
		"missing architecture doc from small candidate set: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_ResumeIntentKeepsMatchingDecisionContext(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/plans/2026-05-01-entitlement-sync-plan.md", Body: "Current progress for entitlement_sync hardening and billing-webhook-hardening."},
		{Path: "docs/adr/0002-webhook-idempotency-boundary.md", Body: "Decision: billing-webhook-hardening uses entitlement_sync after durable webhook idempotency."},
		{Path: "docs/prd/billing-entitlements-v1.md", Body: "Product requirements mention entitlement_sync, entitlements, customers, access, and billing."},
		{Path: "services/api/src/billing/entitlements.ts", Body: "function entitlement_sync() { return billingWebhookHardening(); }"},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume entitlement sync hardening")
	require.True(t, containsCandidatePath(got, "docs/adr/0002-webhook-idempotency-boundary.md"),
		"missing matching ADR decision context: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_LifecycleIntentPrefersStaleDecision(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/adr/0003-superseded-local-entitlements.md", Status: "superseded", Body: "The local entitlement caching plan was abandoned."},
		{Path: "docs/plans/active-entitlement-rollout.md", Status: "active", Body: "Mentions local entitlement caching as old context but tracks current rollout."},
		{Path: ".claude/notes/local-entitlements-experiment.md", Status: "stale", Body: "Historical local entitlement cache experiment."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "continue local entitlement caching plan")
	require.True(t, containsCandidatePath(got, "docs/adr/0003-superseded-local-entitlements.md"),
		"missing superseded ADR: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "docs/plans/active-entitlement-rollout.md"),
		"active rollout should not beat lifecycle candidates: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_ExpandsOpenSpecCompanionsWithoutParentNoise(t *testing.T) {
	candidates := []Candidate{
		{
			ID:       "bundle_1",
			Path:     "openspec/changes/add-sso",
			Title:    "Add SSO",
			Body:     "OpenSpec bundle for add-sso.",
			Metadata: map[string]string{"artifact_scope": "bundle", "openspec_role": "change_bundle"},
		},
		{
			ID:    "tasks_1",
			Path:  "openspec/changes/add-sso/tasks.md",
			Title: "Tasks",
			Body:  "Tasks for add-sso OAuth provider setup.",
			Metadata: map[string]string{
				"artifact_scope":          "file",
				"openspec_role":           "tasks",
				"link_contained_by":       "artifact:bundle_1",
				"link_openspec_companion": "artifact:design_1",
			},
		},
		{
			ID:    "design_1",
			Path:  "openspec/changes/add-sso/design.md",
			Title: "Design",
			Body:  "Design for add-sso OAuth provider setup.",
			Metadata: map[string]string{
				"artifact_scope":    "file",
				"openspec_role":     "design",
				"link_contained_by": "artifact:bundle_1",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume OAuth provider tasks")
	require.True(t, containsCandidatePath(got, "openspec/changes/add-sso/tasks.md"),
		"missing tasks child: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "openspec/changes/add-sso/design.md"),
		"missing expanded design companion: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "openspec/changes/add-sso"),
		"structural parent bundle should not be included for ordinary task retrieval: %#v", CandidatePaths(got))

	designIndex := candidatePathIndex(got, "openspec/changes/add-sso/design.md")
	require.GreaterOrEqual(t, designIndex, 0, "missing design companion: %#v", CandidatePaths(got))
	assert.Equal(t, "openspec_companion", got[designIndex].Metadata["retrieval_expansion_reason"],
		"companion expansion reason = %#v", got[designIndex].Metadata)
}

func TestWeightedFilesRetrieverV0_IncludesOpenSpecParentForStructureIntent(t *testing.T) {
	candidates := []Candidate{
		{
			ID:       "bundle_1",
			Path:     "openspec/changes/add-sso",
			Title:    "Add SSO",
			Body:     "OpenSpec bundle for add-sso.",
			Metadata: map[string]string{"artifact_scope": "bundle", "openspec_role": "change_bundle"},
		},
		{
			ID:    "tasks_1",
			Path:  "openspec/changes/add-sso/tasks.md",
			Title: "Tasks",
			Body:  "Tasks for add-sso OAuth provider setup.",
			Metadata: map[string]string{
				"artifact_scope":    "file",
				"openspec_role":     "tasks",
				"link_contained_by": "artifact:bundle_1",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "OpenSpec change bundle for add-sso OAuth provider")
	require.True(t, containsCandidatePath(got, "openspec/changes/add-sso"),
		"missing explicit OpenSpec parent bundle: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_ProductBackgroundAnchorsNamedSubject(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/prd/billing-entitlements-v1.md", Body: "Product requirements for billing entitlements and customer access."},
		{Path: "docs/prd/billing-analytics-v1.md", Body: "Product background for billing analytics, customers, access, entitlement convergence, and support."},
		{Path: "docs/prd/customer-portal-v2.md", Body: "Product background for customer portal billing access and entitlements."},
		{Path: "docs/adr/0001-use-stripe-as-billing-source.md", Status: "accepted", Body: "Stripe is the authoritative billing source. customer_id joins Stripe to entitlement records and access checks."},
		{Path: "docs/adr/0002-webhook-idempotency-boundary.md", Status: "accepted", Body: "The idempotency boundary for billing webhooks prevents entitlement_sync replay from creating confusing customer access state."},
		{Path: "docs/adr/0004-admin-billing-overrides.md", Status: "accepted", Body: "Admin billing overrides mention customer access and entitlement materialization, but are a separate support feature."},
		{Path: "services/api/src/billing/entitlements.ts", Body: "customer access billing entitlements implementation code"},
		{Path: "docs/plans/customer-access-notes.md", Body: "customer access support notes"},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "product background for billing entitlements and customer access")
	assert.True(t, containsCandidatePath(got, "docs/prd/billing-entitlements-v1.md"),
		"missing billing entitlements PRD: %#v", CandidatePaths(got))
	assert.True(t, containsCandidatePath(got, "docs/adr/0001-use-stripe-as-billing-source.md"),
		"missing Stripe source ADR: %#v", CandidatePaths(got))
	assert.True(t, containsCandidatePath(got, "docs/adr/0002-webhook-idempotency-boundary.md"),
		"missing webhook idempotency ADR: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/prd/billing-analytics-v1.md"),
		"billing analytics PRD should not be selected: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/prd/customer-portal-v2.md"),
		"customer portal PRD should not be selected: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/adr/0004-admin-billing-overrides.md"),
		"admin override ADR should not be selected: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "services/api/src/billing/entitlements.ts"),
		"implementation source should not be selected: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "docs/plans/customer-access-notes.md"),
		"planning notes should not be selected: %#v", CandidatePaths(got))
}

func TestWeightedFilesRetrieverV0_BridgesArtifactPhrasesToAcronyms(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "agents/prd.agent.md",
			Title:    "Create PRD Chat Mode",
			Subtype:  "agent_instruction",
			Body:     "Generate Product Requirements Documents with user stories and acceptance criteria.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
		{
			Path:     "agents/atlassian-requirements-to-jira.agent.md",
			Title:    "Requirements to Jira",
			Subtype:  "agent_instruction",
			Body:     "Convert requirements into Jira issues.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
		{
			Path:     "skills/reference/documentation-full.md",
			Title:    "Documentation Reference",
			Subtype:  "skill",
			Body:     "Generic product documentation with users and acceptance examples.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "agent instructions for generating a product requirements document with user stories and acceptance criteria")
	require.True(t, containsCandidatePath(got, "agents/prd.agent.md"),
		"missing PRD agent via product requirements document bridge: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "skills/reference/documentation-full.md"),
		"generic documentation should not beat PRD path/title match: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_UsesClassifierRoleForDesignDocs(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "docs/docs/en/architecture/design.md",
			Kind:     "design",
			Title:    "Architecture Design",
			Body:     "Master Worker API Alert DAO modules and distributed architecture.",
			Metadata: map[string]string{"classifier_model": "rfc", "classifier_kind": "design"},
		},
		{
			Path:     "CLAUDE.md",
			Subtype:  "agent_instruction",
			Title:    "Repository Instructions",
			Body:     "Master Worker API Alert DAO modules and distributed architecture instructions.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
		{
			Path:     "module/CLAUDE.md",
			Subtype:  "agent_instruction",
			Title:    "Module Instructions",
			Body:     "Master Worker API Alert DAO module implementation instructions.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "architecture design for master worker api alert and dao modules")
	require.True(t, containsCandidatePath(got, "docs/docs/en/architecture/design.md"),
		"missing classified architecture design doc: %#v", CandidatePaths(got))

	assert.False(t, containsCandidatePath(got, "CLAUDE.md"),
		"root instructions should not appear for non-protocol design query: %#v", CandidatePaths(got))
	assert.False(t, containsCandidatePath(got, "module/CLAUDE.md"),
		"module instructions should not appear for non-protocol design query: %#v", CandidatePaths(got))
}

func TestWeightedFilesRetrieverV0_IncludesProtocolGuidelinesWhenRequested(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "DESIGN_GUIDELINES.md",
			Kind:    "markdown_artifact",
			Subtype: "standard",
			Title:   "MCP Server Design Guidelines",
			Body:    "Design guidelines and best practices for developing MCP servers, including project structure and code organization.",
			Metadata: map[string]string{
				"classifier_family": "protocol.standard",
				"classifier_mode":   "protocol",
			},
		},
		{
			Path:  "src/example/server.py",
			Kind:  "source_context",
			Title: "src/example/server.py (python)",
			Body:  "Example MCP server implementation.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "add or update an AWS MCP server; follow the MCP server design guidelines first")
	require.True(t, containsCandidatePath(got, "DESIGN_GUIDELINES.md"),
		"missing requested protocol guidelines: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_PrefersRepositoryWideInstructionsWhenRequested(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "CLAUDE.md",
			Subtype:  "agent_instruction",
			Title:    "Repository Instructions",
			Body:     "Project-wide Claude Code development guidance.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
		{
			Path:     "service/CLAUDE.md",
			Subtype:  "agent_instruction",
			Title:    "Service Instructions",
			Body:     "Service-specific Claude Code development guidance.",
			Metadata: map[string]string{"classifier_mode": "protocol"},
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "Claude Code repository instructions and development guidance")
	require.True(t, containsCandidatePath(got, "CLAUDE.md"),
		"missing shallow repository instructions: %#v", CandidatePaths(got))
	require.False(t, containsCandidatePath(got, "service/CLAUDE.md"),
		"nested instructions should not backfill repository-wide instruction query: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_KeepsRoadmapPathSignal(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/roadmap.md",
			Kind:  "plan",
			Title: "Roadmap",
			Body:  "Roadmap for realtime multimodal voice agents, tool invocation, and production readiness.",
		},
	}

	query := "roadmap for realtime multimodal voice agents, tool invocation, and production readiness"
	score := scoreCandidate(candidates[0], expandedTerms(query), query)
	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)

	require.GreaterOrEqual(t, score, 4.0,
		"roadmap score = %.2f, want retrievable", score)
	require.True(t, containsCandidatePath(got, "docs/roadmap.md"),
		"missing roadmap path signal: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_AuthorityPriorDoesNotCreateUnrelatedMatches(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/prd/billing-entitlements.md",
			Kind:  "requirements",
			Title: "Billing Entitlements",
			Body:  "Product requirements for billing entitlements.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "oauth provider session handoff")
	require.Empty(t, got,
		"authority prior should not rescue unrelated canonical docs: %#v", CandidatePaths(got))

}

func TestRankConceptCandidates_CompactTestIdentifier(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "pkg/tools/exposed_tool_test.go#L24-L39",
			Subtype:  "test_case",
			Title:    "TestPutAndGetExposedTool",
			Body:     "Test: TestPutAndGetExposedTool\nassert exposed tool is returned.",
			Metadata: map[string]string{"source_type": "test_case", "test_name": "TestPutAndGetExposedTool"},
		},
		{
			Path:  "docs/testing.md",
			Title: "Testing",
			Body:  "General tests cover tool behavior.",
		},
	}

	ranks := RankConceptCandidates(candidates, "what tests cover testputandgetexposedtool behavior")
	require.NotEmpty(t, ranks, "expected compact test identifier rank")
	assert.Equal(t, "pkg/tools/exposed_tool_test.go#L24-L39", ranks[0].Path,
		"compact test identifier should rank first: %#v", ranks)
	assert.True(t, containsString(ranks[0].MatchedCompacts, "testputandgetexposedtool"),
		"missing compact diagnostic: %#v", ranks[0])

}

func TestApplyConceptBackfill_AddsSpecificProductRequirementsDoc(t *testing.T) {
	selected := []Candidate{{Path: "docs/roadmap.md", Body: "Roadmap for unrelated launch work."}}
	universe := []Candidate{
		selected[0],
		{
			Path:    "docs/product-specs/fluxnova-aigf.md",
			Kind:    "requirements",
			Subtype: "prd",
			Title:   "FluxNova AIGF Requirements",
			Body:    "Product requirements for FluxNova AIGF workflows.",
		},
		{
			Path:  "docs/product-specs/general-ai-video.md",
			Title: "AI Video",
			Body:  "Generic product requirements and background.",
		},
	}

	query := "FluxNova AIGF requirements"
	got := applyConceptBackfill(selected, universe, query, strings.ToLower(query), expandedTerms(query), 5, false, false)
	require.True(t, containsCandidatePath(got, "docs/product-specs/fluxnova-aigf.md"),
		"missing specific product requirements backfill: %#v", CandidatePaths(got))

	backfillIndex := candidatePathIndex(got, "docs/product-specs/fluxnova-aigf.md")
	require.GreaterOrEqual(t, backfillIndex, 0)
	assert.NotEmpty(t, got[backfillIndex].Metadata["concept_backfill_score"],
		"missing concept backfill metadata: %#v", got[backfillIndex].Metadata)
}

func TestApplyConceptBackfill_IgnoresBroadTemplateNoise(t *testing.T) {
	universe := []Candidate{
		{
			Path:     "templates/PROPOSAL_TEMPLATE.md",
			Subtype:  "template",
			Title:    "Proposal Template",
			Body:     "Architecture requirements instructions template for proposals.",
			Metadata: map[string]string{"classifier_mode": "template"},
		},
	}

	query := "architecture requirements instructions"
	got := applyConceptBackfill(nil, universe, query, strings.ToLower(query), expandedTerms(query), 5, false, false)
	require.Empty(t, got,
		"broad query should not backfill template noise: %#v", CandidatePaths(got))

}

func TestApplyConceptBackfillWithGlossary_SuppressesBroadRepoConcept(t *testing.T) {
	var universe []Candidate
	for i := 0; i < 16; i++ {
		universe = append(universe, Candidate{
			Path:  fmt.Sprintf("docs/cloudnativepg/module-%02d.md", i),
			Title: fmt.Sprintf("CloudNativePG Module %02d", i),
			Body:  "Generic module documentation.",
		})
	}
	universe = append(universe, Candidate{
		Path:     "tests/e2e/suite_test.go#L236",
		Subtype:  "test_case",
		Title:    "CloudNativePG upgrade suite",
		Body:     "Test: CloudNativePG upgrade suite",
		Metadata: map[string]string{"source_type": "test_case", "test_name": "CloudNativePG upgrade suite"},
	})

	query := "CloudNativePG roadmap process"
	got := applyConceptBackfill(nil, universe, query, strings.ToLower(query), expandedTerms(query), 5, true, false)
	require.Empty(t, got,
		"glossary should suppress broad repo concept backfill: %#v", CandidatePaths(got))

}

func TestRankConceptCandidatesWithGlossary_WithExactTestName_RetainsDiagnosticEvidence(t *testing.T) {
	candidates := []Candidate{{
		Path:    "pkg/tools/exposed_tool_test.go#L24-L39",
		Subtype: "test_case",
		Title:   "TestPutAndGetExposedTool",
		Body:    "Test: TestPutAndGetExposedTool\nassert exposed tool is returned.",
		Metadata: map[string]string{
			"source_type": "test_case",
			"test_name":   "TestPutAndGetExposedTool",
		},
	}}

	ranks := RankConceptCandidatesWithGlossary(candidates, "what tests cover testputandgetexposedtool behavior")

	require.Len(t, ranks, 1)
	assert.Equal(t, "pkg/tools/exposed_tool_test.go#L24-L39", ranks[0].Path)
	assert.NotEmpty(t, ranks[0].MatchedCompacts)
}

func TestApplyConceptBackfillWithGlossary_KeepsRareProductConcept(t *testing.T) {
	selected := []Candidate{{Path: "docs/roadmap.md", Body: "Roadmap for unrelated launch work."}}
	universe := append([]Candidate{}, selected...)
	for i := 0; i < 12; i++ {
		universe = append(universe, Candidate{
			Path:  fmt.Sprintf("docs/product-specs/general-%02d.md", i),
			Title: fmt.Sprintf("General Product Spec %02d", i),
			Body:  "Generic product requirements and background.",
		})
	}
	universe = append(universe, Candidate{
		Path:    "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
		Kind:    "requirements",
		Subtype: "prd",
		Title:   "FluxNova AIGF Requirements",
		Body:    "Requirements for FluxNova templates and AIGF integration.",
	})

	query := "FluxNova AIGF requirements"
	got := applyConceptBackfill(selected, universe, query, strings.ToLower(query), expandedTerms(query), 5, true, false)
	require.True(t, containsCandidatePath(got, "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md"),
		"glossary should preserve rare product concept backfill: %#v", CandidatePaths(got))

	backfillIndex := candidatePathIndex(got, "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md")
	require.GreaterOrEqual(t, backfillIndex, 0)
	assert.Equal(t, "true", got[backfillIndex].Metadata["concept_glossary_enabled"],
		"missing glossary metadata: %#v", got[backfillIndex].Metadata)
}

func TestApplyConceptBackfillWithGlossary_KeepsExactTestNameConcept(t *testing.T) {
	universe := []Candidate{
		{
			Path:     "pkg/tools/exposed_tool_test.go#L24-L39",
			Subtype:  "test_case",
			Title:    "TestPutAndGetExposedTool",
			Body:     "Test: TestPutAndGetExposedTool\nassert exposed tool is returned.",
			Metadata: map[string]string{"source_type": "test_case", "test_name": "TestPutAndGetExposedTool"},
		},
	}

	query := "what tests cover testputandgetexposedtool behavior"
	got := applyConceptBackfill(nil, universe, query, strings.ToLower(query), expandedTerms(query), 5, true, false)
	require.True(t, containsCandidatePath(got, "pkg/tools/exposed_tool_test.go#L24-L39"),
		"glossary should preserve exact test-name concept: %#v", CandidatePaths(got))

}

func TestConceptBackfillTier_PrimaryForExactTestName(t *testing.T) {
	candidate := Candidate{
		Path:     "pkg/tools/exposed_tool_test.go#L24-L39",
		Subtype:  "test_case",
		Title:    "TestPutAndGetExposedTool",
		Body:     "Test: TestPutAndGetExposedTool",
		Metadata: map[string]string{"source_type": "test_case", "test_name": "TestPutAndGetExposedTool"},
	}
	rank := ConceptRank{
		Candidate:       candidate,
		Path:            candidate.Path,
		Score:           36,
		MatchedCompacts: []string{"testputandgetexposedtool"},
	}
	profile := buildConceptQueryProfile("what tests cover testputandgetexposedtool behavior")

	tier, reason := conceptBackfillTier(rank, profile, profile.queryLower, true)
	require.Equal(t, PackTierPrimary, tier,
		"exact test-name concept should stay primary, tier=%q reason=%q", tier, reason)

}

func TestApplyConceptBackfillTiered_DemotesPlausiblePlanToRelated(t *testing.T) {
	selected := []Candidate{{Path: "docs/roadmap.md", Body: "Roadmap for unrelated launch work."}}
	universe := []Candidate{
		selected[0],
		{
			Path:  "docs/plans/billing-entitlement-rollout.md",
			Kind:  "plan",
			Title: "Billing Entitlement Rollout",
			Body:  "Plan for billing entitlement rollout and integration work.",
		},
	}

	query := "billing entitlement rollout integration"
	got := applyConceptBackfill(selected, universe, query, strings.ToLower(query), expandedTerms(query), 5, false, true)
	var target *Candidate
	for index := range got {
		if got[index].Path != "docs/plans/billing-entitlement-rollout.md" {
			continue
		}
		target = &got[index]
		break
	}
	require.NotNil(t, target, "missing tiered concept backfill: %#v", CandidatePaths(got))
	assert.Equal(t, PackTierRelated, target.Metadata["pack_tier"])
}

func TestWeightedFilesRetrieverV0_AuthorityPriorRanksCanonicalCurrentArtifacts(t *testing.T) {
	query := "architecture design for API boundary"
	terms := expandedTerms(query)
	queryLower := "architecture design for api boundary"
	canonical := Candidate{
		Path:     "docs/architecture/design.md",
		Kind:     "design",
		Title:    "Architecture Design",
		Body:     "API boundary design.",
		Metadata: map[string]string{"classifier_model": "rfc", "classifier_confidence": "0.900"},
	}
	archived := Candidate{
		Path:     "docs/archive/architecture/design.md",
		Kind:     "design",
		Title:    "Architecture Design",
		Body:     "API boundary design.",
		Metadata: map[string]string{"classifier_model": "rfc", "classifier_confidence": "0.900"},
	}

	canonicalScore := scoreCandidate(canonical, terms, queryLower) + authorityPrior(canonical, candidateRole(canonical), queryLower).score
	archivedScore := scoreCandidate(archived, terms, queryLower) + authorityPrior(archived, candidateRole(archived), queryLower).score
	require.Greater(t, canonicalScore, archivedScore,
		"canonical score %.2f should beat archived score %.2f", canonicalScore, archivedScore)

}

func TestWeightedFilesRetrieverV0_RanksActiveIntentAboveBlockedHistoricalPlan(t *testing.T) {
	query := "epoch 4 external validity bridge"
	candidates := []Candidate{
		{
			Path:   "docs/plans/D4.2-blocked-external-validity-bridge.md",
			Kind:   "plan",
			Title:  "D4.2 blocked external validity bridge",
			Status: "blocked",
			Body:   "Status: blocked\nExternal validity bridge plan from an older epoch.",
		},
		{
			Path:   "docs/notes/next_epoch_decision_memo.md",
			Kind:   "plan",
			Title:  "Epoch 4 external validity bridge decision memo",
			Status: "next",
			Body:   "Status: next\nOwner decision: Epoch 4 external validity bridge is the active work.",
		},
		{
			Path:  "docs/north_star.md",
			Kind:  "plan",
			Title: "Research north star",
			Body:  "Active phase: Epoch 4 external validity bridge. Route current implementation slices here.",
		},
		{
			Path:  "docs/plans/PLAN-008.1-synthetic-repo-world.md",
			Kind:  "plan",
			Title: "PLAN-008.1 synthetic repo world",
			Body:  "Historical external validity analogy for synthetic repo world experiments.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)
	require.NotEmpty(t, got,
		"expected matches")

	topTwo := CandidatePaths(got)
	if len(topTwo) > 2 {
		topTwo = topTwo[:2]
	}
	assert.True(t, containsString(topTwo, "docs/notes/next_epoch_decision_memo.md"),
		"active decision memo should rank in the top two, got %#v", CandidatePaths(got))
	assert.True(t, containsString(topTwo, "docs/north_star.md"),
		"active decision docs should outrank historical plans, got %#v", CandidatePaths(got))

	blockedIndex := candidatePathIndex(got, "docs/plans/D4.2-blocked-external-validity-bridge.md")
	activeIndex := candidatePathIndex(got, "docs/notes/next_epoch_decision_memo.md")
	if blockedIndex >= 0 && activeIndex >= 0 {
		assert.Greater(t, blockedIndex, activeIndex,
			"blocked historical plan outranked active intent: %#v", CandidatePaths(got))
	}

}

func TestExplainCandidates_ActiveOwnerDecisionReportsAuthorityPrior(t *testing.T) {
	query := "epoch 4 external validity bridge"
	candidates := []Candidate{{
		Path:   "docs/notes/next_epoch_decision_memo.md",
		Kind:   "plan",
		Title:  "Epoch 4 external validity bridge decision memo",
		Status: "next",
		Body:   "Status: next\nOwner decision: Epoch 4 external validity bridge is the active work.",
	}}

	reasons := ExplainCandidates(candidates, query)

	require.NotEmpty(t, reasons, "missing owner decision reasons")
	assert.True(t, reasonContains(reasons[0].Reasons, "authority prior: owner decision record"),
		"missing owner decision authority reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_BrownfieldRecoveryWithoutCurrentDecisionDocsKeepsBlockedPlanVisible(t *testing.T) {
	query := "epoch 4 external validity bridge"
	candidates := brownfieldRecoveryHistoricalCandidates()

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)

	assert.True(t, containsCandidatePath(got, "docs/plans/D4.2-blocked-external-validity-bridge.md"),
		"blocked historical plan should remain visible when no current decision artifact exists, got %#v", CandidatePaths(got))
}

func TestExplainCandidates_BrownfieldRecoveryBlockedPlanExplainsInactiveLifecycle(t *testing.T) {
	query := "epoch 4 external validity bridge"
	candidates := brownfieldRecoveryHistoricalCandidates()

	reasons := ExplainCandidates([]Candidate{candidates[0]}, query)

	require.NotEmpty(t, reasons, "missing blocked plan reasons")
	assert.True(t, reasonContains(reasons[0].Reasons, "authority prior: blocked, closed, stale, or superseded"),
		"blocked historical plan should explain inactive lifecycle, got %#v", reasons)
}

func TestWeightedFilesRetrieverV0_BrownfieldRecoveryWithCurrentDecisionDocsPrefersCurrentAnchors(t *testing.T) {
	query := "epoch 4 external validity bridge"
	candidates := append(brownfieldRecoveryHistoricalCandidates(),
		Candidate{
			Path:   "docs/notes/next_epoch_decision_memo.md",
			Kind:   "plan",
			Title:  "Epoch 4 external validity bridge decision memo",
			Status: "next",
			Body:   "Status: next\nOwner decision: Epoch 4 external validity bridge is the active bridge work.",
		},
		Candidate{
			Path:  "docs/north_star.md",
			Kind:  "plan",
			Title: "Research north star",
			Body:  "Active phase: Epoch 4 external validity bridge. Route current implementation slices here.",
		},
	)

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)

	topTwo := CandidatePaths(got)
	if len(topTwo) > 2 {
		topTwo = topTwo[:2]
	}
	assert.True(t, containsString(topTwo, "docs/notes/next_epoch_decision_memo.md"),
		"decision memo should become a brownfield recovery anchor: %#v", CandidatePaths(got))
	assert.True(t, containsString(topTwo, "docs/north_star.md"),
		"north star should become a brownfield recovery anchor: %#v", CandidatePaths(got))

	northStarIndex := candidatePathIndex(got, "docs/north_star.md")
	require.GreaterOrEqual(t, northStarIndex, 0, "missing north star: %#v", CandidatePaths(got))
	assert.NotEmpty(t, got[northStarIndex].Metadata["current_intent_anchor_reason"],
		"active phase doc should carry current-intent anchor metadata, got %#v", got)

	blockedIndex := candidatePathIndex(got, "docs/plans/D4.2-blocked-external-validity-bridge.md")
	decisionIndex := candidatePathIndex(got, "docs/notes/next_epoch_decision_memo.md")
	require.GreaterOrEqual(t, blockedIndex, 0, "missing blocked historical plan: %#v", CandidatePaths(got))
	require.GreaterOrEqual(t, decisionIndex, 0, "missing current decision memo: %#v", CandidatePaths(got))
	assert.Greater(t, blockedIndex, decisionIndex,
		"blocked historical plan outranked current decision doc after it existed: %#v", CandidatePaths(got))
	assert.NotEmpty(t, got[blockedIndex].Metadata["current_intent_demotion_reason"],
		"blocked historical plan should carry current-intent demotion metadata, got %#v", got[blockedIndex].Metadata)

}

func brownfieldRecoveryHistoricalCandidates() []Candidate {
	return []Candidate{
		{
			Path:   "docs/plans/D4.2-blocked-external-validity-bridge.md",
			Kind:   "plan",
			Title:  "D4.2 blocked external validity bridge",
			Status: "blocked",
			Body:   "Status: blocked\nExternal validity bridge plan from an older epoch. Blocked on repo-world assumptions.",
		},
		{
			Path:   "docs/plans/T00-epoch-index.md",
			Kind:   "plan",
			Title:  "T00 epoch planning index",
			Status: "superseded",
			Body:   "Status: superseded\nOlder epoch index mentioning external validity bridge work.",
		},
		{
			Path:  "docs/plans/PLAN-008.1-synthetic-repo-world.md",
			Kind:  "plan",
			Title: "PLAN-008.1 synthetic repo world",
			Body:  "Historical external validity analogy for synthetic repo world experiments.",
		},
	}
}

func TestWeightedFilesRetrieverV0_ExactPlanIDKeepsDirectArtifactAndLinkedNeighborAheadOfHistoricalAnalog(t *testing.T) {
	query := "EV-R1 external validity bridge"
	candidates := []Candidate{
		{
			Path:   "docs/plans/PLAN-EV-R1-external-validity-bridge.md",
			Kind:   "plan",
			Title:  "PLAN-EV-R1: external validity bridge",
			Status: "next",
			Body:   "Status: next\nImplement the EV-R1 external validity bridge.",
		},
		{
			Path:   "docs/plans/PLAN-EV00-index.md",
			Kind:   "plan",
			Title:  "PLAN-EV00: external validity index",
			Status: "current",
			Body:   "Direct next slice: EV-R1. Follow with EV-R2 only after EV-R1 promotes.",
		},
		{
			Path:  "docs/plans/PLAN-008.1-synthetic-repo-world.md",
			Kind:  "plan",
			Title: "PLAN-008.1 synthetic repo world bridge",
			Body:  strings.Repeat("external validity bridge historical synthetic repo world context.\n", 12),
		},
		{
			Path:  "docs/plans/external-validity-bridge-notes.md",
			Kind:  "plan",
			Title: "External validity bridge notes",
			Body:  strings.Repeat("external validity bridge notes and operational context.\n", 8),
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, query)
	paths := CandidatePaths(got)
	direct := candidatePathIndex(got, "docs/plans/PLAN-EV-R1-external-validity-bridge.md")
	neighbor := candidatePathIndex(got, "docs/plans/PLAN-EV00-index.md")
	historical := candidatePathIndex(got, "docs/plans/PLAN-008.1-synthetic-repo-world.md")
	require.Equal(t, 0, direct,
		"direct exact ID artifact should rank first, got %#v", paths)
	require.GreaterOrEqual(t, neighbor, 0,
		"explicit EV-R1 neighbor should be retained, got %#v", paths)
	if historical >= 0 {
		assert.Greater(t, historical, neighbor,
			"historical analog outranked explicit neighbor: %#v", paths)
		assert.Equal(t, PackTierRelated, got[historical].Metadata["pack_tier"],
			"historical analog should be related in exact ID mode, metadata=%#v", got[historical].Metadata)
	}

}

func TestExplainCandidates_ExactPlanIDReportsIntentIDReason(t *testing.T) {
	query := "EV-R1 external validity bridge"
	candidates := []Candidate{{
		Path:   "docs/plans/PLAN-EV-R1-external-validity-bridge.md",
		Kind:   "plan",
		Title:  "PLAN-EV-R1: external validity bridge",
		Status: "next",
		Body:   "Status: next\nImplement the EV-R1 external validity bridge.",
		Metadata: map[string]string{
			"exact_intent_id_reasons": "query exact intent ID EV-R1 matched artifact",
		},
	}}

	reasons := ExplainCandidates(candidates, query)

	require.NotEmpty(t, reasons, "missing exact intent ID reasons")
	assert.True(t, reasonContainsPrefix(reasons[0].Reasons, "exact intent ID:"),
		"missing exact intent ID reason: %#v", reasons)

}

func TestWeightedFilesRetrieverV0_ExactPlanIDModeDoesNotDampenWhenIDAbsent(t *testing.T) {
	query := "EV-R9 external validity bridge"
	candidates := []Candidate{
		{
			Path:  "docs/plans/external-validity-bridge-notes.md",
			Kind:  "plan",
			Title: "External validity bridge notes",
			Body:  "Current external validity bridge notes without the requested artifact.",
		},
		{
			Path:  "docs/plans/PLAN-008.1-synthetic-repo-world.md",
			Kind:  "plan",
			Title: "PLAN-008.1 synthetic repo world",
			Body:  "Historical external validity bridge context.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, query)
	require.NotEmpty(t, got,
		"expected useful results even when the exact ID is absent")

	for _, candidate := range got {
		require.Equal(t, "", candidate.Metadata["exact_intent_id_score"],
			"exact ID mode should not mark candidates when no exact ID exists, got %#v", candidate.Metadata)

	}
}

func TestExplainCandidatesIncludesAuthorityPriorReason(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "docs/adr/0001-billing-source.md",
			Kind:     "decision",
			Subtype:  "adr",
			Status:   "accepted",
			Title:    "Billing Source",
			Body:     "Decision: billing source is authoritative.",
			Metadata: map[string]string{"classifier_model": "adr", "classifier_confidence": "0.920"},
		},
	}

	reasons := ExplainCandidates(candidates, "why billing source decision")
	require.Len(t, reasons, 1,
		"reasons = %#v", reasons)
	require.True(t, reasonContains(reasons[0].Reasons, "authority prior: canonical ADR path"),
		"missing authority reason: %#v", reasons[0].Reasons)

}

func TestExplainCandidatesIncludesClassifierAuthorityPrior(t *testing.T) {
	candidates := []Candidate{
		{
			Path:     "docs/migration-context.md",
			Kind:     "plan",
			Title:    "Roadmap",
			Body:     "Roadmap for API migration.",
			Metadata: map[string]string{"classifier_authority": "working_plan"},
		},
	}

	reasons := ExplainCandidates(candidates, "api migration roadmap")
	require.Len(t, reasons, 1,
		"reasons = %#v", reasons)
	require.True(t, reasonContains(reasons[0].Reasons, "authority prior: classifier working-plan authority"),
		"missing classifier authority reason: %#v", reasons[0].Reasons)

}

func TestAuthorityCuesAreSparse(t *testing.T) {
	candidate := Candidate{
		Status: "accepted",
		Metadata: map[string]string{
			"classifier_authority": "high_decision",
			"artifact_scope":       "bundle",
		},
	}

	cues := AuthorityCues(candidate)
	require.Len(t, cues, 2,
		"cues = %#v", cues)
	assert.Equal(t, "decision authority", cues[0])
	assert.Equal(t, "accepted", cues[1])

}

func TestWeightedFilesRetrieverV0_CollapsesLocalizedMirror(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/en/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
		{
			Path:  "docs/zh/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "architecture design for billing boundary entitlement sync")
	require.False(t, containsCandidatePath(got, "docs/zh/architecture/billing-boundary.md"),
		"localized mirror should be collapsed: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "docs/en/architecture/billing-boundary.md"),
		"missing default-language candidate: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_CollapsesArchiveCurrentVariant(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
		{
			Path:  "docs/archive/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "architecture design for billing boundary entitlement sync")
	require.False(t, containsCandidatePath(got, "docs/archive/architecture/billing-boundary.md"),
		"archive variant should be collapsed: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "docs/architecture/billing-boundary.md"),
		"missing current candidate: %#v", CandidatePaths(got))
	require.Equal(t, "1", got[0].Metadata["variant_collapsed_count"],
		"missing collapsed metadata: %#v", got[0].Metadata)

}

func TestWeightedFilesRetrieverV0_PreservesRequestedArchiveVariant(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "docs/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
		{
			Path:  "docs/archive/architecture/billing-boundary.md",
			Kind:  "design",
			Title: "Billing Boundary",
			Body:  "Architecture design for billing boundary and entitlement sync.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "historical archive docs/archive/architecture/billing-boundary.md")
	require.True(t, containsCandidatePath(got, "docs/archive/architecture/billing-boundary.md"),
		"explicitly requested archive variant should remain visible: %#v", CandidatePaths(got))

}

func TestWeightedFilesRetrieverV0_CollapsesTemplateInstanceVariant(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "docs/product/billing-prd.md",
			Kind:    "requirements",
			Subtype: "prd",
			Title:   "Billing PRD",
			Body:    "Product requirements for billing entitlement sync.",
		},
		{
			Path:    "docs/templates/product/billing-prd.md",
			Kind:    "requirements",
			Subtype: "prd",
			Title:   "Billing PRD Template",
			Body:    "Product requirements template for billing entitlement sync.",
		},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "billing product requirements entitlement sync")
	require.False(t, containsCandidatePath(got, "docs/templates/product/billing-prd.md"),
		"template variant should be collapsed for non-template query: %#v", CandidatePaths(got))
	require.True(t, containsCandidatePath(got, "docs/product/billing-prd.md"),
		"missing concrete PRD instance: %#v", CandidatePaths(got))

}

func TestExtractMarkdownSectionsTracksStructureAndSignals(t *testing.T) {
	body := strings.Join([]string{
		"---",
		"status: accepted",
		"owner = platform",
		"---",
		"# Root",
		"Intro with [design link](docs/design.md).",
		"## Plan",
		"- [ ] Wire the billing worker",
		"### Acceptance Criteria",
		"- Must preserve idempotency",
		"```go",
		"# Not A Heading",
		"```",
	}, "\n")

	sections := extractMarkdownSections(body)
	require.Len(t, sections, 3,
		"sections = %#v", sections)
	assert.Equal(t, "Root > Plan", sections[1].HeadingPath,
		"nested heading path = %q", sections[1].HeadingPath)
	assert.Equal(t, "accepted", sections[1].Frontmatter["status"],
		"status frontmatter not inherited: %#v", sections[1].Frontmatter)
	assert.Equal(t, "platform", sections[1].Frontmatter["owner"],
		"owner frontmatter not inherited: %#v", sections[1].Frontmatter)
	require.Len(t, sections[1].Tasks, 1,
		"tasks = %#v", sections[1].Tasks)
	assert.Contains(t, sections[1].Tasks[0], "billing worker")
	assert.NotEmpty(t, sections[2].AcceptanceCriteria,
		"missing acceptance criteria: %#v", sections[2])
	assert.NotContains(t, sections[2].HeadingPath, "Not A Heading",
		"code fence heading leaked into section path: %#v", sections)
	require.Len(t, sections[0].Links, 1,
		"links = %#v", sections[0].Links)
	assert.Equal(t, "docs/design.md", sections[0].Links[0])

}

func TestPackCandidateSectionsSelectsRelevantLargeSections(t *testing.T) {
	body := strings.Join([]string{
		"# Overview",
		strings.Repeat("general background without requested identifiers\n", 45),
		"# Billing Boundary",
		"The stripe_event_id idempotency rule controls webhook replay protection.",
		"The worker stores stripe_event_id before side effects.",
		"# Appendix",
		strings.Repeat("irrelevant appendix sentence\n", 45),
	}, "\n")
	candidate := Candidate{
		Path:  "docs/rfcs/webhook-replay.md",
		Title: "Webhook Replay",
		Body:  body,
	}

	got := packCandidateSection(candidate, "stripe_event_id idempotency", expandedTerms("stripe_event_id idempotency"))
	assert.Equal(t, "sections", got.Metadata["section_pack_mode"],
		"expected section-packed candidate, metadata=%#v body=%s", got.Metadata, got.Body)
	assert.Contains(t, got.Body, "### Billing Boundary",
		"packed body missing selected heading: %s", got.Body)
	assert.Contains(t, got.Body, "Source: docs/rfcs/webhook-replay.md",
		"packed body missing source citation: %s", got.Body)
	assert.Contains(t, got.Body, "Lines:",
		"packed body missing line citation: %s", got.Body)
	assert.NotContains(t, got.Body, "irrelevant appendix sentence",
		"packed body retained unrelated appendix: %s", got.Body)

}

func TestPackCandidateSectionsFallsBackForShortFiles(t *testing.T) {
	candidate := Candidate{
		Path: "docs/adr/0001.md",
		Body: "# Decision\n\nUse stripe_event_id for idempotency.",
	}

	got := packCandidateSection(candidate, "stripe_event_id idempotency", expandedTerms("stripe_event_id idempotency"))
	if got.Metadata != nil {
		assert.Empty(t, got.Metadata["section_pack_mode"],
			"short file should not be section packed: %#v", got.Metadata)
	}
	assert.Equal(t, candidate.Body, got.Body,
		"body changed for short file")

}

func reasonContains(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func reasonContainsPrefix(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, want) {
			return true
		}
	}
	return false
}

func containsCandidatePath(candidates []Candidate, path string) bool {
	return candidatePathIndex(candidates, path) >= 0
}

func candidatePathIndex(candidates []Candidate, path string) int {
	for i, c := range candidates {
		if c.Path == path {
			return i
		}
	}
	return -1
}

func reasonPathIndex(reasons []Reason, path string) int {
	for index, reason := range reasons {
		if reason.Path == path {
			return index
		}
	}
	return -1
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
