package retrieval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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
	if retriever.Name() != "eval_weighted_files_v0" {
		t.Fatalf("retriever name = %q", retriever.Name())
	}
	if len(got) != 1 {
		t.Fatalf("retrieved %d candidates, want 1: %#v", len(got), got)
	}
	if got[0].Path != "openspec/changes/harden-entitlement-sync/design.md" {
		t.Fatalf("retrieved path = %q", got[0].Path)
	}

	reasons := ExplainCandidates(got, "stripe_event_id idempotency")
	if len(reasons) != 1 || reasons[0].Path != got[0].Path || len(reasons[0].Reasons) == 0 {
		t.Fatalf("missing reasons: %#v", reasons)
	}
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
	if !containsCandidatePath(got, "docs/plans/broad.md") {
		t.Fatalf("missing section-selected artifact: %#v", CandidatePaths(got))
	}
	if got[0].Metadata["indexed_section_retrieval_mode"] != "section_aware" {
		t.Fatalf("expected indexed section match metadata, got %#v", got[0].Metadata)
	}
	reasons := ExplainCandidates(got, "stripe_event_id idempotency")
	if len(reasons) == 0 || !strings.Contains(strings.Join(reasons[0].Reasons, "\n"), "indexed section match") {
		t.Fatalf("expected indexed section reason, got %#v", reasons)
	}
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
	if retriever.Name() != "eval_weighted_files_v0_evidence_balanced" {
		t.Fatalf("retriever name = %q", retriever.Name())
	}
	if len(got) == 0 || got[0].Path != "docs/plans/entitlement-sync-rollout.md" {
		t.Fatalf("anchored plan should rank first, got %#v", CandidatePaths(got))
	}
	if got[0].Metadata["retrieval_evidence_mode"] != EvidenceModeBalanced {
		t.Fatalf("missing balanced evidence metadata: %#v", got[0].Metadata)
	}
}

func TestWeightedFilesRetrieverV0_UsesAttachedSectionsForPackingAndAblationDisablesThem(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "plan_broad",
			Path:  "docs/plans/broad.md",
			Title: "Broad Plan",
			Kind:  "plan",
			Body:  "# Broad Plan\n\n" + strings.Repeat("general implementation background.\n", 140) + "\nstripe_event_id idempotency protects webhook replay behavior.\n",
			Sections: []IndexedSection{
				{
					ID:           "sec_replay",
					ArtifactID:   "plan_broad",
					SourcePath:   "docs/plans/broad.md",
					HeadingPath:  "Requirements > Replay Boundary",
					Title:        "Replay Boundary",
					StartLine:    22,
					EndLine:      40,
					Body:         "stripe_event_id idempotency protects webhook replay behavior.",
					HeadingDepth: 2,
				},
			},
		},
		{ID: "unrelated", Path: "docs/plans/unrelated.md", Body: "general implementation background"},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "stripe_event_id idempotency")
	if !containsCandidatePath(got, "docs/plans/broad.md") {
		t.Fatalf("missing section-selected artifact: %#v", CandidatePaths(got))
	}
	if got[0].Metadata["indexed_section_match_source"] != "candidate_sections" {
		t.Fatalf("expected attached section metadata, got %#v", got[0].Metadata)
	}

	disabled := (WeightedFilesRetrieverV0{DisableSectionAware: true}).Retrieve(candidates, "stripe_event_id idempotency")
	if !containsCandidatePath(disabled, "docs/plans/broad.md") {
		t.Fatalf("ablation should keep the file-level match: %#v", CandidatePaths(disabled))
	}
	if disabled[0].Metadata != nil && disabled[0].Metadata["indexed_section_retrieval_mode"] == "section_aware" {
		t.Fatalf("section-aware ablation should not annotate section evidence: %#v", disabled[0].Metadata)
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
	if got[0].Metadata != nil && got[0].Metadata["indexed_section_retrieval_mode"] == "section_aware" {
		t.Fatalf("generic body-only section should not be selected: %#v", got[0].Metadata)
	}
}

