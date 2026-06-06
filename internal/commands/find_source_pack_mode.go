package commands

import "strings"

const (
	findSourcePackModeOff               = "off"
	findSourcePackModeCompactManifestV0 = "compact_manifest_v0"
	findSourcePackModeCompactManifestV1 = "compact_manifest_v1"
	findSourcePackModeCompactManifestV2 = "compact_manifest_v2"
)

func normalizeFindSourcePackMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off", "none", "false", "0":
		return findSourcePackModeOff
	case "compact", "manifest", "source_manifest", "source_manifest_v0", "compact_manifest", "compact_manifest_v0":
		return findSourcePackModeCompactManifestV0
	case "source_manifest_v1", "compact_manifest_v1", "compact-v1", "manifest-v1":
		return findSourcePackModeCompactManifestV1
	case "source_manifest_v2", "compact_manifest_v2", "compact-v2", "manifest-v2":
		return findSourcePackModeCompactManifestV2
	default:
		return ""
	}
}

func validFindSourcePackModes() []string {
	return []string{findSourcePackModeOff, findSourcePackModeCompactManifestV0, findSourcePackModeCompactManifestV1, findSourcePackModeCompactManifestV2}
}
