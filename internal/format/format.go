// Package format defines closed vocabulary for artifact format_profile and layout grouping hints.
package format

import (
	"path/filepath"
	"strings"
)

// Canonical format_profile values (ingestion / layout convention).
const (
	ProfileGeneric    = "generic"
	ProfileCursorPlan = "cursor_plan"
	ProfileSpeckit    = "speckit"
	ProfileBmad       = "bmad"
	ProfileOpenspec   = "openspec"
	ProfileADR        = "adr"
	ProfileClaude     = "claude"
	ProfileCodex      = "codex"
)

var knownProfiles = map[string]string{
	ProfileGeneric:    ProfileGeneric,
	ProfileCursorPlan: ProfileCursorPlan,
	"cursor":          ProfileCursorPlan,
	"cursor-plan":     ProfileCursorPlan,
	"cursor-desktop":  ProfileCursorPlan,
	ProfileSpeckit:    ProfileSpeckit,
	"spec-kit":        ProfileSpeckit,
	"github-spec-kit": ProfileSpeckit,
	ProfileBmad:       ProfileBmad,
	"bmad-method":     ProfileBmad,
	ProfileOpenspec:   ProfileOpenspec,
	ProfileADR:        ProfileADR,
	ProfileClaude:     ProfileClaude,
	"claude-desktop":  ProfileClaude,
	ProfileCodex:      ProfileCodex,
	"codex-desktop":   ProfileCodex,
}

// Normalize returns a canonical profile slug or ProfileGeneric for unknown input.
func Normalize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ProfileGeneric
	}
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if canon, ok := knownProfiles[s]; ok {
		return canon
	}
	return ProfileGeneric
}

// FromPath infers format_profile from repository-relative path (slash-normalized).
func FromPath(relPath string) string {
	norm := filepath.ToSlash(relPath)

	if strings.Contains(norm, "_bmad-output/") {
		return ProfileBmad
	}

	dir := filepath.ToSlash(filepath.Dir(norm))
	base := filepath.Base(norm)
	if base == "spec.md" && strings.HasPrefix(dir, "specs/") && len(dir) > len("specs/") {
		return ProfileSpeckit
	}

	if strings.Contains(norm, ".cursor/plans/") {
		return ProfileCursorPlan
	}

	return ProfileGeneric
}

// FromFrontmatterTool maps generator / tool / source frontmatter strings to a profile.
// Empty strings are ignored; first non-empty wins in caller order.
func FromFrontmatterTool(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n := Normalize(slugFromRough(v))
		if n != ProfileGeneric {
			return n
		}
	}
	return ProfileGeneric
}

func slugFromRough(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// LayoutGroup returns a repo-relative grouping key for multi-file layouts (optional).
func LayoutGroup(relPath string) string {
	norm := filepath.ToSlash(relPath)

	if strings.Contains(norm, "_bmad-output/planning-artifacts/") {
		i := strings.Index(norm, "_bmad-output/planning-artifacts/")
		base := "_bmad-output/planning-artifacts"
		if i >= 0 {
			return base
		}
	}

	dir := filepath.ToSlash(filepath.Dir(norm))
	base := filepath.Base(norm)
	if base == "spec.md" && strings.HasPrefix(dir, "specs/") && len(dir) > len("specs/") {
		// specs/001-feature[/...]
		rest := strings.TrimPrefix(dir, "specs/")
		if idx := strings.Index(rest, "/"); idx >= 0 {
			return "specs/" + rest[:idx]
		}
		return "specs/" + rest
	}

	return ""
}
