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
