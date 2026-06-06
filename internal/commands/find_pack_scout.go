package commands

import "strings"

const (
	findPackScoutModeOff    = "off"
	findPackScoutModeBetaV0 = "beta_v0"
)

type findPackScoutPresetOptions struct {
	SourcePackMode             string
	SourcePackConfigured       bool
	PackPresentationMode       string
	PackPresentationConfigured bool
}

func normalizeFindPackScoutMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "none", "off":
		return findPackScoutModeOff
	case "beta", "beta_v0", "beta-v0", "scout", "q06":
		return findPackScoutModeBetaV0
	default:
		return ""
	}
}

func validFindPackScoutModes() []string {
	return []string{findPackScoutModeOff, findPackScoutModeBetaV0}
}

func applyFindPackScoutPreset(mode string, opts *findPackScoutPresetOptions) {
	if opts == nil {
		return
	}
	switch normalizeFindPackScoutMode(mode) {
	case findPackScoutModeBetaV0:
		if !opts.SourcePackConfigured {
			opts.SourcePackMode = findSourcePackModeCompactManifestV2
		}
		if !opts.PackPresentationConfigured {
			opts.PackPresentationMode = findPackPresentationModeFamilyPrimaryV1
		}
	}
}

func findPackScoutModeFromMetadata(metadata map[string]string) string {
	if metadata == nil {
		return ""
	}
	return normalizeFindPackScoutMode(metadata["pack_scout_mode"])
}

func findPackScoutDisplayName(mode string) string {
	switch normalizeFindPackScoutMode(mode) {
	case findPackScoutModeBetaV0:
		return "beta"
	default:
		return ""
	}
}
