package retrieval

import "testing"

func TestApplyDemotionOnlyNegativeEvidenceMovesUnrequestedPlaygroundRows(t *testing.T) {
	pack := RoleGroupedPack{
		Mode: "role_grouped_pack_v0_family_primary_v1",
		Groups: []PackGroup{{
			Role:   PackRoleImplementation,
			Title:  PackRoleTitle(PackRoleImplementation),
			Budget: 6,
			Items: []PackItem{
				{OriginalRank: 1, ID: "source", Path: "packages/vite/src/node/plugins/importMetaGlob.ts", Title: "importMetaGlob", Role: PackRoleImplementation, PackTier: PackTierPrimary},
				{OriginalRank: 2, ID: "playground", Path: "playground/glob-import/root/array-common-base/pattern1/a.js", Title: "playground glob import", Role: PackRoleImplementation, PackTier: PackTierPrimary},
			},
		}},
	}

	got := ApplyDemotionOnlyNegativeEvidence(pack, "Match import glob common base by path segment correctly")
	if len(got.Groups) != 1 || len(got.Groups[0].Items) != 1 {
		t.Fatalf("expected one kept implementation row, got %#v", got.Groups)
	}
	if got.Groups[0].Items[0].Path != "packages/vite/src/node/plugins/importMetaGlob.ts" {
		t.Fatalf("kept wrong row: %#v", got.Groups[0].Items)
	}
	if len(got.ExcludedNoise) != 1 {
		t.Fatalf("expected one demoted row, got %#v", got.ExcludedNoise)
	}
	if got.ExcludedNoise[0].Path != "playground/glob-import/root/array-common-base/pattern1/a.js" {
		t.Fatalf("demoted wrong row: %#v", got.ExcludedNoise)
	}
	if got.Metadata[packNegativeEvidenceCountKey] != "1" {
		t.Fatalf("missing negative evidence metadata: %#v", got.Metadata)
	}
	if got.Counts[PackRoleImplementation] != 1 || got.Counts[PackRoleExcludedNoise] != 1 {
		t.Fatalf("counts not recomputed: %#v", got.Counts)
	}
}

func TestApplyDemotionOnlyNegativeEvidenceKeepsRequestedPlaygroundRows(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{{
			Role:   PackRoleImplementation,
			Title:  PackRoleTitle(PackRoleImplementation),
			Budget: 6,
			Items: []PackItem{
				{OriginalRank: 1, ID: "playground", Path: "playground/glob-import/root/array-common-base/pattern1/a.js", Title: "playground glob import", Role: PackRoleImplementation},
			},
		}},
	}

	got := ApplyDemotionOnlyNegativeEvidence(pack, "Fix glob import playground coverage")
	if len(got.ExcludedNoise) != 0 {
		t.Fatalf("playground row should be kept when requested: %#v", got.ExcludedNoise)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Items) != 1 {
		t.Fatalf("expected playground row to remain: %#v", got.Groups)
	}
}

func TestApplyDemotionOnlyNegativeEvidenceKeepsNormalTests(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{{
			Role:   PackRoleBehaviorTests,
			Title:  PackRoleTitle(PackRoleBehaviorTests),
			Budget: 5,
			Items: []PackItem{
				{OriginalRank: 1, ID: "test", Path: "packages/vite/src/node/__tests__/config.spec.ts", Title: "config cacheDir test", Role: PackRoleBehaviorTests},
			},
		}},
	}

	got := ApplyDemotionOnlyNegativeEvidence(pack, "Use node_modules vite cacheDir when node_modules exists")
	if len(got.ExcludedNoise) != 0 {
		t.Fatalf("normal test should not be demoted: %#v", got.ExcludedNoise)
	}
	if len(got.Groups) != 1 || len(got.Groups[0].Items) != 1 {
		t.Fatalf("expected normal test to remain: %#v", got.Groups)
	}
}

func TestApplyDemotionOnlyNegativeEvidenceDemotesBlockedIntentWhenCurrentDecisionExists(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role:   PackRoleOpenWork,
				Title:  PackRoleTitle(PackRoleOpenWork),
				Budget: 3,
				Items: []PackItem{
					{
						OriginalRank: 1,
						ID:           "active",
						Path:         "docs/notes/next_epoch_decision_memo.md",
						Title:        "Epoch 4 external validity bridge decision memo",
						Status:       "next",
						Role:         PackRoleOpenWork,
						Reasons:      []string{"authority prior: owner decision record", "authority prior: active/next intent status"},
					},
					{
						OriginalRank: 2,
						ID:           "blocked",
						Path:         "docs/plans/D4.2-blocked-external-validity-bridge.md",
						Title:        "D4.2 blocked external validity bridge",
						Status:       "blocked",
						Role:         PackRoleOpenWork,
					},
				},
			},
		},
	}

	got := ApplyDemotionOnlyNegativeEvidence(pack, "epoch 4 external validity bridge")
	if len(got.Groups) != 1 || len(got.Groups[0].Items) != 1 {
		t.Fatalf("expected one active item in the working set, got %#v", got.Groups)
	}
	if got.Groups[0].Items[0].Path != "docs/notes/next_epoch_decision_memo.md" {
		t.Fatalf("kept wrong active row: %#v", got.Groups[0].Items)
	}
	if len(got.ExcludedNoise) != 1 || got.ExcludedNoise[0].Path != "docs/plans/D4.2-blocked-external-validity-bridge.md" {
		t.Fatalf("expected blocked plan to be downgraded, got %#v", got.ExcludedNoise)
	}
	if got.ExcludedNoise[0].RoleReason == "" {
		t.Fatalf("expected downgrade reason, got %#v", got.ExcludedNoise[0])
	}
}
