package retrieval

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	FamilyPrimaryPackMode = "role_grouped_pack_v0_family_primary_v0"

	familyPrimaryCap          = 6
	familyPrimaryStrongFamily = 4
)

var familyPrimaryPresentationStopTerms = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "before": true, "by": true, "can": true, "change": true,
	"changes": true, "code": true, "correct": true, "correctly": true,
	"do": true, "does": true, "done": true, "file": true, "files": true,
	"fix": true, "for": true, "from": true, "get": true, "handle": true,
	"handling": true, "helper": true, "helpers": true, "if": true, "in": true,
	"index": true, "into": true, "is": true, "it": true, "make": true,
	"manager": true, "new": true, "node": true, "of": true, "old": true,
	"on": true, "or": true, "set": true, "sets": true, "should": true,
	"support": true, "the": true, "to": true, "update": true, "use": true,
	"uses": true, "using": true, "util": true, "utils": true, "when": true,
	"with": true, "without": true, "work": true,
}

var familyPrimarySourceExts = map[string]bool{
	".c": true, ".cc": true, ".cpp": true, ".cs": true, ".css": true,
	".go": true, ".java": true, ".js": true, ".jsx": true, ".kt": true,
	".php": true, ".py": true, ".rs": true, ".scss": true, ".swift": true,
	".ts": true, ".tsx": true, ".vue": true,
}

type familyPrimaryEntry struct {
	groupIdx int
	itemIdx  int
	role     string
	item     PackItem
	family   string
	class    string
	score    float64
}

func ApplyFamilyPrimaryPackForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	entries := familyPrimaryEntries(pack, query)
	if len(entries) == 0 {
		pack.Mode = FamilyPrimaryPackMode
		pack.Metadata = familyPrimaryMetadata(pack, nil, 0, 0)
		return pack
	}

	selected := selectFamilyPrimaryEntries(entries)
	primaryCount := 0
	relatedCount := 0
	for groupIdx := range pack.Groups {
		for itemIdx := range pack.Groups[groupIdx].Items {
			item := &pack.Groups[groupIdx].Items[itemIdx]
			key := packBoundaryItemKey(*item)
			entry := entries[key]
			if entry != nil {
				item.Boundary = entry.family
			}
			if selected[key] {
				item.PackTier = PackTierPrimary
				primaryCount++
			} else {
				item.PackTier = PackTierRelated
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

	anchors, suppressed := familyPrimaryQueryAnchors(query)
	pack.Mode = FamilyPrimaryPackMode
	pack.Metadata = familyPrimaryMetadata(pack, anchors, primaryCount, relatedCount)
	if len(suppressed) > 0 {
		pack.Metadata["family_primary_suppressed_anchors"] = strings.Join(suppressed, ",")
	}
	pack.Notes = appendFamilyPrimaryNote(pack.Notes, primaryCount, relatedCount)
	return pack
}

func IsFamilyPrimaryPack(pack RoleGroupedPack) bool {
	return pack.Mode == FamilyPrimaryPackMode || (pack.Metadata != nil && pack.Metadata["family_primary"] == "true")
}

func FamilyPrimaryRelatedSummaries(pack RoleGroupedPack) []PackBoundarySummary {
	summaries := map[string]*PackBoundarySummary{}
	order := make([]string, 0)
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if !PackItemIsRelated(item) {
				continue
			}
			family := item.Boundary
			if family == "" {
				family = familyPrimaryFamilyKey(group.Role, item, nil)
			}
			key := group.Role + "\x00" + family
			summary := summaries[key]
			if summary == nil {
				summary = &PackBoundarySummary{
					Role:     group.Role,
					Title:    PackRoleTitle(group.Role),
					Boundary: family,
				}
				summaries[key] = summary
				order = append(order, key)
			}
			summary.Count++
			if len(summary.Examples) < 3 && !packBoundaryContainsString(summary.Examples, packBoundaryItemLabel(item)) {
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

func familyPrimaryEntries(pack RoleGroupedPack, query string) map[string]*familyPrimaryEntry {
	anchors, _ := familyPrimaryQueryAnchors(query)
	out := map[string]*familyPrimaryEntry{}
	for groupIdx, group := range pack.Groups {
		for itemIdx, item := range group.Items {
			key := packBoundaryItemKey(item)
			if key == "" {
				continue
			}
			class := familyPrimaryClass(group.Role, item)
			entry := &familyPrimaryEntry{
				groupIdx: groupIdx,
				itemIdx:  itemIdx,
				role:     group.Role,
				item:     item,
				family:   familyPrimaryFamilyKey(group.Role, item, anchors),
				class:    class,
				score:    familyPrimaryScore(item, anchors),
			}
			out[key] = entry
		}
	}
	return out
}

func selectFamilyPrimaryEntries(entries map[string]*familyPrimaryEntry) map[string]bool {
	byFamily := map[string][]*familyPrimaryEntry{}
	for _, entry := range entries {
		byFamily[entry.family] = append(byFamily[entry.family], entry)
	}
	families := make([]string, 0, len(byFamily))
	for family := range byFamily {
		families = append(families, family)
		sort.SliceStable(byFamily[family], func(i, j int) bool {
			left := byFamily[family][i]
			right := byFamily[family][j]
			if left.score == right.score {
				if left.item.OriginalRank == right.item.OriginalRank {
					return left.item.Path < right.item.Path
				}
				return left.item.OriginalRank < right.item.OriginalRank
			}
			return left.score > right.score
		})
	}
	sort.SliceStable(families, func(i, j int) bool {
		left := byFamily[families[i]][0]
		right := byFamily[families[j]][0]
		if left.score == right.score {
			return families[i] < families[j]
		}
		return left.score > right.score
	})

	selected := map[string]bool{}
	add := func(entry *familyPrimaryEntry) {
		if len(selected) >= familyPrimaryCap || entry == nil {
			return
		}
		key := packBoundaryItemKey(entry.item)
		if key != "" {
			selected[key] = true
		}
	}

	for i, family := range families {
		if i >= familyPrimaryStrongFamily || len(selected) >= familyPrimaryCap {
			break
		}
		add(firstFamilyPrimaryClass(byFamily[family], "source"))
		add(firstFamilyPrimaryClass(byFamily[family], "test"))
	}
	for _, family := range families {
		if len(selected) >= familyPrimaryCap {
			break
		}
		for _, entry := range byFamily[family] {
			if entry.score >= 7.0 {
				add(entry)
			}
			if len(selected) >= familyPrimaryCap {
				break
			}
		}
	}
	for _, family := range families {
		if len(selected) >= minInt(familyPrimaryCap, 4) {
			break
		}
		for _, entry := range byFamily[family] {
			add(entry)
			if len(selected) >= minInt(familyPrimaryCap, 4) {
				break
			}
		}
	}
	if len(selected) == 0 {
		for _, family := range families {
			add(byFamily[family][0])
			break
		}
	}
	return selected
}

func firstFamilyPrimaryClass(entries []*familyPrimaryEntry, class string) *familyPrimaryEntry {
	for _, entry := range entries {
		if entry.class == class {
			return entry
		}
	}
	return nil
}

func familyPrimaryQueryAnchors(query string) ([]string, []string) {
	seen := map[string]bool{}
	suppressedSeen := map[string]bool{}
	var anchors []string
	var suppressed []string
	add := func(term string) {
		term = familyPrimaryCompact(term)
		if term == "" {
			return
		}
		if familyPrimaryPresentationStopTerms[term] {
			if !suppressedSeen[term] {
				suppressedSeen[term] = true
				suppressed = append(suppressed, term)
			}
			return
		}
		if len(term) < 3 || seen[term] {
			return
		}
		seen[term] = true
		anchors = append(anchors, term)
	}
	for _, term := range buildExactQueryAnchorProfile(query).Specific {
		add(term)
	}
	for _, term := range meaningfulTerms(query) {
		add(term)
	}
	for _, raw := range tokenizePreservingIdentifiers(query) {
		add(raw)
		for _, part := range splitIdentifierParts(raw) {
			add(part)
		}
	}
	return anchors, suppressed
}

func familyPrimaryFamilyKey(role string, item PackItem, anchors []string) string {
	path := filepath.ToSlash(strings.TrimSpace(item.Path))
	if path == "" {
		path = filepath.ToSlash(strings.TrimSpace(item.SourcePath))
	}
	path = strings.SplitN(path, "#", 2)[0]
	stem := familyPrimaryStem(path)
	for _, anchor := range anchors {
		if anchor != "" && anchor == stem {
			return "stem:" + stem
		}
	}
	parent := familyPrimaryParent(path)
	if parent == "" {
		if stem != "" {
			return familyPrimaryClass(role, item) + ":" + stem
		}
		return familyPrimaryClass(role, item) + ":unknown"
	}
	if stem == "" {
		return parent
	}
	return parent + "/" + stem
}

func familyPrimaryClass(role string, item PackItem) string {
	if role == PackRoleBehaviorTests || familyPrimaryTestPath(item.Path) || item.Subtype == "test_case" {
		return "test"
	}
	if role == PackRoleImplementation || familyPrimarySourceExts[strings.ToLower(filepath.Ext(strings.SplitN(item.Path, "#", 2)[0]))] {
		return "source"
	}
	return packBoundaryClass(role, item)
}

func familyPrimaryScore(item PackItem, anchors []string) float64 {
	path := filepath.ToSlash(strings.ToLower(item.Path))
	title := strings.ToLower(item.Title)
	reasons := strings.ToLower(strings.Join(item.Reasons, "\n"))
	pathCompact := familyPrimaryCompact(path)
	titleCompact := familyPrimaryCompact(title)
	reasonCompact := familyPrimaryCompact(reasons)
	tokens := familyPrimaryPathTokenSet(path)
	stem := familyPrimaryStem(path)
	var score float64
	for _, anchor := range anchors {
		switch {
		case anchor == stem:
			score += 8
		case tokens[anchor]:
			score += 3
		case strings.Contains(pathCompact, anchor):
			score += 2
		case strings.Contains(titleCompact, anchor):
			score += 2.5
		case strings.Contains(reasonCompact, anchor):
			score += 1
		}
	}
	rank := item.OriginalRank
	if rank <= 0 {
		rank = 99
	}
	rankBonus := 2.5 - float64(rank)*0.12
	if rankBonus > 0 {
		score += rankBonus
	}
	if familyPrimaryClass(item.Role, item) == "test" {
		score += 0.5
	}
	return score
}

func familyPrimaryStem(path string) string {
	path = strings.SplitN(filepath.ToSlash(path), "#", 2)[0]
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	for _, suffix := range []string{".test", ".spec", "_test", "-test"} {
		base = strings.TrimSuffix(strings.TrimSuffix(base, strings.ToUpper(suffix)), suffix)
	}
	base = strings.TrimPrefix(base, "test_")
	return familyPrimaryCompact(base)
}

func familyPrimaryParent(path string) string {
	path = strings.SplitN(filepath.ToSlash(path), "#", 2)[0]
	parts := splitPathParts(path)
	if len(parts) <= 1 {
		return ""
	}
	dirs := parts[:len(parts)-1]
	filtered := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		switch familyPrimaryCompact(dir) {
		case "test", "tests", "__tests__", "spec", "specs", "e2e":
			continue
		default:
			filtered = append(filtered, dir)
		}
	}
	if len(filtered) > 3 {
		filtered = filtered[len(filtered)-3:]
	}
	return strings.ToLower(strings.Join(filtered, "/"))
}

func familyPrimaryPathTokenSet(path string) map[string]bool {
	out := map[string]bool{}
	for _, part := range splitPathParts(filepath.ToSlash(path)) {
		for _, token := range splitIdentifierParts(part) {
			if compact := familyPrimaryCompact(token); compact != "" {
				out[compact] = true
			}
		}
		if compact := familyPrimaryCompact(part); compact != "" {
			out[compact] = true
		}
	}
	return out
}

func familyPrimaryCompact(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func familyPrimaryTestPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(strings.SplitN(path, "#", 2)[0]))
	base := filepath.Base(path)
	return strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/__tests__/") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.")
}

func familyPrimaryMetadata(pack RoleGroupedPack, anchors []string, primaryCount, relatedCount int) map[string]string {
	metadata := map[string]string{}
	for k, v := range pack.Metadata {
		metadata[k] = v
	}
	metadata["family_primary"] = "true"
	metadata["family_primary_cap"] = strconv.Itoa(familyPrimaryCap)
	metadata["family_primary_count"] = strconv.Itoa(primaryCount)
	metadata["family_related_count"] = strconv.Itoa(relatedCount)
	if len(anchors) > 0 {
		metadata["family_primary_anchors"] = strings.Join(anchors, ",")
	}
	return metadata
}

func appendFamilyPrimaryNote(notes []string, primaryCount, relatedCount int) []string {
	note := fmt.Sprintf("Family-primary view shows %d primary artifact(s)", primaryCount)
	if relatedCount > 0 {
		note += fmt.Sprintf(" and keeps %d related artifact(s) summarized.", relatedCount)
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
