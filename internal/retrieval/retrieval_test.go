package retrieval

import "testing"

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

func containsCandidatePath(candidates []Candidate, path string) bool {
	for _, c := range candidates {
		if c.Path == path {
			return true
		}
	}
	return false
}
