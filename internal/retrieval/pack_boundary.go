package retrieval

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	BoundaryPrimaryPackMode = "role_grouped_pack_v0_boundary_primary_v0"

	boundaryPrimaryTarget  = 8
	boundaryPrimarySoftMax = 10
)

var boundaryPrimaryRoleBudgets = map[string]int{
	PackRoleBackgroundDecisions: 3,
	PackRoleImplementation:      6,
	PackRoleBehaviorTests:       4,
	PackRoleConfigSchema:        2,
	PackRoleOpenWork:            2,
	PackRoleSupportingContext:   2,
}

// PackBoundarySummary is a compact description of related context hidden from
// the default boundary-primary human view. The underlying items stay present in
// JSON and verbose output.
type PackBoundarySummary struct {
	Role     string   `json:"role"`
	Title    string   `json:"title,omitempty"`
	Boundary string   `json:"boundary"`
	Count    int      `json:"count"`
	Examples []string `json:"examples,omitempty"`
}

// ApplyBoundaryPrimaryPack tiers an already selected pack into a small primary
// view plus related context. It is presentation/ranking only: no candidates are
// admitted or removed from JSON/verbose output.
func ApplyBoundaryPrimaryPack(pack RoleGroupedPack) RoleGroupedPack {
	return ApplyBoundaryPrimaryPackForQuery(pack, "")
}

func ApplyBoundaryPrimaryPackForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	flat := flattenPackItems(pack.Groups)
	if len(flat) == 0 {
		pack.Mode = BoundaryPrimaryPackMode
		pack.Metadata = boundaryPrimaryMetadata(pack, 0, 0)
		return pack
	}

	sort.SliceStable(flat, func(i, j int) bool {
		if flat[i].item.OriginalRank == flat[j].item.OriginalRank {
			return flat[i].item.Path < flat[j].item.Path
		}
		return flat[i].item.OriginalRank < flat[j].item.OriginalRank
	})

	selected := selectBoundaryPrimaryItems(flat, strings.ToLower(query))
	primaryCount := 0
	relatedCount := 0
	for groupIdx := range pack.Groups {
		for itemIdx := range pack.Groups[groupIdx].Items {
			key := packBoundaryItemKey(pack.Groups[groupIdx].Items[itemIdx])
			boundary := packItemBoundary(pack.Groups[groupIdx].Role, pack.Groups[groupIdx].Items[itemIdx])
			pack.Groups[groupIdx].Items[itemIdx].Boundary = boundary
			if selected[key] {
				pack.Groups[groupIdx].Items[itemIdx].PackTier = PackTierPrimary
				primaryCount++
			} else {
				pack.Groups[groupIdx].Items[itemIdx].PackTier = PackTierRelated
				relatedCount++
			}
		}
		sort.SliceStable(pack.Groups[groupIdx].Items, func(i, j int) bool {
			left := pack.Groups[groupIdx].Items[i]
			right := pack.Groups[groupIdx].Items[j]
			leftRelated := left.PackTier == PackTierRelated
			rightRelated := right.PackTier == PackTierRelated
			if leftRelated != rightRelated {
				return !leftRelated
			}
			if left.OriginalRank == right.OriginalRank {
				return left.Path < right.Path
			}
			return left.OriginalRank < right.OriginalRank
		})
	}

	pack.Mode = BoundaryPrimaryPackMode
	pack.Metadata = boundaryPrimaryMetadata(pack, primaryCount, relatedCount)
	pack.Notes = appendBoundaryPrimaryNote(pack.Notes, primaryCount, relatedCount)
	return pack
}

func IsBoundaryPrimaryPack(pack RoleGroupedPack) bool {
	return pack.Mode == BoundaryPrimaryPackMode || (pack.Metadata != nil && pack.Metadata["boundary_primary"] == "true")
}

func PackItemIsRelated(item PackItem) bool {
	return strings.EqualFold(item.PackTier, PackTierRelated)
}

