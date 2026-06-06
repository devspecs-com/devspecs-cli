package commands

import (
	"strconv"
	"strings"
)

const (
	findPackPresentationModeOff             = "off"
	findPackPresentationModeFamilyPrimaryV0 = "family_primary_v0"
	findPackPresentationModeFamilyPrimaryV1 = "family_primary_v1"
)

func normalizeFindPackPresentationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "none", "off":
		return findPackPresentationModeOff
	case "family_primary", "family-primary", "family_primary_v0", "family-primary-v0", "family":
		return findPackPresentationModeFamilyPrimaryV0
	case "family_primary_v1", "family-primary-v1", "family_v1", "family-v1":
		return findPackPresentationModeFamilyPrimaryV1
	default:
		return ""
	}
}

func validFindPackPresentationModes() []string {
	return []string{findPackPresentationModeOff, findPackPresentationModeFamilyPrimaryV0, findPackPresentationModeFamilyPrimaryV1}
}

func metadataInt(metadata map[string]string, key string) int {
	if metadata == nil {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(metadata[key]))
	return value
}
