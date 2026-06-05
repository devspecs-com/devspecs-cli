package commands

import "testing"

func TestNormalizeFindSourcePackMode(t *testing.T) {
	tests := map[string]string{
		"":                    findSourcePackModeOff,
		"off":                 findSourcePackModeOff,
		"false":               findSourcePackModeOff,
		"compact":             findSourcePackModeCompactManifestV0,
		"source_manifest":     findSourcePackModeCompactManifestV0,
		"compact_manifest_v0": findSourcePackModeCompactManifestV0,
		"wat":                 "",
	}
	for in, want := range tests {
		if got := normalizeFindSourcePackMode(in); got != want {
			t.Fatalf("normalizeFindSourcePackMode(%q) = %q, want %q", in, got, want)
		}
	}
}