func BoundaryRelatedSummaries(pack RoleGroupedPack) []PackBoundarySummary {
	summaries := map[string]*PackBoundarySummary{}
	order := make([]string, 0)
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if !PackItemIsRelated(item) {
				continue
			}
			boundary := item.Boundary
			if boundary == "" {
				boundary = packItemBoundary(group.Role, item)
			}
			key := group.Role + "\x00" + boundary
			summary := summaries[key]
			if summary == nil {
				summary = &PackBoundarySummary{
					Role:     group.Role,
					Title:    PackRoleTitle(group.Role),
					Boundary: boundary,
				}
				summaries[key] = summary
				order = append(order, key)
			}
			summary.Count++
			if len(summary.Examples) < 2 && !packBoundaryContainsString(summary.Examples, packBoundaryItemLabel(item)) {
				summary.Examples = append(summary.Examples, packBoundaryItemLabel(item))
			}
		}
	}
	out := make([]PackBoundarySummary, 0, len(order))
	for _, key := range order {
		out = append(out, *summaries[key])
	}
	return out
}

type boundaryPackItem struct {
	groupRole string
	item      PackItem
	boundary  string
	class     string
}

func flattenPackItems(groups []PackGroup) []boundaryPackItem {
	out := make([]boundaryPackItem, 0)
	for _, group := range groups {
		for _, item := range group.Items {
			boundary := packItemBoundary(group.Role, item)
			out = append(out, boundaryPackItem{
				groupRole: group.Role,
				item:      item,
				boundary:  boundary,
				class:     packBoundaryClass(group.Role, item),
			})
		}
	}
	return out
}

func selectBoundaryPrimaryItems(items []boundaryPackItem, queryLower string) map[string]bool {
	selected := map[string]bool{}
	roleCounts := map[string]int{}
	boundaryCounts := map[string]int{}

	for _, entry := range items {
		if len(selected) >= boundaryPrimarySoftMax {
			break
		}
		key := packBoundaryItemKey(entry.item)
		if key == "" || selected[key] {
			continue
		}
		protected := boundaryProtectedClass(entry.class) || boundaryFamilyAnchor(entry, queryLower)
		if len(selected) >= boundaryPrimaryTarget && !protected {
			continue
		}
		if !protected && !boundaryRoleBudgetAllows(roleCounts, entry.groupRole) {
			continue
		}
		if !protected && !boundaryCapAllows(boundaryCounts, entry) {
			continue
		}
		selected[key] = true
		roleCounts[entry.groupRole]++
		boundaryCounts[entry.groupRole+"\x00"+entry.boundary]++
	}

	if len(selected) == 0 && len(items) > 0 {
		selected[packBoundaryItemKey(items[0].item)] = true
	}
	return selected
}

func boundaryRoleBudgetAllows(roleCounts map[string]int, role string) bool {
	budget := boundaryPrimaryRoleBudgets[role]
	if budget <= 0 {
		budget = 1
	}
	return roleCounts[role] < budget
}

func boundaryCapAllows(boundaryCounts map[string]int, entry boundaryPackItem) bool {
	cap := 1
	switch entry.class {
	case "source":
		cap = 6
	case "test":
		cap = 4
	case "config", "script":
		cap = 2
	case "doc":
		cap = 2
	}
	return boundaryCounts[entry.groupRole+"\x00"+entry.boundary] < cap
}

func boundaryProtectedClass(class string) bool {
	return class == "source" || class == "test"
}

func boundaryFamilyAnchor(entry boundaryPackItem, queryLower string) bool {
	pathLower := strings.ToLower(filepath.ToSlash(entry.item.Path))
	titleLower := strings.ToLower(strings.TrimSpace(entry.item.Title))
	base := strings.ToLower(filepath.Base(pathLower))
	if pathLower == "" && titleLower == "" {
		return false
	}
	if queryRequestsProposalDesign(queryLower) {
		if base == "readme.md" && (hasPathSegment(pathLower, "beps") || hasPathSegment(pathLower, "bep") || hasPathSegment(pathLower, "rfcs") || hasPathSegment(pathLower, "rfc") || hasPathSegment(pathLower, "proposals") || hasPathSegment(pathLower, "proposal")) {
			return true
		}
		if containsAny(titleLower, "enhancement proposals", "request for comments", "proposal process") {
			return true
		}
	}
	if hasNonIntentModeIntent(queryLower, "protocol") || queryRequestsAgentInstructions(queryLower) || queryRequestsProtocol(queryLower) {
		switch base {
		case "agents.md", "claude.md", "codex.md", "contributing.md", "contribution.md":
			return true
		}
	}
	if strings.Contains(queryLower, "openspec") || strings.Contains(queryLower, "open spec") {
		return pathLower == "openspec" || base == "openspec.md" || (base == "readme.md" && hasPathSegment(pathLower, "openspec"))
	}
	return false
}

