package scan

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/gitfacts"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestWorkstreamEvidence_TaskIDConnectsPlanAndChangedSource(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "plan",
			title:   "Token Refresh Plan",
			body:    "Track DEV-123 for the refresh token implementation.",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "plans/token-refresh.md", SourceIdentity: "plans/token-refresh.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "RefreshTokenService",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/auth/refresh.go", SourceIdentity: "internal/auth/refresh.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"plans/token-refresh.md": {
			{id: "art_plan", kind: "markdown_artifact", subtype: "plan", title: "Token Refresh Plan", path: "plans/token-refresh.md"},
		},
		"internal/auth/refresh.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "RefreshTokenService", path: "internal/auth/refresh.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["plans/token-refresh.md"][0],
		"art_source": byPath["internal/auth/refresh.go"][0],
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "abcdef1234567890",
			Message:      "DEV-123 implement refresh token service",
			CommittedAt:  "2026-05-26T10:00:00Z",
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files: []gitfacts.FileChange{{
			CommitSHA: "abcdef1234567890",
			FilePath:  "internal/auth/refresh.go",
		}},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull, Branch: "feature/DEV-123-token-refresh"},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	if built.diagnostics.AnchorsMaterialized == 0 {
		t.Fatalf("expected materialized anchors, got diagnostics %#v", built.diagnostics)
	}
	if len(built.edges) == 0 {
		t.Fatalf("expected same_workstream_anchor edge")
	}
	edge := built.edges[0]
	if edge.EdgeType != edgeTypeSameWorkstreamAnchor {
		t.Fatalf("edge type: got %q", edge.EdgeType)
	}
	if edge.SrcArtifactID != "art_plan" || edge.DstArtifactID != "art_source" {
		t.Fatalf("edge endpoints: got %s -> %s", edge.SrcArtifactID, edge.DstArtifactID)
	}
	if edge.Confidence < 0.9 {
		t.Fatalf("expected high-confidence task edge, got %.3f", edge.Confidence)
	}
	if len(built.diagnostics.TopClusters) == 0 || built.diagnostics.TopClusters[0].PackStrength != workstreamPackStrengthStrong {
		t.Fatalf("expected strong pack candidate, got %#v", built.diagnostics.TopClusters)
	}
	if built.diagnostics.TopClusters[0].Dialect != workstreamDialectTicketLikeUpper {
		t.Fatalf("expected ticket-like dialect, got %#v", built.diagnostics.TopClusters[0])
	}
}

func TestWorkstreamEvidence_SlugWindowsConnectPlanAndChangedSource(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "plan",
			title:   "Priority 3.2 Workstream Evidence Clustering Plan",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "docs/2026-05-26-priority-3-2-workstream-evidence-clustering-plan.md", SourceIdentity: "docs/2026-05-26-priority-3-2-workstream-evidence-clustering-plan.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "workstreamEvidence",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/scan/workstream_evidence.go", SourceIdentity: "internal/scan/workstream_evidence.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"docs/2026-05-26-priority-3-2-workstream-evidence-clustering-plan.md": {
			{id: "art_plan", kind: "markdown_artifact", subtype: "plan", title: "Priority 3.2 Workstream Evidence Clustering Plan", path: "docs/2026-05-26-priority-3-2-workstream-evidence-clustering-plan.md"},
		},
		"internal/scan/workstream_evidence.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "workstreamEvidence", path: "internal/scan/workstream_evidence.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["docs/2026-05-26-priority-3-2-workstream-evidence-clustering-plan.md"][0],
		"art_source": byPath["internal/scan/workstream_evidence.go"][0],
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "123456abcdef7890",
			Message:      "docs: plan priority 3.2 workstream evidence",
			CommittedAt:  "2026-05-26T11:00:00Z",
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files: []gitfacts.FileChange{{
			CommitSHA: "123456abcdef7890",
			FilePath:  "internal/scan/workstream_evidence.go",
		}},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	found := false
	for _, edge := range built.edges {
		if edge.EdgeType == edgeTypeSameWorkstreamAnchor && edge.SrcArtifactID == "art_plan" && edge.DstArtifactID == "art_source" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected slug-window workstream edge, got %#v", built.diagnostics.TopClusters)
	}
}

