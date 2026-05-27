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
