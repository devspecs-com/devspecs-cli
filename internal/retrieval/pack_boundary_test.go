package retrieval

import "testing"

func TestApplyBoundaryPrimaryPackMarksRelatedDocsButKeepsSourcePrimary(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role:  PackRoleBackgroundDecisions,
				Title: PackRoleTitle(PackRoleBackgroundDecisions),
				Items: []PackItem{
					{OriginalRank: 1, ID: "doc1", Path: "design-docs/auth/local-auth.md", Title: "Local auth design"},
					{OriginalRank: 2, ID: "doc2", Path: "design-docs/auth/cache.md", Title: "Auth cache design"},
					{OriginalRank: 3, ID: "doc3", Path: "design-docs/auth/glue.md", Title: "Glue catalog auth"},
				},
			},
			{
				Role:  PackRoleImplementation,
				Title: PackRoleTitle(PackRoleImplementation),
				Items: []PackItem{
					{OriginalRank: 9, ID: "src1", Path: "internal/auth/session.go", Title: "session.go"},
					{OriginalRank: 10, ID: "src2", Path: "internal/auth/token.go", Title: "token.go"},
				},
			},
			{
				Role:  PackRoleBehaviorTests,
				Title: PackRoleTitle(PackRoleBehaviorTests),
				Items: []PackItem{
					{OriginalRank: 11, ID: "test1", Path: "internal/auth/session_test.go", Title: "session_test.go"},
				},
			},
		},
	}

	got := ApplyBoundaryPrimaryPack(pack)
	if got.Mode != BoundaryPrimaryPackMode {
		t.Fatalf("mode = %q", got.Mode)
	}
	if got.Metadata["boundary_primary"] != "true" {
		t.Fatalf("missing boundary metadata: %#v", got.Metadata)
	}
	relatedDocs := 0
	primarySource := 0
	primaryTests := 0
	for _, group := range got.Groups {
		for _, item := range group.Items {
			if item.Boundary == "" {
				t.Fatalf("item missing boundary: %#v", item)
			}
			if group.Role == PackRoleBackgroundDecisions && item.PackTier == PackTierRelated {
				relatedDocs++
			}
			if group.Role == PackRoleImplementation && item.PackTier == PackTierPrimary {
				primarySource++
			}
			if group.Role == PackRoleBehaviorTests && item.PackTier == PackTierPrimary {
				primaryTests++
			}
		}
	}
	if relatedDocs == 0 {
		t.Fatalf("expected at least one duplicate doc to be related: %#v", got.Groups[0].Items)
	}
	if primarySource != 2 || primaryTests != 1 {
		t.Fatalf("source/test not protected, source=%d tests=%d groups=%#v", primarySource, primaryTests, got.Groups)
	}
	if summaries := BoundaryRelatedSummaries(got); len(summaries) == 0 {
		t.Fatalf("expected related summaries")
	}
}

func TestApplyBoundaryPrimaryPackPreservesAllItems(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role:  PackRoleBackgroundDecisions,
				Title: PackRoleTitle(PackRoleBackgroundDecisions),
				Items: []PackItem{
					{OriginalRank: 1, ID: "doc1", Path: "docs/design/a.md", Title: "A"},
					{OriginalRank: 2, ID: "doc2", Path: "docs/design/b.md", Title: "B"},
					{OriginalRank: 3, ID: "doc3", Path: "docs/design/c.md", Title: "C"},
					{OriginalRank: 4, ID: "doc4", Path: "docs/design/d.md", Title: "D"},
				},
			},
		},
	}

	got := ApplyBoundaryPrimaryPack(pack)
	total := 0
	related := 0
	for _, group := range got.Groups {
		total += len(group.Items)
		for _, item := range group.Items {
			if item.PackTier == PackTierRelated {
				related++
			}
		}
	}
	if total != 4 {
		t.Fatalf("boundary pack should preserve all items, got %d", total)
	}
	if related == 0 {
		t.Fatalf("expected some docs to be related")
	}
}

func TestApplyBoundaryPrimaryPackForQueryKeepsProposalFamilyAnchorVisible(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role:  PackRoleBackgroundDecisions,
				Title: PackRoleTitle(PackRoleBackgroundDecisions),
				Items: []PackItem{
					{OriginalRank: 1, ID: "bep", Path: "beps/0012-metrics-service/README.md", Title: "Backstage Metrics Service"},
					{OriginalRank: 2, ID: "naming", Path: "docs/backend-system/architecture/08-naming-patterns.md", Title: "Backend System Naming Patterns"},
					{OriginalRank: 3, ID: "adr", Path: "docs/architecture-decisions/adr005-catalog-core-entities.md", Title: "ADR005"},
					{OriginalRank: 4, ID: "frontend", Path: "docs/frontend-system/architecture/50-naming-patterns.md", Title: "Frontend System Naming Patterns"},
					{OriginalRank: 5, ID: "beps", Path: "beps/README.md", Title: "Backstage Enhancement Proposals (BEPs)"},
				},
			},
		},
	}

	got := ApplyBoundaryPrimaryPackForQuery(pack, "Backstage core MetricsService proposal with OpenTelemetry naming conventions")
	tiers := map[string]string{}
	for _, group := range got.Groups {
		for _, item := range group.Items {
			tiers[item.Path] = item.PackTier
		}
	}
	if tiers["beps/README.md"] != PackTierPrimary {
		t.Fatalf("proposal family anchor should remain primary, tiers=%#v", tiers)
	}
}