func TestWorkstreamEvidence_SourceTestOnlySlugIsLocalityOnly(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "Entity Relation",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/entity_relation.go", SourceIdentity: "internal/entity_relation.go"}},
		},
		{
			id:      "art_test",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "test_case",
			title:   "TestEntityRelation",
			sources: []store.SourceRow{{ArtifactID: "art_test", SourceType: "source_context", Path: "internal/entity_relation_test.go", SourceIdentity: "internal/entity_relation_test.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"internal/entity_relation.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Entity Relation", path: "internal/entity_relation.go"},
		},
		"internal/entity_relation_test.go": {
			{id: "art_test", kind: "source_context", subtype: "test_case", title: "TestEntityRelation", path: "internal/entity_relation_test.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_source": byPath["internal/entity_relation.go"][0],
		"art_test":   byPath["internal/entity_relation_test.go"][0],
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "abcdef1234567890",
			Message:      "test entity relation handling",
			CommittedAt:  "2026-05-26T12:00:00Z",
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files: []gitfacts.FileChange{
			{CommitSHA: "abcdef1234567890", FilePath: "internal/entity_relation.go"},
			{CommitSHA: "abcdef1234567890", FilePath: "internal/entity_relation_test.go"},
		},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	if len(built.edges) != 0 {
		t.Fatalf("expected source/test-only locality to avoid edge materialization, got %#v", built.edges)
	}
	if len(built.diagnostics.TopClusters) == 0 || built.diagnostics.TopClusters[0].PackStrength != workstreamPackStrengthSupportLocal {
		t.Fatalf("expected locality-support pack cluster, got %#v", built.diagnostics.TopClusters)
	}
}

func TestWorkstreamEvidence_PlanSourceSlugIsCrossRoleSupport(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "plan",
			title:   "Billing Entitlements",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "docs/billing-entitlements.md", SourceIdentity: "docs/billing-entitlements.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "Billing Entitlements",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/billing/entitlements.go", SourceIdentity: "internal/billing/entitlements.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"docs/billing-entitlements.md": {
			{id: "art_plan", kind: "plan", title: "Billing Entitlements", path: "docs/billing-entitlements.md"},
		},
		"internal/billing/entitlements.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Billing Entitlements", path: "internal/billing/entitlements.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["docs/billing-entitlements.md"][0],
		"art_source": byPath["internal/billing/entitlements.go"][0],
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, gitfacts.Facts{})
	if len(built.diagnostics.TopClusters) == 0 || built.diagnostics.TopClusters[0].PackStrength != workstreamPackStrengthSupportCross {
		t.Fatalf("expected cross-role support pack cluster, got %#v", built.diagnostics.TopClusters)
	}
	if len(built.edges) == 0 {
		t.Fatalf("expected cross-role support edge")
	}
}

func TestWorkstreamEvidence_CappedClusterPreservesImplementationRepresentative(t *testing.T) {
	var artifacts []evidenceArtifact
	byPath := map[string][]gitArtifactRef{}
	byID := map[string]gitArtifactRef{}
	for i := 0; i < maxWorkstreamArtifactsPerAnchor+3; i++ {
		id := "art_spec_" + string(rune('a'+i))
		path := "docs/billing-entitlements/spec-" + string(rune('a'+i)) + ".md"
		ref := gitArtifactRef{id: id, kind: "spec", subtype: "openspec_child", title: "Billing Entitlements", path: path}
		artifacts = append(artifacts, evidenceArtifact{
			id:      id,
			repoID:  "repo",
			kind:    "spec",
			subtype: "openspec_child",
			title:   "Billing Entitlements",
			sources: []store.SourceRow{{ArtifactID: id, SourceType: "markdown", Path: path, SourceIdentity: path}},
		})
		byPath[path] = []gitArtifactRef{ref}
		byID[id] = ref
	}
	sourceRef := gitArtifactRef{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Billing Entitlements", path: "internal/billing/entitlements.go"}
	artifacts = append(artifacts, evidenceArtifact{
		id:      "art_source",
		repoID:  "repo",
		kind:    "source_context",
		subtype: "code_comment",
		title:   "Billing Entitlements",
		sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/billing/entitlements.go", SourceIdentity: "internal/billing/entitlements.go"}},
	})
	byPath["internal/billing/entitlements.go"] = []gitArtifactRef{sourceRef}
	byID["art_source"] = sourceRef
	for i := 0; i < 30; i++ {
		id := "art_unrelated_" + string(rune('a'+i))
		path := "docs/notes-" + string(rune('a'+i)) + ".md"
		ref := gitArtifactRef{id: id, kind: "markdown_artifact", title: "Notes", path: path}
		artifacts = append(artifacts, evidenceArtifact{
			id:      id,
			repoID:  "repo",
			kind:    "markdown_artifact",
			title:   "Notes",
			sources: []store.SourceRow{{ArtifactID: id, SourceType: "markdown", Path: path, SourceIdentity: path}},
		})
		byPath[path] = []gitArtifactRef{ref}
		byID[id] = ref
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "fedcba1234567890",
			Message:      "implement billing entitlements",
			CommittedAt:  "2026-05-26T13:00:00Z",
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files:       []gitfacts.FileChange{{CommitSHA: "fedcba1234567890", FilePath: "internal/billing/entitlements.go"}},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	for _, cluster := range built.diagnostics.TopClusters {
		if cluster.Anchor != "billing-entitlements" {
			continue
		}
		if cluster.PackStrength != workstreamPackStrengthStrong {
			t.Fatalf("expected capped doc/source cluster to stay strong, got %#v", cluster)
		}
		if cluster.RoleFamilyMix["source"] == 0 {
			t.Fatalf("expected capped cluster to keep source representative, got %#v", cluster)
		}
		return
	}
	t.Fatalf("expected billing-entitlements cluster, got %#v", built.diagnostics.TopClusters)
}

func TestWorkstreamEvidence_RejectsDateLikeBareGithubRef(t *testing.T) {
	extracted := extractFormalWorkstreamAnchors("See #2026 for the annual plan.", "body")
	if len(extracted.anchors) != 0 {
		t.Fatalf("expected no anchors, got %#v", extracted.anchors)
	}
	if len(extracted.rejected) == 0 || extracted.rejected[0].reason != "date_like_number" {
		t.Fatalf("expected date_like_number rejection, got %#v", extracted.rejected)
	}
}

func TestWorkstreamEvidence_ExplicitWorkRefDialects(t *testing.T) {
	extracted := extractFormalWorkstreamAnchors("See PR-42, pull/43, ISSUE-44, issues/45, GH-46, #47, ADR-001, GPT-2, and LM-19.", "body")
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}
	want := map[string]string{
		"pr-42":    workstreamDialectExplicitPRRef,
		"pr-43":    workstreamDialectExplicitPRRef,
		"issue-44": workstreamDialectExplicitIssueRef,
		"issue-45": workstreamDialectExplicitIssueRef,
		"gh-46":    workstreamDialectExplicitGHRef,
		"gh-47":    workstreamDialectBareHashRef,
		"ADR-001":  workstreamDialectDocumentNumberRef,
		"GPT-2":    workstreamDialectGenericTechnical,
		"LM-19":    workstreamDialectTicketLikeUpper,
	}
	for canonical, dialect := range want {
		if got[canonical] != dialect {
			t.Fatalf("dialect for %s: got %q want %q from %#v", canonical, got[canonical], dialect, extracted.anchors)
		}
	}
}

func TestWorkstreamEvidence_OpenSpecChangeSlugStaysStrong(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:        "art_change",
			repoID:    "repo",
			kind:      "spec",
			subtype:   "openspec_child",
			title:     "Add Token Refresh",
			extracted: map[string]any{"openspec_change_id": "add-token-refresh"},
			sources:   []store.SourceRow{{ArtifactID: "art_change", SourceType: "markdown", Path: "openspec/changes/add-token-refresh/specs/auth/spec.md", SourceIdentity: "openspec/changes/add-token-refresh/specs/auth/spec.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "Add Token Refresh",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/auth/add_token_refresh.go", SourceIdentity: "internal/auth/add_token_refresh.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"openspec/changes/add-token-refresh/specs/auth/spec.md": {
			{id: "art_change", kind: "spec", subtype: "openspec_child", title: "Add Token Refresh", path: "openspec/changes/add-token-refresh/specs/auth/spec.md"},
		},
		"internal/auth/add_token_refresh.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Add Token Refresh", path: "internal/auth/add_token_refresh.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_change": byPath["openspec/changes/add-token-refresh/specs/auth/spec.md"][0],
		"art_source": byPath["internal/auth/add_token_refresh.go"][0],
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, gitfacts.Facts{})
	cluster := findWorkstreamTestCluster(t, built.diagnostics.TopClusters, "token-refresh")
	if cluster.Dialect != workstreamDialectOpenSpecChangeSlug {
		t.Fatalf("expected OpenSpec change dialect, got %#v", cluster)
	}
	if cluster.PackStrength != workstreamPackStrengthStrong {
		t.Fatalf("expected OpenSpec change slug to remain strong, got %#v", cluster)
	}
}

func TestWorkstreamEvidence_BareHashRefNeverStrong(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "plan",
			title:   "Invitation Expiry Plan",
			body:    "Fixes #42 for the invitation expiry workflow.",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "docs/invitation-expiry.md", SourceIdentity: "docs/invitation-expiry.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "InvitationExpiryService",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/invitations/expiry.go", SourceIdentity: "internal/invitations/expiry.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"docs/invitation-expiry.md": {
			{id: "art_plan", kind: "markdown_artifact", subtype: "plan", title: "Invitation Expiry Plan", path: "docs/invitation-expiry.md"},
		},
		"internal/invitations/expiry.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "InvitationExpiryService", path: "internal/invitations/expiry.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["docs/invitation-expiry.md"][0],
		"art_source": byPath["internal/invitations/expiry.go"][0],
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "abcdef1234567890",
			Message:      "fixes #42 implement invitation expiry",
			CommittedAt:  "2026-05-26T10:00:00Z",
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files:       []gitfacts.FileChange{{CommitSHA: "abcdef1234567890", FilePath: "internal/invitations/expiry.go"}},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	cluster := findWorkstreamTestCluster(t, built.diagnostics.TopClusters, "gh-42")
	if cluster.Dialect != workstreamDialectBareHashRef {
		t.Fatalf("expected bare hash dialect, got %#v", cluster)
	}
	if cluster.PackStrength == workstreamPackStrengthStrong {
		t.Fatalf("expected bare hash to avoid strong evidence, got %#v", cluster)
	}
	for _, edge := range built.edges {
		meta := decodeEvidenceJSON(edge.MetadataJSON)
		anchors, _ := meta["anchors"].([]any)
		for _, rawAnchor := range anchors {
			anchor, _ := rawAnchor.(map[string]any)
			if evidenceString(anchor["canonical"]) == "gh-42" && evidenceString(anchor["pack_strength"]) == workstreamPackStrengthStrong {
				t.Fatalf("expected no strong bare-hash edge, got %#v", edge)
			}
		}
	}
}

func TestWorkstreamEvidence_BranchSlugDemotedBelowStrong(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "plan",
			title:   "Token Refresh",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "docs/token-refresh.md", SourceIdentity: "docs/token-refresh.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "Token Refresh",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/auth/token_refresh.go", SourceIdentity: "internal/auth/token_refresh.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"docs/token-refresh.md": {
			{id: "art_plan", kind: "markdown_artifact", subtype: "plan", title: "Token Refresh", path: "docs/token-refresh.md"},
		},
		"internal/auth/token_refresh.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Token Refresh", path: "internal/auth/token_refresh.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["docs/token-refresh.md"][0],
		"art_source": byPath["internal/auth/token_refresh.go"][0],
	}
	facts := gitfacts.Facts{
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull, Branch: "feature/token-refresh"},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	cluster := findWorkstreamTestCluster(t, built.diagnostics.TopClusters, "token-refresh")
	if cluster.Dialect != workstreamDialectBranchSlug {
		t.Fatalf("expected branch dialect to be tracked, got %#v", cluster)
	}
	if cluster.PackStrength == workstreamPackStrengthStrong {
		t.Fatalf("expected branch-derived slug below strong, got %#v", cluster)
	}
}

