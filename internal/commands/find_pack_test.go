package commands

import (
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestDisplayPackReasons_HidesDebugScoreAndGenericTerms(t *testing.T) {
	got := displayPackReasons([]string{
		"anchor-first ranking: score 24.000; matches activity, event, query; fields path, title, body",
		"query term match in body: how",
		"query term match in path: activity",
	}, false)
	joined := strings.Join(got, "; ")
	if strings.Contains(joined, "score") {
		t.Fatalf("display reasons leaked scorer internals: %#v", got)
	}
	if strings.Contains(joined, "how") {
		t.Fatalf("display reasons kept generic task word: %#v", got)
	}
	for _, want := range []string{"matched anchors: activity, event, query", "path matched: activity"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("display reasons missing %q: %#v", want, got)
		}
	}
}

func TestDisplayPackReasons_CompactsSectionReceipts(t *testing.T) {
	got := displayPackReasons([]string{
		"section-packed context: Architecture Design > Human Attention Optimization > Two-Tier Event Handling; Architecture Design > The 8 Plugin Slots > Agent",
		"indexed section match: Architecture Design > Human Attention Optimization lines 395-418; Architecture Design > The 8 Plugin Slots lines 100-120",
	}, false)
	joined := strings.Join(got, "; ")
	for _, want := range []string{"section focus:", "section evidence:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("display reasons missing compact section label %q: %#v", want, got)
		}
	}
	if strings.Contains(joined, "section-packed context") || strings.Contains(joined, "indexed section match") {
		t.Fatalf("display reasons leaked internal section labels: %#v", got)
	}
}

func TestConcisePackReasons_AvoidsCollapsedMoreMarkers(t *testing.T) {
	got := concisePackReasons([]string{
		"section-packed context: MCP Server Design Guidelines > Table of Contents; MCP Server Design Guidelines > Project Structure; MCP Server Design Guidelines > Package Naming and Versioning",
		"indexed section match: MCP Server Design Guidelines > Table of Contents lines 5-50; MCP Server Design Guidelines > Package Naming and Versioning lines 119-165",
		"anchor-first ranking: score 24.000; matches server, design, guidelines; fields title, heading, body, path",
	})
	joined := strings.Join(got, "; ")
	for _, notWant := range []string{"+1 more", "section focus", "section evidence", "Table of Contents"} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("concise reasons leaked %q: %#v", notWant, got)
		}
	}
	for _, want := range []string{"matched: server, design, guidelines", "sections: Project Structure; Package Naming and Versioning"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("concise reasons missing %q: %#v", want, got)
		}
	}
}

func TestPackCoverageText_UsesRoleNames(t *testing.T) {
	got := packCoverageText(retrieval.PackSummary{
		HasBackgroundDecisions: true,
		HasImplementation:      true,
		HasBehaviorTests:       true,
	})
	if got != "background + implementation + tests" {
		t.Fatalf("coverage text = %q", got)
	}
}

func TestWriteFindPackTextBoundaryPrimarySummarizesRelatedByDefault(t *testing.T) {
	pack := retrieval.ApplyBoundaryPrimaryPack(retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Summary: retrieval.PackSummary{
			IncludedCount:          4,
			RoleDiversity:          1,
			HasBackgroundDecisions: true,
		},
		Groups: []retrieval.PackGroup{
			{
				Role:  retrieval.PackRoleBackgroundDecisions,
				Title: retrieval.PackRoleTitle(retrieval.PackRoleBackgroundDecisions),
				Items: []retrieval.PackItem{
					{OriginalRank: 1, ID: "a", ShortID: "a", Path: "docs/design/auth/a.md", Title: "Primary Auth Design"},
					{OriginalRank: 2, ID: "b", ShortID: "b", Path: "docs/design/auth/b.md", Title: "Related Auth Design"},
					{OriginalRank: 3, ID: "c", ShortID: "c", Path: "docs/design/auth/c.md", Title: "Related Auth Notes"},
					{OriginalRank: 4, ID: "d", ShortID: "d", Path: "docs/design/auth/d.md", Title: "Related Auth Followup"},
				},
			},
		},
	})
	var b strings.Builder
	if err := writeFindPackText(&b, "auth design", "test", pack, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Primary Auth Design") {
		t.Fatalf("default boundary output missing primary item:\n%s", out)
	}
	if !strings.Contains(out, "Related context kept for verbose/JSON:") {
		t.Fatalf("default boundary output missing related summary:\n%s", out)
	}
	if strings.Contains(out, "   4. d  Related Auth Followup") {
		t.Fatalf("default boundary output should not print related items as full rows:\n%s", out)
	}
}

