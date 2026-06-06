package retrieval

import (
	"strings"
	"testing"
)

func TestApplyFamilyPrimaryPackForQueryKeepsExactSourceAndTestPrimary(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role:  PackRoleImplementation,
				Title: PackRoleTitle(PackRoleImplementation),
				Items: []PackItem{
					{OriginalRank: 1, ID: "rds", Path: "discovery/aws/rds.go", Title: "discovery/aws/rds.go", Role: PackRoleImplementation},
					{OriginalRank: 2, ID: "ecs", Path: "discovery/aws/ecs.go", Title: "discovery/aws/ecs.go", Role: PackRoleImplementation},
					{OriginalRank: 3, ID: "elasticache", Path: "discovery/aws/elasticache.go", Title: "discovery/aws/elasticache.go", Role: PackRoleImplementation},
					{OriginalRank: 4, ID: "ec2", Path: "discovery/aws/ec2.go", Title: "discovery/aws/ec2.go", Role: PackRoleImplementation},
					{OriginalRank: 5, ID: "msk", Path: "discovery/aws/msk.go", Title: "discovery/aws/msk.go", Role: PackRoleImplementation},
				},
			},
			{
				Role:  PackRoleBehaviorTests,
				Title: PackRoleTitle(PackRoleBehaviorTests),
				Items: []PackItem{
					{OriginalRank: 6, ID: "rds-test", Path: "discovery/aws/rds_test.go#L439", SourcePath: "discovery/aws/rds_test.go", Title: "TestDescribeAllDBClusters", Subtype: "test_case", Role: PackRoleBehaviorTests},
					{OriginalRank: 7, ID: "ecs-test", Path: "discovery/aws/ecs_test.go#L42", SourcePath: "discovery/aws/ecs_test.go", Title: "TestECSDiscoveryListClusterARNs", Subtype: "test_case", Role: PackRoleBehaviorTests},
					{OriginalRank: 8, ID: "msk-test", Path: "discovery/aws/msk_test.go#L129", SourcePath: "discovery/aws/msk_test.go", Title: "TestMSKDiscoveryDescribeClusters", Subtype: "test_case", Role: PackRoleBehaviorTests},
				},
			},
		},
	}

	got := ApplyFamilyPrimaryPackForQuery(pack, "Handle RDS clusters without instances in AWS discovery")
	if got.Mode != FamilyPrimaryPackMode {
		t.Fatalf("mode = %q", got.Mode)
	}
	if got.Metadata["family_primary"] != "true" {
		t.Fatalf("missing metadata: %#v", got.Metadata)
	}
	tiers := map[string]string{}
	for _, group := range got.Groups {
		for _, item := range group.Items {
			tiers[item.Path] = item.PackTier
			if item.Boundary == "" {
				t.Fatalf("item missing family boundary: %#v", item)
			}
		}
	}
	if tiers["discovery/aws/rds.go"] != PackTierPrimary {
		t.Fatalf("rds source should be primary, tiers=%#v", tiers)
	}
	if tiers["discovery/aws/rds_test.go#L439"] != PackTierPrimary {
		t.Fatalf("rds test should be primary, tiers=%#v", tiers)
	}
	if related := FamilyPrimaryRelatedSummaries(got); len(related) == 0 {
		t.Fatalf("expected related family summaries")
	}
}

func TestApplyFamilyPrimaryPackForQuerySuppressesGenericMetadataAnchors(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, ID: "anim", Path: "src/textual/css/scalar_animation.py", Title: "scalar animation", Role: PackRoleImplementation},
				},
			},
		},
	}

	got := ApplyFamilyPrimaryPackForQuery(pack, "Fix on complete animation callback")
	suppressed := got.Metadata["family_primary_suppressed_anchors"]
	if !(strings.Contains(suppressed, "fix") && strings.Contains(suppressed, "on")) {
		t.Fatalf("suppressed anchors = %#v", got.Metadata)
	}
	if anchors := got.Metadata["family_primary_anchors"]; anchors == "" || anchors == "on" {
		t.Fatalf("specific anchors missing or generic: %#v", got.Metadata)
	}
}

