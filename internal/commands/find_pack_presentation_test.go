package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeFindPackPresentationMode_WithEmptyInput_ReturnsOff(t *testing.T) {
	got := normalizeFindPackPresentationMode("")

	assert.Equal(t, findPackPresentationModeOff, got)
}

func TestNormalizeFindPackPresentationMode_WithOff_ReturnsOff(t *testing.T) {
	got := normalizeFindPackPresentationMode("off")

	assert.Equal(t, findPackPresentationModeOff, got)
}

func TestNormalizeFindPackPresentationMode_WithFamily_ReturnsFamilyPrimaryV0(t *testing.T) {
	got := normalizeFindPackPresentationMode("family")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV0, got)
}

func TestNormalizeFindPackPresentationMode_WithUnderscoreFamilyPrimaryV0_ReturnsFamilyPrimaryV0(t *testing.T) {
	got := normalizeFindPackPresentationMode("family_primary_v0")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV0, got)
}

func TestNormalizeFindPackPresentationMode_WithHyphenatedFamilyPrimaryV0_ReturnsFamilyPrimaryV0(t *testing.T) {
	got := normalizeFindPackPresentationMode("family-primary-v0")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV0, got)
}

func TestNormalizeFindPackPresentationMode_WithUnderscoreFamilyPrimaryV1_ReturnsFamilyPrimaryV1(t *testing.T) {
	got := normalizeFindPackPresentationMode("family_primary_v1")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV1, got)
}

func TestNormalizeFindPackPresentationMode_WithFamilyV1_ReturnsFamilyPrimaryV1(t *testing.T) {
	got := normalizeFindPackPresentationMode("family-v1")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV1, got)
}

func TestNormalizeFindPackPresentationMode_WithUnderscoreFamilyPrimaryV2_ReturnsFamilyPrimaryV2(t *testing.T) {
	got := normalizeFindPackPresentationMode("family_primary_v2")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV2, got)
}

func TestNormalizeFindPackPresentationMode_WithFamilyV2_ReturnsFamilyPrimaryV2(t *testing.T) {
	got := normalizeFindPackPresentationMode("family-v2")

	assert.Equal(t, findPackPresentationModeFamilyPrimaryV2, got)
}

func TestNormalizeFindPackPresentationMode_WithUnknownInput_ReturnsEmpty(t *testing.T) {
	got := normalizeFindPackPresentationMode("wat")

	assert.Empty(t, got)
}
