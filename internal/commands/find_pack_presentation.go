package commands

import (
	"strconv"
	"strings"
)

const (
	findPackPresentationModeOff             = "off"
	findPackPresentationModeFamilyPrimaryV0 = "family_primary_v0"
)

func normalizeFindPackPresentationMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "none", "off":
		return findPackPresentationModeOff
	case "family_primary", "family-primary", "family_primary_v0", "family-primary-v0", "family":
		return findPackPresentationModeFamilyPrimaryV0
	default:
		return ""
	}
}

func validFindPackPresentationModes() []string {
	return []string{findPackPresentationModeOff, findPackPresentationModeFamilyPrimaryV0}
}

func metadataInt(metadata map[string]string, key string) int {
	if metadata == nil {
		return 0
	}
	value, _ := strconv.Atoi(strings.TrimSpace(metadata[key]))
	return value
}