func TestApplyFamilyPrimaryPackV1ForQueryProtectsTopRankedEditTargets(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, ID: "cli", Path: "src/cli.rs", Title: "src/cli.rs", Role: PackRoleImplementation},
					{OriginalRank: 2, ID: "main", Path: "src/main.rs", Title: "src/main.rs", Role: PackRoleImplementation},
					{OriginalRank: 3, ID: "regex", Path: "src/regex_helper.rs", Title: "src/regex_helper.rs", Role: PackRoleImplementation},
					{OriginalRank: 4, ID: "walk", Path: "src/walk.rs", Title: "src/walk.rs", Role: PackRoleImplementation},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 5, ID: "tests", Path: "tests/tests.rs", Title: "tests/tests.rs", Role: PackRoleBehaviorTests, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyFamilyPrimaryPackV1ForQuery(pack, "tell users to enable hidden search when their pattern only matches dotfiles")
	if got.Mode != FamilyPrimaryPackModeV1 {
		t.Fatalf("mode = %q", got.Mode)
	}
	if got.Metadata["family_primary_exact_protection"] != "true" {
		t.Fatalf("missing exact protection metadata: %#v", got.Metadata)
	}
	tiers := map[string]string{}
	for _, group := range got.Groups {
		for _, item := range group.Items {
			tiers[item.Path] = item.PackTier
		}
	}
	if tiers["src/cli.rs"] != PackTierPrimary {
		t.Fatalf("top-ranked cli edit target should stay primary: %#v", tiers)
	}
	if tiers["src/main.rs"] != PackTierPrimary {
		t.Fatalf("top-ranked main edit target should stay primary: %#v", tiers)
	}
}

func TestFamilyPrimaryProtectedEntrySkipsWeakTutorialSource(t *testing.T) {
	entry := &familyPrimaryEntry{
		class: "source",
		item: PackItem{
			OriginalRank: 1,
			Path:         "docs_src/custom_docs_ui/tutorial001.py",
			Role:         PackRoleImplementation,
		},
		score: 12,
	}
	if familyPrimaryProtectedEntry(entry) {
		t.Fatalf("tutorial source should not be exact-protected")
	}
}

func TestFamilyPrimaryProtectedEntryKeepsLossSafePreservedSource(t *testing.T) {
	entry := &familyPrimaryEntry{
		class: "source",
		item: PackItem{
			OriginalRank: 12,
			Path:         "apps/web/lib/api/links/get-links-for-workspace.ts",
			Role:         PackRoleImplementation,
			Reasons:      []string{"relationship expansion: source_manifest_loss_safe_preserved"},
		},
		score: 2,
	}
	if !familyPrimaryProtectedEntry(entry) {
		t.Fatalf("loss-safe preserved source should stay protected")
	}
}

func TestApplyFamilyPrimaryPackV2KeepsSameFamilyTestPrimary(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, ID: "link-card", Path: "web/src/components/MemoContent/LinkMetadataCard.tsx", Title: "LinkMetadataCard", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches description, metadata, link"}},
					{OriginalRank: 2, ID: "dialog", Path: "web/src/components/MemoMetadata/Relation/LinkMemoDialog.tsx", Title: "LinkMemoDialog", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches description, metadata, link, preview"}},
					{OriginalRank: 3, ID: "html-meta", Path: "internal/httpgetter/html_meta.go", Title: "internal/httpgetter/html_meta.go", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches parse, html, description"}},
					{OriginalRank: 4, ID: "markdown-link", Path: "web/src/components/MemoContent/markdown/Link.tsx", Title: "Link", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches link"}},
					{OriginalRank: 5, ID: "linked", Path: "web/src/components/Settings/LinkedIdentitySection.tsx", Title: "LinkedIdentitySection", Role: PackRoleImplementation, Reasons: []string{"relationship expansion: source_manifest_loss_safe_preserved"}},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 6, ID: "link-meta-test", Path: "server/router/api/v1/link_metadata_test.go", Title: "TestGetLinkMetadata", Role: PackRoleBehaviorTests, Subtype: "test_case", Reasons: []string{"relationship expansion: source_manifest_family_recovery"}},
					{OriginalRank: 7, ID: "preview-test", Path: "web/tests/preview-image-dialog.test.tsx", Title: "preview image dialog", Role: PackRoleBehaviorTests, Subtype: "test_case", Reasons: []string{"relationship expansion: source_manifest_family_recovery"}},
					{OriginalRank: 8, ID: "html-meta-test", Path: "internal/httpgetter/html_meta_test.go", Title: "TestGetHTMLMetaWithNameOnly", Role: PackRoleBehaviorTests, Subtype: "test_case", Reasons: []string{"relationship expansion: source_manifest_loss_safe_preserved"}},
				},
			},
		},
	}

	got := ApplyFamilyPrimaryPackV2ForQuery(pack, "parse html description metadata when building link preview cards")
	tiers := familyPrimaryTestTiers(got)
	if tiers["internal/httpgetter/html_meta.go"] != PackTierPrimary {
		t.Fatalf("html_meta source should stay primary: %#v", tiers)
	}
	if tiers["internal/httpgetter/html_meta_test.go"] != PackTierPrimary {
		t.Fatalf("same-family html_meta test should be primary: %#v", tiers)
	}
}

