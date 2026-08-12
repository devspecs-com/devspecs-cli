package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Items, 1)
	require.Equal(t, "packages/vite/src/node/plugins/importMetaGlob.ts", got.Groups[0].Items[0].Path,
		"kept wrong row: %#v", got.Groups[0].Items)
	require.Len(t, got.ExcludedNoise, 1,
		"expected one demoted row, got %#v", got.ExcludedNoise)
	require.Equal(t, "playground/glob-import/root/array-common-base/pattern1/a.js", got.ExcludedNoise[0].Path,
		"demoted wrong row: %#v", got.ExcludedNoise)
	require.Equal(t, "1", got.Metadata[packNegativeEvidenceCountKey],
		"missing negative evidence metadata: %#v", got.Metadata)
	assert.Equal(t, 1, got.Counts[PackRoleImplementation])
	assert.Equal(t, 1, got.Counts[PackRoleExcludedNoise])

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
	require.Empty(t, got.ExcludedNoise,
		"playground row should be kept when requested: %#v", got.ExcludedNoise)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Items, 1)

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
	require.Empty(t, got.ExcludedNoise,
		"normal test should not be demoted: %#v", got.ExcludedNoise)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Items, 1)

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
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Items, 1)
	require.Equal(t, "docs/notes/next_epoch_decision_memo.md", got.Groups[0].Items[0].Path,
		"kept wrong active row: %#v", got.Groups[0].Items)
	require.Len(t, got.ExcludedNoise, 1)
	assert.Equal(t, "docs/plans/D4.2-blocked-external-validity-bridge.md", got.ExcludedNoise[0].Path)
	require.NotEqual(t, "", got.ExcludedNoise[0].RoleReason,
		"expected downgrade reason, got %#v", got.ExcludedNoise[0])

}

func TestApplyDemotionOnlyNegativeEvidenceKeepsBlockedIntentWhenCurrentDecisionAbsent(t *testing.T) {
	pack := RoleGroupedPack{
		Groups: []PackGroup{
			{
				Role:   PackRoleOpenWork,
				Title:  PackRoleTitle(PackRoleOpenWork),
				Budget: 3,
				Items: []PackItem{
					{
						OriginalRank: 1,
						ID:           "blocked",
						Path:         "docs/plans/D4.2-blocked-external-validity-bridge.md",
						Title:        "D4.2 blocked external validity bridge",
						Status:       "blocked",
						Role:         PackRoleOpenWork,
						Reasons:      []string{"authority prior: blocked, closed, stale, or superseded"},
					},
					{
						OriginalRank: 2,
						ID:           "historical",
						Path:         "docs/plans/PLAN-008.1-synthetic-repo-world.md",
						Title:        "PLAN-008.1 synthetic repo world",
						Role:         PackRoleOpenWork,
					},
				},
			},
		},
	}

	got := ApplyDemotionOnlyNegativeEvidence(pack, "epoch 4 external validity bridge")
	require.Empty(t, got.ExcludedNoise,
		"blocked plan should stay visible when no current decision context exists, got %#v", got.ExcludedNoise)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Items, 2)
	require.Equal(t, "", got.Metadata[packNegativeEvidenceCountKey],
		"negative evidence should not fire without active decision context: %#v", got.Metadata)

}
