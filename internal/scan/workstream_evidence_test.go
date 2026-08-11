package scan

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/gitfacts"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotEqual(t, 0, built.diagnostics.AnchorsMaterialized,
		"expected materialized anchors, got diagnostics %#v", built.diagnostics)
	require.NotEmpty(t, built.edges,
		"expected same_workstream_anchor edge")

	edge := built.edges[0]
	assert.Equal(t, edgeTypeSameWorkstreamAnchor, edge.EdgeType)
	assert.Equal(t, "art_plan", edge.SrcArtifactID)
	assert.Equal(t, "art_source", edge.DstArtifactID)
	assert.GreaterOrEqual(t, edge.Confidence, 0.9)
	require.NotEmpty(t, built.diagnostics.TopClusters)
	assert.Equal(t, workstreamPackStrengthStrong, built.diagnostics.TopClusters[0].PackStrength)
	assert.Equal(t, workstreamDialectTicketLikeUpper, built.diagnostics.TopClusters[0].Dialect)
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
	require.True(t, found,
		"expected slug-window workstream edge, got %#v", built.diagnostics.TopClusters)

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
	require.Empty(t, built.edges,
		"expected source/test-only locality to avoid edge materialization, got %#v", built.edges)
	require.NotEmpty(t, built.diagnostics.TopClusters)
	assert.Equal(t, workstreamPackStrengthSupportLocal, built.diagnostics.TopClusters[0].PackStrength)

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
	require.NotEmpty(t, built.diagnostics.TopClusters)
	assert.Equal(t, workstreamPackStrengthSupportCross, built.diagnostics.TopClusters[0].PackStrength)
	require.NotEmpty(t, built.edges,
		"expected cross-role support edge")

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
	var target *WorkstreamClusterExample
	for index := range built.diagnostics.TopClusters {
		if built.diagnostics.TopClusters[index].Anchor != "billing-entitlements" {
			continue
		}
		target = &built.diagnostics.TopClusters[index]
		break
	}
	require.NotNil(t, target, "expected billing-entitlements cluster, got %#v", built.diagnostics.TopClusters)
	assert.Equal(t, workstreamPackStrengthStrong, target.PackStrength)
	assert.NotZero(t, target.RoleFamilyMix["source"])
}

func TestWorkstreamEvidence_RejectsDateLikeBareGithubRef(t *testing.T) {
	extracted := extractFormalWorkstreamAnchors("See #2026 for the annual plan.", "body")
	require.Empty(t, extracted.anchors,
		"expected no anchors, got %#v", extracted.anchors)
	require.Len(t, extracted.rejected, 1)
	assert.Equal(t, "date_like_number", extracted.rejected[0].reason)
}

func TestWorkstreamEvidence_ExplicitWorkRefDialects(t *testing.T) {
	extracted := extractFormalWorkstreamAnchors("See PR-42, pull/43, ISSUE-44, issues/45, GH-46, #47, ADR-001, GPT-2, and LM-19.", "body")
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}
	require.Len(t, got, 12)
	assert.Equal(t, workstreamDialectExplicitPRRef, got["pr-42"])
	assert.Equal(t, workstreamDialectExplicitPRRef, got["pr-43"])
	assert.Equal(t, workstreamDialectExplicitIssueRef, got["issue-44"])
	assert.Equal(t, workstreamDialectExplicitIssueRef, got["issue-45"])
	assert.Equal(t, workstreamDialectExplicitGHRef, got["gh-46"])
	assert.Equal(t, workstreamDialectBareHashRef, got["gh-47"])
	assert.Equal(t, workstreamDialectDocumentNumberRef, got["ADR-001"])
	assert.Equal(t, workstreamDialectGenericTechnical, got["GPT-2"])
	assert.Equal(t, workstreamDialectTicketLikeUpper, got["LM-19"])
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
	require.Equal(t, workstreamDialectOpenSpecChangeSlug, cluster.Dialect,
		"expected OpenSpec change dialect, got %#v", cluster)
	require.Equal(t, workstreamPackStrengthStrong, cluster.PackStrength,
		"expected OpenSpec change slug to remain strong, got %#v", cluster)

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
	require.Equal(t, workstreamDialectBareHashRef, cluster.Dialect,
		"expected bare hash dialect, got %#v", cluster)
	require.NotEqual(t, workstreamPackStrengthStrong, cluster.PackStrength,
		"expected bare hash to avoid strong evidence, got %#v", cluster)

	for _, edge := range built.edges {
		meta := decodeEvidenceJSON(edge.MetadataJSON)
		anchors, _ := meta["anchors"].([]any)
		for _, rawAnchor := range anchors {
			anchor, ok := rawAnchor.(map[string]any)
			require.True(t, ok)
			if evidenceString(anchor["canonical"]) == "gh-42" {
				assert.NotEqual(t, workstreamPackStrengthStrong, evidenceString(anchor["pack_strength"]))
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
	require.Equal(t, workstreamDialectBranchSlug, cluster.Dialect,
		"expected branch dialect to be tracked, got %#v", cluster)
	require.NotEqual(t, workstreamPackStrengthStrong, cluster.PackStrength,
		"expected branch-derived slug below strong, got %#v", cluster)

}

func TestWorkstreamEvidence_MergeCommitExtractsPRAndTitleFossils(t *testing.T) {
	extracted := extractWorkstreamAnchorsFromCommit(gitfacts.Commit{
		Message:     "Merge pull request #123 from owner/fix-parser-regression",
		BodyPreview: "Fix parser regression for optional chaining",
		IsMerge:     true,
	})
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}

	assert.Equal(t, workstreamDialectGitHubMergePRRef, got["pr-123"])
	assert.Equal(t, workstreamDialectPRTitleSlug, got["parser-regression"])
}

func TestWorkstreamEvidence_MergeCommitWithoutTitleExtractsSourceBranchFossil(t *testing.T) {
	extracted := extractWorkstreamAnchorsFromCommit(gitfacts.Commit{
		Message: "Merge pull request #124 from owner/fix-parser-regression",
		IsMerge: true,
	})
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}

	assert.Equal(t, workstreamDialectMergeSourceBranch, got["parser-regression"])
}