func TestEnrichCandidatesWithSectionMatchesDoesNotRescueRoadmapWithoutRoadmapIntent(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "roadmap",
			Path:  "ROADMAP.md",
			Title: "AReaL Roadmap",
			Kind:  "plan",
			Body:  "# AReaL Roadmap\n\nGeneral future planning.",
			Sections: []IndexedSection{
				{
					ID:          "sec_roadmap",
					ArtifactID:  "roadmap",
					SourcePath:  "ROADMAP.md",
					HeadingPath: "AReaL Roadmap",
					Title:       "AReaL Roadmap",
					StartLine:   1,
					EndLine:     9,
					Body:        "AReaL project timeline and future planning notes.",
				},
			},
		},
	}

	got := EnrichCandidatesWithSectionMatches(candidates, "repository agent operating instructions and contributor guidance for AReaL")
	if got[0].Metadata != nil && got[0].Metadata["indexed_section_retrieval_mode"] == "section_aware" {
		t.Fatalf("roadmap should not be section-rescued without roadmap intent: %#v", got[0].Metadata)
	}

	roadmap := EnrichCandidatesWithSectionMatches(candidates, "AReaL roadmap future work and milestones")
	if roadmap[0].Metadata == nil || roadmap[0].Metadata["indexed_section_retrieval_mode"] != "section_aware" {
		t.Fatalf("roadmap intent should allow section evidence: %#v", roadmap[0].Metadata)
	}
}