func TestWriteFindPackTextBoundaryPrimaryVerboseShowsRelatedItems(t *testing.T) {
	pack := retrieval.ApplyBoundaryPrimaryPack(retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Summary: retrieval.PackSummary{
			IncludedCount:          3,
			RoleDiversity:          1,
			HasBackgroundDecisions: true,
		},
		Groups: []retrieval.PackGroup{
			{
				Role:  retrieval.PackRoleBackgroundDecisions,
				Title: retrieval.PackRoleTitle(retrieval.PackRoleBackgroundDecisions),
				Items: []retrieval.PackItem{
					{OriginalRank: 1, ID: "a", ShortID: "a", Path: "docs/design/auth/a.md", Title: "Primary Auth Design"},
					{OriginalRank: 2, ID: "b", ShortID: "b", Path: "docs/design/auth/b.md", Title: "Related Auth Design"},
					{OriginalRank: 3, ID: "c", ShortID: "c", Path: "docs/design/auth/c.md", Title: "Related Auth Notes"},
				},
			},
		},
	})
	var b strings.Builder
	if err := writeFindPackText(&b, "auth design", "test", pack, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Related Auth Notes") {
		t.Fatalf("verbose boundary output should show related items:\n%s", out)
	}
	if strings.Contains(out, "Related context kept for verbose/JSON:") {
		t.Fatalf("verbose boundary output should keep detailed role groups instead of compact summary:\n%s", out)
	}
}

