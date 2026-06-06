package commands

import (
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestNormalizeFindPackScoutMode(t *testing.T) {
	tests := map[string]string{
		"":        findPackScoutModeOff,
		"off":     findPackScoutModeOff,
		"false":   findPackScoutModeOff,
		"beta":    findPackScoutModeBetaV0,
		"beta_v0": findPackScoutModeBetaV0,
		"beta-v0": findPackScoutModeBetaV0,
		"scout":   findPackScoutModeBetaV0,
		"q06":     findPackScoutModeBetaV0,
		"wat":     "",
	}
	for in, want := range tests {
		if got := normalizeFindPackScoutMode(in); got != want {
			t.Fatalf("normalizeFindPackScoutMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyFindPackScoutPresetSetsQ06Baseline(t *testing.T) {
	opts := findPackScoutPresetOptions{}
	applyFindPackScoutPreset(findPackScoutModeBetaV0, &opts)
	if opts.SourcePackMode != findSourcePackModeCompactManifestV2 {
		t.Fatalf("source pack mode = %q", opts.SourcePackMode)
	}
	if opts.PackPresentationMode != findPackPresentationModeFamilyPrimaryV1 {
		t.Fatalf("pack presentation mode = %q", opts.PackPresentationMode)
	}
}

func TestApplyFindPackScoutPresetPreservesExplicitOverrides(t *testing.T) {
	opts := findPackScoutPresetOptions{
		SourcePackMode:             findSourcePackModeCompactManifestV1,
		SourcePackConfigured:       true,
		PackPresentationMode:       findPackPresentationModeFamilyPrimaryV2,
		PackPresentationConfigured: true,
	}
	applyFindPackScoutPreset(findPackScoutModeBetaV0, &opts)
	if opts.SourcePackMode != findSourcePackModeCompactManifestV1 {
		t.Fatalf("source pack mode override was not preserved: %#v", opts)
	}
	if opts.PackPresentationMode != findPackPresentationModeFamilyPrimaryV2 {
		t.Fatalf("presentation override was not preserved: %#v", opts)
	}
}

func TestFindCommandExposesPackScoutFlag(t *testing.T) {
	cmd := NewFindCmd()
	flag := cmd.Flags().Lookup("pack-scout")
	if flag == nil {
		t.Fatal("missing --pack-scout flag")
	}
	if flag.Hidden {
		t.Fatal("--pack-scout should be visible as the explicit beta surface")
	}
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
	if err := writeFindPackText(&b, "auth session", "test", pack, nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "Scout: beta first working set") {
		t.Fatalf("missing scout contract:\n%s", b.String())
	}
}

func TestFindPackOutputIncludesScoutMode(t *testing.T) {
	out := findPackOutput("auth", "test", nil, nil, retrieval.RoleGroupedPack{}, findPackScoutModeBetaV0)
	if out.ScoutMode != "beta" {
		t.Fatalf("scout mode = %q", out.ScoutMode)
	}
}
