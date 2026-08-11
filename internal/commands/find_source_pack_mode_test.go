package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeFindSourcePackMode_WithEmptyInput_ReturnsOff(t *testing.T) {
	got := normalizeFindSourcePackMode("")

	assert.Equal(t, findSourcePackModeOff, got)
}

func TestNormalizeFindSourcePackMode_WithOff_ReturnsOff(t *testing.T) {
	got := normalizeFindSourcePackMode("off")

	assert.Equal(t, findSourcePackModeOff, got)
}

func TestNormalizeFindSourcePackMode_WithFalse_ReturnsOff(t *testing.T) {
	got := normalizeFindSourcePackMode("false")

	assert.Equal(t, findSourcePackModeOff, got)
}

func TestNormalizeFindSourcePackMode_WithCompact_ReturnsCompactManifestV0(t *testing.T) {
	got := normalizeFindSourcePackMode("compact")

	assert.Equal(t, findSourcePackModeCompactManifestV0, got)
}

func TestNormalizeFindSourcePackMode_WithSourceManifest_ReturnsCompactManifestV0(t *testing.T) {
	got := normalizeFindSourcePackMode("source_manifest")

	assert.Equal(t, findSourcePackModeCompactManifestV0, got)
}

func TestNormalizeFindSourcePackMode_WithCompactManifestV0_ReturnsCompactManifestV0(t *testing.T) {
	got := normalizeFindSourcePackMode("compact_manifest_v0")

	assert.Equal(t, findSourcePackModeCompactManifestV0, got)
}

func TestNormalizeFindSourcePackMode_WithCompactManifestV1_ReturnsCompactManifestV1(t *testing.T) {
	got := normalizeFindSourcePackMode("compact_manifest_v1")

	assert.Equal(t, findSourcePackModeCompactManifestV1, got)
}

func TestNormalizeFindSourcePackMode_WithManifestV1_ReturnsCompactManifestV1(t *testing.T) {
	got := normalizeFindSourcePackMode("manifest-v1")

	assert.Equal(t, findSourcePackModeCompactManifestV1, got)
}

func TestNormalizeFindSourcePackMode_WithCompactManifestV2_ReturnsCompactManifestV2(t *testing.T) {
	got := normalizeFindSourcePackMode("compact_manifest_v2")

	assert.Equal(t, findSourcePackModeCompactManifestV2, got)
}

func TestNormalizeFindSourcePackMode_WithManifestV2_ReturnsCompactManifestV2(t *testing.T) {
	got := normalizeFindSourcePackMode("manifest-v2")

	assert.Equal(t, findSourcePackModeCompactManifestV2, got)
}

func TestNormalizeFindSourcePackMode_WithUnknownInput_ReturnsEmpty(t *testing.T) {
	got := normalizeFindSourcePackMode("wat")

	assert.Empty(t, got)
}