func TestWriteFindPackTextFamilyPrimarySummarizesRelatedFamilies(t *testing.T) {
	pack := retrieval.ApplyFamilyPrimaryPackForQuery(retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Summary: retrieval.PackSummary{
			IncludedCount:     4,
			RoleDiversity:     2,
			HasImplementation: true,
			HasBehaviorTests:  true,
		},
		Metadata: map[string]string{
			"local_language_receipts": strings.Join([]string{
				"exact anchor on appears in path/body evidence across the pack",
				"exact anchor rds appears in path/body evidence across the pack",
			}, "\n"),
		},
		Groups: []retrieval.PackGroup{
			{
				Role:  retrieval.PackRoleImplementation,
				Title: retrieval.PackRoleTitle(retrieval.PackRoleImplementation),
				Items: []retrieval.PackItem{
					{OriginalRank: 1, ID: "rds", ShortID: "rds", Path: "discovery/aws/rds.go", Title: "discovery/aws/rds.go", Role: retrieval.PackRoleImplementation},
					{OriginalRank: 2, ID: "ecs", ShortID: "ecs", Path: "discovery/aws/ecs.go", Title: "discovery/aws/ecs.go", Role: retrieval.PackRoleImplementation},
					{OriginalRank: 3, ID: "elasticache", ShortID: "elasticache", Path: "discovery/aws/elasticache.go", Title: "discovery/aws/elasticache.go", Role: retrieval.PackRoleImplementation},
					{OriginalRank: 4, ID: "ec2", ShortID: "ec2", Path: "discovery/aws/ec2.go", Title: "discovery/aws/ec2.go", Role: retrieval.PackRoleImplementation},
					{OriginalRank: 5, ID: "msk", ShortID: "msk", Path: "discovery/aws/msk.go", Title: "discovery/aws/msk.go", Role: retrieval.PackRoleImplementation},
				},
			},
			{
				Role:  retrieval.PackRoleBehaviorTests,
				Title: retrieval.PackRoleTitle(retrieval.PackRoleBehaviorTests),
				Items: []retrieval.PackItem{
					{OriginalRank: 6, ID: "rds-test", ShortID: "rds-test", Path: "discovery/aws/rds_test.go#L1", SourcePath: "discovery/aws/rds_test.go", Title: "TestRDS", Subtype: "test_case", Role: retrieval.PackRoleBehaviorTests},
					{OriginalRank: 7, ID: "ecs-test", ShortID: "ecs-test", Path: "discovery/aws/ecs_test.go#L1", SourcePath: "discovery/aws/ecs_test.go", Title: "TestECS", Subtype: "test_case", Role: retrieval.PackRoleBehaviorTests},
					{OriginalRank: 8, ID: "elasticache-test", ShortID: "elasticache-test", Path: "discovery/aws/elasticache_test.go#L1", SourcePath: "discovery/aws/elasticache_test.go", Title: "TestElasticache", Subtype: "test_case", Role: retrieval.PackRoleBehaviorTests},
					{OriginalRank: 9, ID: "ec2-test", ShortID: "ec2-test", Path: "discovery/aws/ec2_test.go#L1", SourcePath: "discovery/aws/ec2_test.go", Title: "TestEC2", Subtype: "test_case", Role: retrieval.PackRoleBehaviorTests},
					{OriginalRank: 10, ID: "msk-test", ShortID: "msk-test", Path: "discovery/aws/msk_test.go#L1", SourcePath: "discovery/aws/msk_test.go", Title: "TestMSK", Subtype: "test_case", Role: retrieval.PackRoleBehaviorTests},
				},
			},
		},
	}, "Handle RDS clusters without instances in AWS discovery")

	var b strings.Builder
	if err := writeFindPackText(&b, "Handle RDS clusters without instances in AWS discovery", "test", pack, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "Related families kept for verbose/JSON:") {
		t.Fatalf("family-primary output missing related summary:\n%s", out)
	}
	if strings.Contains(out, "exact anchor on appears") {
		t.Fatalf("family-primary output leaked generic local-language receipt:\n%s", out)
	}
	if !strings.Contains(out, "exact anchor rds appears") {
		t.Fatalf("family-primary output should keep specific local-language receipt:\n%s", out)
	}
	if strings.Contains(out, "  10. msk-test") {
		t.Fatalf("family-primary default output should collapse related rows:\n%s", out)
	}
}

func TestWriteFindPackTextFamilyPrimaryVerboseShowsRelatedRows(t *testing.T) {
	pack := retrieval.ApplyFamilyPrimaryPackForQuery(retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []retrieval.PackGroup{
			{
				Role: retrieval.PackRoleImplementation,
				Items: []retrieval.PackItem{
					{OriginalRank: 1, ID: "rds", ShortID: "rds", Path: "discovery/aws/rds.go", Title: "discovery/aws/rds.go", Role: retrieval.PackRoleImplementation},
					{OriginalRank: 2, ID: "ecs", ShortID: "ecs", Path: "discovery/aws/ecs.go", Title: "discovery/aws/ecs.go", Role: retrieval.PackRoleImplementation},
				},
			},
		},
	}, "Handle RDS clusters without instances in AWS discovery")

	var b strings.Builder
	if err := writeFindPackText(&b, "Handle RDS clusters without instances in AWS discovery", "test", pack, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "discovery/aws/ecs.go") {
		t.Fatalf("verbose family-primary output should show related rows:\n%s", out)
	}
	if strings.Contains(out, "Related families kept for verbose/JSON:") {
		t.Fatalf("verbose family-primary output should show rows instead of summary:\n%s", out)
	}
}
