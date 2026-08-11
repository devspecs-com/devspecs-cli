package commands

import (
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFindPackScoutMode_WithEmptyInput_ReturnsOff(t *testing.T) {
	got := normalizeFindPackScoutMode("")

	assert.Equal(t, findPackScoutModeOff, got)
}

func TestNormalizeFindPackScoutMode_WithOff_ReturnsOff(t *testing.T) {
	got := normalizeFindPackScoutMode("off")

	assert.Equal(t, findPackScoutModeOff, got)
}

func TestNormalizeFindPackScoutMode_WithFalse_ReturnsOff(t *testing.T) {
	got := normalizeFindPackScoutMode("false")

	assert.Equal(t, findPackScoutModeOff, got)
}

func TestNormalizeFindPackScoutMode_WithBeta_ReturnsBetaV0(t *testing.T) {
	got := normalizeFindPackScoutMode("beta")

	assert.Equal(t, findPackScoutModeBetaV0, got)
}

func TestNormalizeFindPackScoutMode_WithUnderscoreBetaV0_ReturnsBetaV0(t *testing.T) {
	got := normalizeFindPackScoutMode("beta_v0")

	assert.Equal(t, findPackScoutModeBetaV0, got)
}

func TestNormalizeFindPackScoutMode_WithHyphenatedBetaV0_ReturnsBetaV0(t *testing.T) {
	got := normalizeFindPackScoutMode("beta-v0")

	assert.Equal(t, findPackScoutModeBetaV0, got)
}

func TestNormalizeFindPackScoutMode_WithScout_ReturnsBetaV0(t *testing.T) {
	got := normalizeFindPackScoutMode("scout")

	assert.Equal(t, findPackScoutModeBetaV0, got)
}

func TestNormalizeFindPackScoutMode_WithQ06_ReturnsBetaV0(t *testing.T) {
	got := normalizeFindPackScoutMode("q06")

	assert.Equal(t, findPackScoutModeBetaV0, got)
}

func TestNormalizeFindPackScoutMode_WithUnknownInput_ReturnsEmpty(t *testing.T) {
	got := normalizeFindPackScoutMode("wat")

	assert.Empty(t, got)
}

func TestResolveFindPackScoutModeDefaultsToBetaForPack(t *testing.T) {
	got := resolveFindPackScoutMode(findPackScoutModeOff, true, false, "")
	assert.Equal(t, findPackScoutModeBetaV0, got,
		"default pack scout mode = %q", got)

}

func TestResolveFindPackScoutModeLeavesNonPackOff(t *testing.T) {
	got := resolveFindPackScoutMode(findPackScoutModeOff, false, false, "")
	assert.Equal(t, findPackScoutModeOff, got,
		"default non-pack scout mode = %q", got)

}

func TestResolveFindPackScoutModePreservesExplicitOff(t *testing.T) {
	got := resolveFindPackScoutMode(findPackScoutModeOff, true, true, "")
	assert.Equal(t, findPackScoutModeOff, got,
		"explicit off scout mode = %q", got)

}

func TestResolveFindPackScoutModePreservesEnvOverride(t *testing.T) {
	got := resolveFindPackScoutMode(findPackScoutModeOff, true, false, "off")
	assert.Equal(t, findPackScoutModeOff, got,
		"env override scout mode = %q", got)

}

func TestApplyFindPackScoutPresetSetsQ06Baseline(t *testing.T) {
	opts := findPackScoutPresetOptions{}
	applyFindPackScoutPreset(findPackScoutModeBetaV0, &opts)
	assert.Equal(t, findSourcePackModeCompactManifestV2, opts.SourcePackMode,
		"source pack mode = %q", opts.SourcePackMode)
	assert.Equal(t, findPackPresentationModeFamilyPrimaryV1, opts.PackPresentationMode,
		"pack presentation mode = %q", opts.PackPresentationMode)

}

func TestApplyFindPackScoutPresetPreservesExplicitOverrides(t *testing.T) {
	opts := findPackScoutPresetOptions{
		SourcePackMode:             findSourcePackModeCompactManifestV1,
		SourcePackConfigured:       true,
		PackPresentationMode:       findPackPresentationModeFamilyPrimaryV2,
		PackPresentationConfigured: true,
	}
	applyFindPackScoutPreset(findPackScoutModeBetaV0, &opts)
	assert.Equal(t, findSourcePackModeCompactManifestV1, opts.SourcePackMode,
		"source pack mode override was not preserved: %#v", opts)
	assert.Equal(t, findPackPresentationModeFamilyPrimaryV2, opts.PackPresentationMode,
		"presentation override was not preserved: %#v", opts)

}

func TestFindCommandKeepsPackScoutFlagInternal(t *testing.T) {
	cmd := NewFindCmd()
	flag := cmd.Flags().Lookup("pack-scout")
	require.NotNil(t, flag,
		"missing --pack-scout flag")
	assert.True(t, flag.Hidden,
		"--pack-scout should stay hidden from public find help")

}

func TestWriteFindPackTextShowsScoutContract(t *testing.T) {
	pack := retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0_family_primary_v1",
		Summary: retrieval.PackSummary{
			IncludedCount:     1,
			RoleDiversity:     1,
			HasImplementation: true,
		},
		Metadata: map[string]string{
			"pack_scout_mode": findPackScoutModeBetaV0,
		},
		Groups: []retrieval.PackGroup{{
			Role:  retrieval.PackRoleImplementation,
			Title: retrieval.PackRoleTitle(retrieval.PackRoleImplementation),
			Items: []retrieval.PackItem{{
				OriginalRank: 1,
				ID:           "auth",
				Path:         "internal/auth/session.go",
				Title:        "internal/auth/session.go",
				Role:         retrieval.PackRoleImplementation,
			}},
		}},
	}
	var b strings.Builder
	{
		err := writeFindPackText(&b, "auth session", "test", pack, nil, nil, false)
		require.NoError(t, err)
	}
	assert.Contains(t, b.String(), "Scout: beta first working set",
		"missing scout contract:\n%s", b.String())

}

