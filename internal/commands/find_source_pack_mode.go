package commands

import "strings"

const (
	findSourcePackModeOff               = "off"
	findSourcePackModeCompactManifestV0 = "compact_manifest_v0"
)

func normalizeFindSourcePackMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off", "none", "false", "0":
		return findSourcePackModeOff
	case "compact", "manifest", "source_manifest", "source_manifest_v0", "compact_manifest", "compact_manifest_v0":
		return findSourcePackModeCompactManifestV0
	default:
		return ""
	}
}

func validFindSourcePackModes() []string {
	return []string{findSourcePackModeOff, findSourcePackModeCompactManifestV0}
}