func TestWorkstreamEvidence_SquashCommitExtractsPRAndTitleFossils(t *testing.T) {
	extracted := extractWorkstreamAnchorsFromCommit(gitfacts.Commit{
		Message: "Fix parser regression (#456)",
	})
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}

	assert.Equal(t, workstreamDialectSquashPRRef, got["pr-456"])
	assert.Equal(t, workstreamDialectPRTitleSlug, got["parser-regression"])
	assert.Empty(t, got["gh-456"])
}

func TestWorkstreamEvidence_ClosingReferenceExtractsIssueFossil(t *testing.T) {
	extracted := extractWorkstreamAnchorsFromCommit(gitfacts.Commit{
		Message:     "release parser fix",
		BodyPreview: "Fixes: #789",
	})
	got := map[string]string{}
	for _, anchor := range extracted.anchors {
		got[anchor.canonical] = anchor.dialect
	}

	assert.Equal(t, workstreamDialectIssueClosingRef, got["issue-789"])
}

func TestWorkstreamEvidence_PRRefFossilNeverStrong(t *testing.T) {
	acc := &workstreamAnchorAccumulator{
		canonical: "pr-123",
		display:   "pr-123",
		types:     map[string]bool{"github_ref": true},
		dialects:  map[string]bool{workstreamDialectGitHubMergePRRef: true},
		sources:   map[string]bool{"merge_header": true, "merge_header_changed_file": true, "body": true},
		contexts:  map[string]bool{"github_merge_pr": true},
		artifacts: map[string]*workstreamArtifactAccumulator{
			"art_doc": {
				ref:     gitArtifactRef{id: "art_doc", kind: "markdown_artifact", subtype: "plan", title: "PR 123", path: "docs/pr-123.md"},
				sources: map[string]bool{"body": true},
			},
			"art_source": {
				ref:     gitArtifactRef{id: "art_source", kind: "source_context", subtype: "code_comment", title: "PR 123", path: "internal/parser/regression.go"},
				sources: map[string]bool{"merge_header_changed_file": true},
			},
		},
	}
	profile := buildWorkstreamDialectProfile(map[string]*workstreamAnchorAccumulator{"pr-123": acc})
	_, _, _, packStrength := workstreamScore(acc, []string{"art_doc", "art_source"}, profile)
	require.NotEqual(t, workstreamPackStrengthStrong, packStrength,
		"expected PR ref fossil below strong")

}

