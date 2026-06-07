package retrieval

import "testing"

func TestApplyScoutSourceTestRescuePromotesRelatedAnimatorSource(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: FamilyPrimaryPackModeV1,
		Groups: []PackGroup{
			{
				Role:  PackRoleImplementation,
				Title: PackRoleTitle(PackRoleImplementation),
				Items: []PackItem{
					{OriginalRank: 1, ID: "scalar", Path: "src/textual/css/scalar_animation.py", Title: "scalar animation", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 2, ID: "await", Path: "src/textual/await_complete.py", Title: "await complete", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 5, ID: "animator", Path: "src/textual/_animator.py", Title: "animator", Role: PackRoleImplementation, PackTier: PackTierRelated},
				},
			},
			{
				Role:  PackRoleBehaviorTests,
				Title: PackRoleTitle(PackRoleBehaviorTests),
				Items: []PackItem{
					{OriginalRank: 6, ID: "animation-test", Path: "tests/test_animation.py", Title: "test animation", Role: PackRoleBehaviorTests, PackTier: PackTierPrimary, Subtype: "test_case"},
					{OriginalRank: 7, ID: "animator-test", Path: "tests/test_animator.py", Title: "test animator", Role: PackRoleBehaviorTests, PackTier: PackTierPrimary, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutSourceTestRescueForQuery(pack, "Fix on complete animation callback")
	tiers := map[string]string{}
	for _, group := range got.Groups {
		for _, item := range group.Items {
			tiers[item.Path] = item.PackTier
		}
	}
	if tiers["src/textual/_animator.py"] != PackTierPrimary {
		t.Fatalf("animator should be promoted to primary: %#v", tiers)
	}
	if got.Metadata[packScoutSourceRescueCountKey] != "1" {
		t.Fatalf("missing source rescue metadata: %#v", got.Metadata)
	}
}

func TestApplyScoutSourceTestRescueRequiresPrimaryTestEvidence(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: FamilyPrimaryPackModeV1,
		Groups: []PackGroup{{
			Role:  PackRoleImplementation,
			Title: PackRoleTitle(PackRoleImplementation),
			Items: []PackItem{
				{OriginalRank: 1, ID: "scalar", Path: "src/textual/css/scalar_animation.py", Title: "scalar animation", Role: PackRoleImplementation, PackTier: PackTierPrimary},
				{OriginalRank: 5, ID: "animator", Path: "src/textual/_animator.py", Title: "animator", Role: PackRoleImplementation, PackTier: PackTierRelated},
			},
		}},
	}

	got := ApplyScoutSourceTestRescueForQuery(pack, "Fix on complete animation callback")
	if got.Groups[0].Items[1].PackTier != PackTierRelated {
		t.Fatalf("source should stay related without primary test evidence: %#v", got.Groups[0].Items)
	}
}

func TestApplyScoutSourceTestRescueStopsAtPrimaryCap(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: FamilyPrimaryPackModeV1,
		Groups: []PackGroup{
			{
				Role:  PackRoleImplementation,
				Title: PackRoleTitle(PackRoleImplementation),
				Items: []PackItem{
					{OriginalRank: 1, ID: "a", Path: "src/a.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 2, ID: "b", Path: "src/b.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 3, ID: "c", Path: "src/c.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 4, ID: "d", Path: "src/d.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 5, ID: "e", Path: "src/e.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 6, ID: "f", Path: "src/f.py", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 7, ID: "animator", Path: "src/textual/_animator.py", Role: PackRoleImplementation, PackTier: PackTierRelated},
				},
			},
			{
				Role:  PackRoleBehaviorTests,
				Title: PackRoleTitle(PackRoleBehaviorTests),
				Items: []PackItem{
					{OriginalRank: 8, ID: "test", Path: "tests/test_animator.py", Role: PackRoleBehaviorTests, PackTier: PackTierRelated, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutSourceTestRescueForQuery(pack, "Fix animation callback")
	for _, group := range got.Groups {
		for _, item := range group.Items {
			if item.Path == "src/textual/_animator.py" && item.PackTier == PackTierPrimary {
				t.Fatalf("source should not promote once cap is full: %#v", got.Groups)
			}
		}
	}
}

func TestApplyScoutSourcePrimaryPreservationPromotesHighRankRelatedSource(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: FamilyPrimaryPackModeV1,
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, ID: "arrow", Path: "src/language-js/print/arrow-function.js", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 3, ID: "type-check", Path: "scripts/tools/prefer-create-type-check-function.js", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 4, ID: "comments", Path: "src/language-js/comments/handle-comments.js", Role: PackRoleImplementation, PackTier: PackTierRelated, Reasons: []string{"query term match in path: comment"}},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 9, ID: "arrow-test", Path: "tests/format/js/in/arrow-function.js", Role: PackRoleBehaviorTests, PackTier: PackTierPrimary, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutSourcePrimaryPreservationForQuery(pack, "fix unstable comment in arrow function with sequence expression body")
	tiers := familyPrimaryTestTiers(got)
	if tiers["src/language-js/comments/handle-comments.js"] != PackTierPrimary {
		t.Fatalf("high-rank comment source should be preserved as primary: %#v", tiers)
	}
	if got.Metadata[packScoutSourcePreservationCountKey] != "1" {
		t.Fatalf("missing preservation metadata: %#v", got.Metadata)
	}
}

func TestApplyScoutSourcePrimaryPreservationCanPromoteOverRescueCap(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: FamilyPrimaryPackModeV1,
		Groups: []PackGroup{
			{
				Role: PackRoleImplementation,
				Items: []PackItem{
					{OriginalRank: 1, ID: "conda", Path: "models/packages/conda/search.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 2, ID: "nuget", Path: "models/packages/nuget/search.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 3, ID: "api", Path: "routers/api/v1/packages/package.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 4, ID: "conan", Path: "models/packages/conan/search.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 6, ID: "rubygems", Path: "routers/api/packages/rubygems/rubygems.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 7, ID: "npm", Path: "modules/packages/npm/creator.go", Role: PackRoleImplementation, PackTier: PackTierPrimary},
					{OriginalRank: 12, ID: "package", Path: "models/packages/package.go", Role: PackRoleImplementation, PackTier: PackTierRelated, Reasons: []string{"relationship expansion: source_manifest_family_recovery"}},
				},
			},
			{
				Role: PackRoleBehaviorTests,
				Items: []PackItem{
					{OriginalRank: 17, ID: "package-test", Path: "models/packages/package_test.go#L24", Role: PackRoleBehaviorTests, PackTier: PackTierPrimary, Subtype: "test_case"},
					{OriginalRank: 19, ID: "rubygems-test", Path: "routers/api/packages/rubygems/rubygems_test.go#L15", Role: PackRoleBehaviorTests, PackTier: PackTierPrimary, Subtype: "test_case"},
				},
			},
		},
	}

	got := ApplyScoutSourcePrimaryPreservationForQuery(pack, "search package versions by type name version and reject duplicate versions")
	tiers := familyPrimaryTestTiers(got)
	if tiers["models/packages/package.go"] != PackTierPrimary {
		t.Fatalf("package source should be preserved beyond old rescue cap: %#v", tiers)
	}
}
