package commands

import "testing"

func TestNormalizeFindPackPresentationMode(t *testing.T) {
	tests := map[string]string{
		"":                  findPackPresentationModeOff,
		"off":               findPackPresentationModeOff,
		"family":            findPackPresentationModeFamilyPrimaryV0,
		"family_primary_v0": findPackPresentationModeFamilyPrimaryV0,
		"family-primary-v0": findPackPresentationModeFamilyPrimaryV0,
		"family_primary_v1": findPackPresentationModeFamilyPrimaryV1,
		"family-v1":         findPackPresentationModeFamilyPrimaryV1,
		"family_primary_v2": findPackPresentationModeFamilyPrimaryV2,
		"family-v2":         findPackPresentationModeFamilyPrimaryV2,
		"wat":               "",
	}
	for in, want := range tests {
		if got := normalizeFindPackPresentationMode(in); got != want {
			t.Fatalf("normalizeFindPackPresentationMode(%q) = %q, want %q", in, got, want)
		}
	}
}