func TestWorkstreamEvidence_PRTitleFossilCanBecomeStrongWhenDocBacked(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_plan",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "plan",
			title:   "Parser Regression Plan",
			body:    "Fix parser regression before releasing optional chaining.",
			sources: []store.SourceRow{{ArtifactID: "art_plan", SourceType: "markdown", Path: "docs/parser-regression.md", SourceIdentity: "docs/parser-regression.md"}},
		},
		{
			id:      "art_source",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "code_comment",
			title:   "Parser Regression",
			sources: []store.SourceRow{{ArtifactID: "art_source", SourceType: "source_context", Path: "internal/parser/regression.go", SourceIdentity: "internal/parser/regression.go"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"docs/parser-regression.md": {
			{id: "art_plan", kind: "markdown_artifact", subtype: "plan", title: "Parser Regression Plan", path: "docs/parser-regression.md"},
		},
		"internal/parser/regression.go": {
			{id: "art_source", kind: "source_context", subtype: "code_comment", title: "Parser Regression", path: "internal/parser/regression.go"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_plan":   byPath["docs/parser-regression.md"][0],
		"art_source": byPath["internal/parser/regression.go"][0],
	}
	facts := gitfacts.Facts{
		Commits: []gitfacts.Commit{{
			SHA:          "abcdef1234567890",
			Message:      "Merge pull request #123 from owner/fix-parser-regression",
			BodyPreview:  "Fix parser regression for optional chaining",
			CommittedAt:  "2026-05-26T10:00:00Z",
			IsMerge:      true,
			HistoryShape: gitfacts.ShapeFull,
		}},
		Files:       []gitfacts.FileChange{{CommitSHA: "abcdef1234567890", FilePath: "internal/parser/regression.go"}},
		Diagnostics: gitfacts.Diagnostics{Enabled: true, HistoryShape: gitfacts.ShapeFull},
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, facts)
	cluster := findWorkstreamTestCluster(t, built.diagnostics.TopClusters, "parser-regression")
	require.Equal(t, workstreamDialectPRTitleSlug, cluster.Dialect,
		"expected PR title dialect, got %#v", cluster)
	require.Equal(t, workstreamPackStrengthStrong, cluster.PackStrength,
		"expected PR title fossil to become strong with doc/source backing, got %#v", cluster)
	assert.Positive(t, built.diagnostics.PRFossilsSeen)
	assert.Positive(t, built.diagnostics.PRFossilsMaterialized)

}

func TestWorkstreamEvidence_TitleOnlyCrossRoleSlugIsLocality(t *testing.T) {
	artifacts := []evidenceArtifact{
		{
			id:      "art_design",
			repoID:  "repo",
			kind:    "markdown_artifact",
			subtype: "design",
			title:   "Jupyter Lab",
			sections: []store.SectionRow{{
				ID:    "section_design",
				Title: "The default JupyterLab connection lost provider",
			}},
			sources: []store.SourceRow{{ArtifactID: "art_design", SourceType: "markdown", Path: "design/about.md", SourceIdentity: "design/about.md"}},
		},
		{
			id:      "art_test",
			repoID:  "repo",
			kind:    "source_context",
			subtype: "test_case",
			title:   "Jupyter Lab",
			sources: []store.SourceRow{{ArtifactID: "art_test", SourceType: "source_context", Path: "galata/test/fixture.spec.ts", SourceIdentity: "galata/test/fixture.spec.ts"}},
		},
	}
	byPath := map[string][]gitArtifactRef{
		"design/about.md": {
			{id: "art_design", kind: "markdown_artifact", subtype: "design", title: "Jupyter Lab", path: "design/about.md"},
		},
		"galata/test/fixture.spec.ts": {
			{id: "art_test", kind: "source_context", subtype: "test_case", title: "Jupyter Lab", path: "galata/test/fixture.spec.ts"},
		},
	}
	byID := map[string]gitArtifactRef{
		"art_design": byPath["design/about.md"][0],
		"art_test":   byPath["galata/test/fixture.spec.ts"][0],
	}

	built := buildWorkstreamEvidence("repo", artifacts, byPath, byID, gitfacts.Facts{})
	cluster := findWorkstreamTestCluster(t, built.diagnostics.TopClusters, "jupyter-lab")
	require.Equal(t, workstreamDialectTitleHeadingSlug, cluster.Dialect,
		"expected title/heading dialect, got %#v", cluster)
	require.Equal(t, workstreamPackStrengthSupportLocal, cluster.PackStrength,
		"expected title-only cross-role slug to stay local support, got %#v", cluster)

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
	require.NotEqual(t, workstreamPackStrengthStrong, packStrength,
		"expected generic technical term below strong")
	require.Equal(t, workstreamTrustWeak, profile.trust[workstreamDialectGenericTechnical],
		"expected weak trust for generic technical term, got %#v", profile.trust)

}

func findWorkstreamTestCluster(t *testing.T, clusters []WorkstreamClusterExample, anchor string) WorkstreamClusterExample {
	t.Helper()
	var target *WorkstreamClusterExample
	for index := range clusters {
		if clusters[index].Anchor == anchor {
			target = &clusters[index]
			break
		}
	}
	require.NotNil(t, target, "expected cluster %q, got %#v", anchor, clusters)
	return *target
}