func packItemBoundary(role string, item PackItem) string {
	class := packBoundaryClass(role, item)
	path := strings.ToLower(filepath.ToSlash(strings.TrimSpace(item.Path)))
	if path == "" {
		return class + ":unknown"
	}
	parts := splitPathParts(path)
	if len(parts) == 0 {
		return class + ":unknown"
	}
	switch {
	case len(parts) >= 3 && parts[0] == "openspec" && parts[1] == "changes":
		if len(parts) >= 5 && parts[3] == "specs" {
			return class + ":openspec/changes/" + parts[2] + "/specs/" + parts[4]
		}
		if len(parts) >= 4 {
			return class + ":openspec/changes/" + parts[2] + "/" + strings.TrimSuffix(parts[3], filepath.Ext(parts[3]))
		}
		return class + ":openspec/changes/" + parts[2]
	case len(parts) >= 2 && (parts[0] == ".github" || parts[0] == ".claude" || parts[0] == ".codex"):
		return class + ":" + strings.Join(parts[:2], "/")
	case len(parts) == 1:
		return class + ":" + strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
	case class == "doc" || class == "support" || class == "trace":
		dirParts := parts[:len(parts)-1]
		if len(dirParts) == 0 {
			return class + ":" + strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
		}
		return class + ":" + strings.Join(dirParts[:minInt(len(dirParts), 3)], "/")
	default:
		dirParts := parts[:len(parts)-1]
		if len(dirParts) == 0 {
			return class + ":" + strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
		}
		return class + ":" + strings.Join(dirParts[:minInt(len(dirParts), 2)], "/")
	}
}

func packBoundaryClass(role string, item PackItem) string {
	switch role {
	case PackRoleImplementation:
		if packPathLooksScript(item.Path) {
			return "script"
		}
		return "source"
	case PackRoleBehaviorTests:
		return "test"
	case PackRoleConfigSchema:
		return "config"
	case PackRoleOpenWork:
		return "doc"
	case PackRoleBackgroundDecisions:
		return "doc"
	case PackRoleSupportingContext:
		return "support"
	default:
		return "other"
	}
}

func packPathLooksScript(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sh", ".ps1", ".bat", ".cmd", ".sql":
		return true
	default:
		return false
	}
}

func splitPathParts(path string) []string {
	raw := strings.Split(filepath.ToSlash(path), "/")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" && part != "." {
			out = append(out, part)
		}
	}
	return out
}

func packBoundaryItemKey(item PackItem) string {
	if item.ID != "" {
		return "id:" + item.ID
	}
	if item.Path != "" {
		return "path:" + filepath.ToSlash(item.Path)
	}
	if item.Title != "" {
		return "title:" + item.Title
	}
	return ""
}

func packBoundaryItemLabel(item PackItem) string {
	if title := strings.TrimSpace(item.Title); title != "" {
		return title
	}
	if path := strings.TrimSpace(item.Path); path != "" {
		return filepath.ToSlash(path)
	}
	if id := strings.TrimSpace(item.ShortID); id != "" {
		return id
	}
	return "related artifact"
}

func boundaryPrimaryMetadata(pack RoleGroupedPack, primaryCount, relatedCount int) map[string]string {
	metadata := map[string]string{}
	for k, v := range pack.Metadata {
		metadata[k] = v
	}
	metadata["boundary_primary"] = "true"
	metadata["boundary_primary_target"] = strconv.Itoa(boundaryPrimaryTarget)
	metadata["boundary_primary_count"] = strconv.Itoa(primaryCount)
	metadata["boundary_related_count"] = strconv.Itoa(relatedCount)
	return metadata
}

func appendBoundaryPrimaryNote(notes []string, primaryCount, relatedCount int) []string {
	note := fmt.Sprintf("Boundary-primary view shows %d primary artifact(s)", primaryCount)
	if relatedCount > 0 {
		note += fmt.Sprintf(" and summarizes %d related artifact(s).", relatedCount)
	} else {
		note += "."
	}
	for _, existing := range notes {
		if existing == note {
			return notes
		}
	}
	return append(notes, note)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func packBoundaryContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