func TestApplyFamilyPrimaryPackV2UsesAnchorVariantsForSharedPages(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{{
			Role: PackRoleImplementation,
			Items: []PackItem{
				{OriginalRank: 1, ID: "dashboard", Path: "src/components/hooks/queries/useDashboardQuery.ts", Title: "useDashboardQuery", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches api, hooks, dashboard"}},
				{OriginalRank: 2, ID: "api", Path: "src/components/hooks/useApi.ts", Title: "useApi", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches api, hooks"}},
				{OriginalRank: 3, ID: "event", Path: "src/components/hooks/queries/useEventStatsQuery.ts", Title: "useEventStatsQuery", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches api, hooks"}},
				{OriginalRank: 4, ID: "website", Path: "src/components/hooks/queries/useWebsiteValuesQuery.ts", Title: "useWebsiteValuesQuery", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches api, hooks"}},
				{OriginalRank: 5, ID: "share", Path: "src/components/hooks/queries/useShareTokenQuery.ts", Title: "useShareTokenQuery", Role: PackRoleImplementation, Reasons: []string{"relationship expansion: source_manifest_loss_safe_preserved", "anchor-first ranking: score 24.000; matches api, hooks"}},
			},
		}},
	}

	got := ApplyFamilyPrimaryPackV2ForQuery(pack, "repair permission checks and API hooks for shared dashboard pages")
	tiers := familyPrimaryTestTiers(got)
	if tiers["src/components/hooks/queries/useShareTokenQuery.ts"] != PackTierPrimary {
		t.Fatalf("share-token hook should be primary via shared/share anchor variant: %#v", tiers)
	}
}

func TestApplyFamilyPrimaryPackV2UsesSynchronizationVariant(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []PackGroup{{
			Role: PackRoleImplementation,
			Items: []PackItem{
				{OriginalRank: 1, ID: "oauth", Path: "routers/web/auth/oauth.go", Title: "oauth.go", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches oauth, external, claims"}},
				{OriginalRank: 2, ID: "external", Path: "models/user/external_login_user.go", Title: "external_login_user.go", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches external"}},
				{OriginalRank: 3, ID: "source-sync", Path: "services/auth/source/oauth2/source_sync.go", Title: "source_sync.go", Role: PackRoleImplementation, Reasons: []string{"anchor-first ranking: score 24.000; matches oauth"}},
				{OriginalRank: 4, ID: "signin-sync", Path: "routers/web/auth/oauth_signin_sync.go", Title: "oauth_signin_sync.go", Role: PackRoleImplementation, Reasons: []string{"relationship expansion: source_manifest_loss_safe_preserved"}},
			},
		}},
	}

	got := ApplyFamilyPrimaryPackV2ForQuery(pack, "make oauth sign in synchronization apply external claims during the first login")
	tiers := familyPrimaryTestTiers(got)
	if tiers["routers/web/auth/oauth_signin_sync.go"] != PackTierPrimary {
		t.Fatalf("signin sync source should be primary via synchronization/sync variant: %#v", tiers)
	}
}

func familyPrimaryTestTiers(pack RoleGroupedPack) map[string]string {
	tiers := map[string]string{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			tiers[item.Path] = item.PackTier
		}
	}
	return tiers
}