func mustJSONList(t *testing.T, values []string) string {
	t.Helper()
	b, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
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
	if len(paths) != 1 || paths[0] != "docs/adr/0002-webhook-idempotency-boundary.md" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestWeightedFilesRetrieverV0_UsesCandidateTitle(t *testing.T) {
	candidates := []Candidate{
		{Path: "plan.md", Title: "Golden Plan", Body: "Short body."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "Golden")
	if len(got) != 1 {
		t.Fatalf("retrieved %d candidates, want 1", len(got))
	}
	reasons := ExplainCandidates(got, "Golden")
	if len(reasons) == 0 || len(reasons[0].Reasons) == 0 || reasons[0].Reasons[0] != "query term match in title: golden" {
		t.Fatalf("reasons = %#v", reasons)
	}
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
	if !containsCandidatePath(got, "services/api/src/auth/session.ts") {
		t.Fatalf("missing session source file: %#v", CandidatePaths(got))
	}
	if !containsCandidatePath(got, "services/api/src/billing/entitlements.ts") {
		t.Fatalf("missing entitlements source file: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "docs/plans/billing-ops-runbook.md") {
		t.Fatalf("broad runbook should not outrank exact source matches: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "docs/rfcs/0008-billing-webhook-replay-protection.md") {
		t.Fatalf("missing RFC candidate: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "docs/rfcs/0009-support-search-ranking.md") {
		t.Fatalf("unrelated RFC should not be selected: %#v", CandidatePaths(got))
	}
}

func TestWeightedFilesRetrieverV0_GenericPlanNeedsCoreEvidence(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/plans/2026-05-01-entitlement-sync-plan.md", Body: "Current progress for entitlement_sync hardening and billing-webhook-hardening."},
		{Path: "docs/plans/generic-implementation-plan.md", Body: "Current progress next steps implementation notes without the requested feature words."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume entitlement sync hardening")
	if !containsCandidatePath(got, "docs/plans/2026-05-01-entitlement-sync-plan.md") {
		t.Fatalf("missing specific plan: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "docs/plans/generic-implementation-plan.md") {
		t.Fatalf("generic plan should not pass without core evidence: %#v", CandidatePaths(got))
	}
}

func TestWeightedFilesRetrieverV0_DemotesNonIntentLanesUnlessRequested(t *testing.T) {
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
	if !containsCandidatePath(got, "docs/plans/auth-token-rollout.md") {
		t.Fatalf("missing plan: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "CLAUDE.md") {
		t.Fatalf("protocol instructions should not appear in ordinary plan retrieval: %#v", CandidatePaths(got))
	}

	got = (WeightedFilesRetrieverV0{}).Retrieve(candidates, "claude instructions auth token rollout")
	if !containsCandidatePath(got, "CLAUDE.md") {
		t.Fatalf("missing explicitly requested instructions: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "services/billing/webhook_test.go#L12") {
		t.Fatalf("missing test-case candidate: %#v", CandidatePaths(got))
	}
	reasons := ExplainCandidates(got, "what regression tests protect stripe_event_id idempotency?")
	var found bool
	for _, reason := range reasons {
		if reason.Path != "services/billing/webhook_test.go#L12" {
			continue
		}
		found = true
		if !reasonContains(reason.Reasons, "test-case behavior signal") {
			t.Fatalf("missing test behavior reason: %#v", reason.Reasons)
		}
	}
	if !found {
		t.Fatalf("missing reasons for test candidate: %#v", reasons)
	}
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
	if len(got) == 0 || got[0].Path != "internal/tools/tool_test.go#L42" {
		t.Fatalf("expected exact test-name anchor first, got %#v", CandidatePaths(got))
	}
	reasons := ExplainCandidates(got, "what tests cover TestPutAndGetExposedTool behavior?")
	if len(reasons) == 0 || !reasonContains(reasons[0].Reasons, "exact test-name anchor") {
		t.Fatalf("missing exact test-name reason: %#v", reasons)
	}
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
	if len(got) == 0 || got[0].Path != "internal/tools/tool_test.go#L42" {
		t.Fatalf("expected token test-name anchor first, got %#v", CandidatePaths(got))
	}
	reasons := ExplainCandidates(got, "what tests cover put and get exposed tool behavior?")
	if len(reasons) == 0 || !reasonContains(reasons[0].Reasons, "test-name token anchor") {
		t.Fatalf("missing token test-name reason: %#v", reasons)
	}
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
	if len(got) == 0 || got[0].Path != "tests/tools_test.py#L12" {
		t.Fatalf("expected snake-case test-name anchor first, got %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "docs/roadmap.md") {
		t.Fatalf("missing roadmap: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "tests/voice_test.py#L20") {
		t.Fatalf("test case should not appear in ordinary roadmap retrieval: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "docs/plans/interactive-mode.md") {
		t.Fatalf("missing planning doc: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "packages/coding-agent/test/interactive-mode-plan-review.test.ts") {
		t.Fatalf("raw test file should not appear in non-test planning retrieval: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "services/billing/webhook_test.go#L12") {
		t.Fatalf("missing precise test unit: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "services/billing/webhook_test.go") {
		t.Fatalf("raw test file should be suppressed when unit-level artifact exists: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "processing/src/test/java/org/example/QuerySegmentSpecTest.java#L39") {
		t.Fatalf("missing test unit: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "processing/src/main/java/org/example/LegacyParser.java#L20") {
		t.Fatalf("legacy inside test name should not request code comments: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "docs/product-specs/course-visit-analytics.md") {
		t.Fatalf("missing product spec: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "src/app/courseVisitTracking.test.ts#L11") {
		t.Fatalf("analytics domain word should not route tests without test intent: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "services/billing/webhook.go#L18") {
		t.Fatalf("missing code-comment candidate: %#v", CandidatePaths(got))
	}
	reasons := ExplainCandidates(got, "implementation rationale for stripe_event_id invariant in code")
	var found bool
	for _, reason := range reasons {
		if reason.Path == "services/billing/webhook.go#L18" {
			found = true
			if !reasonContains(reason.Reasons, "code-comment rationale signal") {
				t.Fatalf("missing code-comment reason: %#v", reason.Reasons)
			}
		}
	}
	if !found {
		t.Fatalf("missing reasons for code-comment candidate: %#v", reasons)
	}
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
	if !containsCandidatePath(got, "docs/design-docs/langfuse-trace-association.md") {
		t.Fatalf("missing anchored design doc: %#v", CandidatePaths(got))
	}
	for _, unwanted := range []string{
		"AGENTS.md",
		"docs/billing-subscription-design.md",
		"docs/shared-admin-table-component.md",
		"docs/engineering-baseline.md",
	} {
		if containsCandidatePath(got, unwanted) {
			t.Fatalf("%s should not backfill body-only retrieval: %#v", unwanted, CandidatePaths(got))
		}
	}
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
	if !containsCandidatePath(got, "docs/src/architecture.md") {
		t.Fatalf("missing architecture doc from small candidate set: %#v", CandidatePaths(got))
	}
}

func TestWeightedFilesRetrieverV0_ResumeIntentKeepsMatchingDecisionContext(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/plans/2026-05-01-entitlement-sync-plan.md", Body: "Current progress for entitlement_sync hardening and billing-webhook-hardening."},
		{Path: "docs/adr/0002-webhook-idempotency-boundary.md", Body: "Decision: billing-webhook-hardening uses entitlement_sync after durable webhook idempotency."},
		{Path: "docs/prd/billing-entitlements-v1.md", Body: "Product requirements mention entitlement_sync, entitlements, customers, access, and billing."},
		{Path: "services/api/src/billing/entitlements.ts", Body: "function entitlement_sync() { return billingWebhookHardening(); }"},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "resume entitlement sync hardening")
	if !containsCandidatePath(got, "docs/adr/0002-webhook-idempotency-boundary.md") {
		t.Fatalf("missing matching ADR decision context: %#v", CandidatePaths(got))
	}
}

func TestWeightedFilesRetrieverV0_LifecycleIntentPrefersStaleDecision(t *testing.T) {
	candidates := []Candidate{
		{Path: "docs/adr/0003-superseded-local-entitlements.md", Status: "superseded", Body: "The local entitlement caching plan was abandoned."},
		{Path: "docs/plans/active-entitlement-rollout.md", Status: "active", Body: "Mentions local entitlement caching as old context but tracks current rollout."},
		{Path: ".claude/notes/local-entitlements-experiment.md", Status: "stale", Body: "Historical local entitlement cache experiment."},
	}

	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, "continue local entitlement caching plan")
	if !containsCandidatePath(got, "docs/adr/0003-superseded-local-entitlements.md") {
		t.Fatalf("missing superseded ADR: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "docs/plans/active-entitlement-rollout.md") {
		t.Fatalf("active rollout should not beat lifecycle candidates: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "openspec/changes/add-sso/tasks.md") {
		t.Fatalf("missing tasks child: %#v", CandidatePaths(got))
	}
	if !containsCandidatePath(got, "openspec/changes/add-sso/design.md") {
		t.Fatalf("missing expanded design companion: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "openspec/changes/add-sso") {
		t.Fatalf("structural parent bundle should not be included for ordinary task retrieval: %#v", CandidatePaths(got))
	}
	for _, candidate := range got {
		if candidate.Path == "openspec/changes/add-sso/design.md" && candidate.Metadata["retrieval_expansion_reason"] != "openspec_companion" {
			t.Fatalf("companion expansion reason = %#v", candidate.Metadata)
		}
	}
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
	if !containsCandidatePath(got, "openspec/changes/add-sso") {
		t.Fatalf("missing explicit OpenSpec parent bundle: %#v", CandidatePaths(got))
	}
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
	for _, want := range []string{
		"docs/prd/billing-entitlements-v1.md",
		"docs/adr/0001-use-stripe-as-billing-source.md",
		"docs/adr/0002-webhook-idempotency-boundary.md",
	} {
		if !containsCandidatePath(got, want) {
			t.Fatalf("missing %s: %#v", want, CandidatePaths(got))
		}
	}
	for _, unwanted := range []string{
		"docs/prd/billing-analytics-v1.md",
		"docs/prd/customer-portal-v2.md",
		"docs/adr/0004-admin-billing-overrides.md",
		"services/api/src/billing/entitlements.ts",
		"docs/plans/customer-access-notes.md",
	} {
		if containsCandidatePath(got, unwanted) {
			t.Fatalf("%s should not be selected for product background: %#v", unwanted, CandidatePaths(got))
		}
	}
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
	if !containsCandidatePath(got, "agents/prd.agent.md") {
		t.Fatalf("missing PRD agent via product requirements document bridge: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "skills/reference/documentation-full.md") {
		t.Fatalf("generic documentation should not beat PRD path/title match: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "docs/docs/en/architecture/design.md") {
		t.Fatalf("missing classified architecture design doc: %#v", CandidatePaths(got))
	}
	for _, unwanted := range []string{"CLAUDE.md", "module/CLAUDE.md"} {
		if containsCandidatePath(got, unwanted) {
			t.Fatalf("%s should not appear for non-protocol design query: %#v", unwanted, CandidatePaths(got))
		}
	}
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
	if !containsCandidatePath(got, "CLAUDE.md") {
		t.Fatalf("missing shallow repository instructions: %#v", CandidatePaths(got))
	}
	if containsCandidatePath(got, "service/CLAUDE.md") {
		t.Fatalf("nested instructions should not backfill repository-wide instruction query: %#v", CandidatePaths(got))
	}
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
	if score < 4.0 {
		t.Fatalf("roadmap score = %.2f, want retrievable", score)
	}
	got := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)
	if !containsCandidatePath(got, "docs/roadmap.md") {
		t.Fatalf("missing roadmap path signal: %#v", CandidatePaths(got))
	}
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
	if len(got) != 0 {
		t.Fatalf("authority prior should not rescue unrelated canonical docs: %#v", CandidatePaths(got))
	}
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
	if len(ranks) == 0 || ranks[0].Path != "pkg/tools/exposed_tool_test.go#L24-L39" {
		t.Fatalf("compact test identifier should rank first: %#v", ranks)
	}
	if !containsString(ranks[0].MatchedCompacts, "testputandgetexposedtool") {
		t.Fatalf("missing compact diagnostic: %#v", ranks[0])
	}
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
	if !containsCandidatePath(got, "docs/product-specs/fluxnova-aigf.md") {
		t.Fatalf("missing specific product requirements backfill: %#v", CandidatePaths(got))
	}
	for _, c := range got {
		if c.Path == "docs/product-specs/fluxnova-aigf.md" && c.Metadata["concept_backfill_score"] == "" {
			t.Fatalf("missing concept backfill metadata: %#v", c.Metadata)
		}
	}
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
	if len(got) != 0 {
		t.Fatalf("broad query should not backfill template noise: %#v", CandidatePaths(got))
	}
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
	if len(got) != 0 {
		t.Fatalf("glossary should suppress broad repo concept backfill: %#v", CandidatePaths(got))
	}
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
	if !containsCandidatePath(got, "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md") {
		t.Fatalf("glossary should preserve rare product concept backfill: %#v", CandidatePaths(got))
	}
	for _, c := range got {
		if c.Path == "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md" && c.Metadata["concept_glossary_enabled"] != "true" {
			t.Fatalf("missing glossary metadata: %#v", c.Metadata)
		}
	}
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
	if !containsCandidatePath(got, "pkg/tools/exposed_tool_test.go#L24-L39") {
		t.Fatalf("glossary should preserve exact test-name concept: %#v", CandidatePaths(got))
	}
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
	if tier != PackTierPrimary {
		t.Fatalf("exact test-name concept should stay primary, tier=%q reason=%q", tier, reason)
	}
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
	for _, c := range got {
		if c.Path != "docs/plans/billing-entitlement-rollout.md" {
			continue
		}
		if c.Metadata["pack_tier"] != PackTierRelated {
			t.Fatalf("plausible non-requested concept should be related, metadata=%#v", c.Metadata)
		}
		return
	}
	t.Fatalf("missing tiered concept backfill: %#v", CandidatePaths(got))
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
	if canonicalScore <= archivedScore {
		t.Fatalf("canonical score %.2f should beat archived score %.2f", canonicalScore, archivedScore)
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
	if len(reasons) != 1 {
		t.Fatalf("reasons = %#v", reasons)
	}
	if !reasonContains(reasons[0].Reasons, "authority prior: canonical ADR path") {
		t.Fatalf("missing authority reason: %#v", reasons[0].Reasons)
	}
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
	if len(reasons) != 1 {
		t.Fatalf("reasons = %#v", reasons)
	}
	if !reasonContains(reasons[0].Reasons, "authority prior: classifier working-plan authority") {
		t.Fatalf("missing classifier authority reason: %#v", reasons[0].Reasons)
	}
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
	if len(cues) != 2 {
		t.Fatalf("cues = %#v", cues)
	}
	if cues[0] != "decision authority" || cues[1] != "accepted" {
		t.Fatalf("cues = %#v", cues)
	}
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
	if containsCandidatePath(got, "docs/zh/architecture/billing-boundary.md") {
		t.Fatalf("localized mirror should be collapsed: %#v", CandidatePaths(got))
	}
	if !containsCandidatePath(got, "docs/en/architecture/billing-boundary.md") {
		t.Fatalf("missing default-language candidate: %#v", CandidatePaths(got))
	}
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
	if containsCandidatePath(got, "docs/archive/architecture/billing-boundary.md") {
		t.Fatalf("archive variant should be collapsed: %#v", CandidatePaths(got))
	}
	if !containsCandidatePath(got, "docs/architecture/billing-boundary.md") {
		t.Fatalf("missing current candidate: %#v", CandidatePaths(got))
	}
	if got[0].Metadata["variant_collapsed_count"] != "1" {
		t.Fatalf("missing collapsed metadata: %#v", got[0].Metadata)
	}
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
	if !containsCandidatePath(got, "docs/archive/architecture/billing-boundary.md") {
		t.Fatalf("explicitly requested archive variant should remain visible: %#v", CandidatePaths(got))
	}
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
	if containsCandidatePath(got, "docs/templates/product/billing-prd.md") {
		t.Fatalf("template variant should be collapsed for non-template query: %#v", CandidatePaths(got))
	}
	if !containsCandidatePath(got, "docs/product/billing-prd.md") {
		t.Fatalf("missing concrete PRD instance: %#v", CandidatePaths(got))
	}
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
	if len(sections) != 3 {
		t.Fatalf("sections = %#v", sections)
	}
	if sections[1].HeadingPath != "Root > Plan" {
		t.Fatalf("nested heading path = %q", sections[1].HeadingPath)
	}
	if sections[1].Frontmatter["status"] != "accepted" || sections[1].Frontmatter["owner"] != "platform" {
		t.Fatalf("frontmatter not inherited: %#v", sections[1].Frontmatter)
	}
	if len(sections[1].Tasks) != 1 || !strings.Contains(sections[1].Tasks[0], "billing worker") {
		t.Fatalf("tasks = %#v", sections[1].Tasks)
	}
	if len(sections[2].AcceptanceCriteria) == 0 {
		t.Fatalf("missing acceptance criteria: %#v", sections[2])
	}
	if strings.Contains(sections[2].HeadingPath, "Not A Heading") {
		t.Fatalf("code fence heading leaked into section path: %#v", sections)
	}
	if len(sections[0].Links) != 1 || sections[0].Links[0] != "docs/design.md" {
		t.Fatalf("links = %#v", sections[0].Links)
	}
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
	if got.Metadata["section_pack_mode"] != "sections" {
		t.Fatalf("expected section-packed candidate, metadata=%#v body=%s", got.Metadata, got.Body)
	}
	if !strings.Contains(got.Body, "### Billing Boundary") {
		t.Fatalf("packed body missing selected heading: %s", got.Body)
	}
	if !strings.Contains(got.Body, "Source: docs/rfcs/webhook-replay.md") || !strings.Contains(got.Body, "Lines:") {
		t.Fatalf("packed body missing source/line citation: %s", got.Body)
	}
	if strings.Contains(got.Body, "irrelevant appendix sentence") {
		t.Fatalf("packed body retained unrelated appendix: %s", got.Body)
	}
}

func TestPackCandidateSectionsFallsBackForShortFiles(t *testing.T) {
	candidate := Candidate{
		Path: "docs/adr/0001.md",
		Body: "# Decision\n\nUse stripe_event_id for idempotency.",
	}

	got := packCandidateSection(candidate, "stripe_event_id idempotency", expandedTerms("stripe_event_id idempotency"))
	if got.Metadata != nil && got.Metadata["section_pack_mode"] != "" {
		t.Fatalf("short file should not be section packed: %#v", got.Metadata)
	}
	if got.Body != candidate.Body {
		t.Fatalf("body changed for short file")
	}
}

func reasonContains(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func containsCandidatePath(candidates []Candidate, path string) bool {
	for _, c := range candidates {
		if c.Path == path {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
