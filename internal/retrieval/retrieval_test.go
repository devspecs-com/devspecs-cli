package retrieval

import (
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