func TestWriteFindPackTextShowsScoutUncertainty(t *testing.T) {
	pack := retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0_family_primary_v1",
		Summary: retrieval.PackSummary{
			IncludedCount:     4,
			RoleDiversity:     2,
			HasImplementation: true,
			HasBehaviorTests:  true,
		},
		Metadata: map[string]string{
			"pack_scout_mode":                        findPackScoutModeBetaV0,
			retrieval.PackScoutUncertaintyKey:        "true",
			retrieval.PackScoutUncertaintyReasonsKey: "implementation surface is thin relative to tests (1 source, 3 tests)",
		},
		Groups: []retrieval.PackGroup{{
			Role:  retrieval.PackRoleImplementation,
			Title: retrieval.PackRoleTitle(retrieval.PackRoleImplementation),
			Items: []retrieval.PackItem{{
				OriginalRank: 1,
				ID:           "selection",
				Path:         "src/textual/selection.py",
				Title:        "src/textual/selection.py",
				Role:         retrieval.PackRoleImplementation,
			}},
		}},
	}
	var b strings.Builder
	{
		err := writeFindPackText(&b, "Fix selection disappearing", "test", pack, nil, nil, false)
		require.NoError(t, err)
	}
	assert.Contains(t, b.String(), "Scout uncertainty: implementation surface is thin relative to tests",
		"missing scout uncertainty:\n%s", b.String())

}

func TestFindPackOutputIncludesScoutMode(t *testing.T) {
	out := findPackOutput("auth", "test", nil, nil, retrieval.RoleGroupedPack{}, findPackScoutModeBetaV0)
	assert.Equal(t, "beta", out.ScoutMode,
		"scout mode = %q", out.ScoutMode)

}

func TestFindPackOutputIncludesScoutWarnings(t *testing.T) {
	pack := retrieval.RoleGroupedPack{
		Metadata: map[string]string{
			retrieval.PackScoutUncertaintyKey:        "true",
			retrieval.PackScoutUncertaintyReasonsKey: "no primary behavior tests are visible",
		},
	}
	out := findPackOutput("auth", "test", nil, nil, pack, findPackScoutModeBetaV0)

	require.Len(t, out.ScoutWarnings, 1)
	assert.Equal(t, "no primary behavior tests are visible", out.ScoutWarnings[0])

}