func TestWorkstreamEvidence_GenericTechnicalTermNeverStrong(t *testing.T) {
	acc := &workstreamAnchorAccumulator{
		canonical: "sha-256",
		display:   "sha-256",
		types:     map[string]bool{"title_slug": true},
		dialects:  map[string]bool{workstreamDialectGenericTechnical: true},
		sources:   map[string]bool{"artifact_title": true, "body": true},
		contexts:  map[string]bool{},
		artifacts: map[string]*workstreamArtifactAccumulator{
			"art_doc": {
				ref:     gitArtifactRef{id: "art_doc", kind: "markdown_artifact", subtype: "doc", title: "SHA-256", path: "docs/sha-256.md"},
				sources: map[string]bool{"artifact_title": true},
			},
			"art_source": {
				ref:     gitArtifactRef{id: "art_source", kind: "source_context", subtype: "code_comment", title: "SHA-256", path: "internal/crypto/sha256.go"},
				sources: map[string]bool{"body": true},
			},
		},
	}
	profile := buildWorkstreamDialectProfile(map[string]*workstreamAnchorAccumulator{"sha-256": acc})
	ids := []string{"art_doc", "art_source"}
	_, _, _, packStrength := workstreamScore(acc, ids, profile)
	if packStrength == workstreamPackStrengthStrong {
		t.Fatalf("expected generic technical term below strong")
	}
	if profile.trust[workstreamDialectGenericTechnical] != workstreamTrustWeak {
		t.Fatalf("expected weak trust for generic technical term, got %#v", profile.trust)
	}
}

func findWorkstreamTestCluster(t *testing.T, clusters []WorkstreamClusterExample, anchor string) WorkstreamClusterExample {
	t.Helper()
	for _, cluster := range clusters {
		if cluster.Anchor == anchor {
			return cluster
		}
	}
	t.Fatalf("expected cluster %q, got %#v", anchor, clusters)
	return WorkstreamClusterExample{}
}
