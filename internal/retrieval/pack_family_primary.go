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
	FamilyPrimaryPackMode   = "role_grouped_pack_v0_family_primary_v0"
	FamilyPrimaryPackModeV1 = "role_grouped_pack_v0_family_primary_v1"
	FamilyPrimaryPackModeV2 = "role_grouped_pack_v0_family_primary_v2"

	familyPrimaryCap          = 6
	familyPrimaryCapV1        = 8
	familyPrimaryCapV2        = 7
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
	anchors, suppressed := familyPrimaryQueryAnchors(query)
	return applyFamilyPrimaryPackForQueryWithEntries(pack, query, FamilyPrimaryPackMode, familyPrimaryCap, false, anchors, suppressed, familyPrimaryEntriesWithAnchors(pack, anchors, false), selectFamilyPrimaryEntries)
}

func ApplyFamilyPrimaryPackV1ForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	anchors, suppressed := familyPrimaryQueryAnchors(query)
	return applyFamilyPrimaryPackForQueryWithEntries(pack, query, FamilyPrimaryPackModeV1, familyPrimaryCapV1, true, anchors, suppressed, familyPrimaryEntriesWithAnchors(pack, anchors, false), selectFamilyPrimaryEntries)
}

func ApplyFamilyPrimaryPackV2ForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	anchors, suppressed := familyPrimaryQueryAnchorsV2(query)
	return applyFamilyPrimaryPackForQueryWithEntries(pack, query, FamilyPrimaryPackModeV2, familyPrimaryCapV2, true, anchors, suppressed, familyPrimaryEntriesWithAnchors(pack, anchors, true), selectFamilyPrimaryEntriesV2)
}

func applyFamilyPrimaryPackForQueryWithEntries(pack RoleGroupedPack, query, mode string, cap int, protectExact bool, anchors, suppressed []string, entries map[string]*familyPrimaryEntry, selector func(map[string]*familyPrimaryEntry, int, bool) map[string]bool) RoleGroupedPack {
	if len(entries) == 0 {
		pack.Mode = mode
		pack.Metadata = familyPrimaryMetadata(pack, nil, 0, 0, cap, protectExact)
		return pack
	}

	selected := selector(entries, cap, protectExact)
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

	pack.Mode = mode
	pack.Metadata = familyPrimaryMetadata(pack, anchors, primaryCount, relatedCount, cap, protectExact)
	if len(suppressed) > 0 {
		pack.Metadata["family_primary_suppressed_anchors"] = strings.Join(suppressed, ",")
	}
	pack.Notes = appendFamilyPrimaryNote(pack.Notes, primaryCount, relatedCount)
	return pack
}

func IsFamilyPrimaryPack(pack RoleGroupedPack) bool {
	return pack.Mode == FamilyPrimaryPackMode || pack.Mode == FamilyPrimaryPackModeV1 || pack.Mode == FamilyPrimaryPackModeV2 || (pack.Metadata != nil && pack.Metadata["family_primary"] == "true")
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

func familyPrimaryEntriesWithAnchors(pack RoleGroupedPack, anchors []string, rarityWeighted bool) map[string]*familyPrimaryEntry {
	out := map[string]*familyPrimaryEntry{}
	var anchorDF map[string]int
	totalItems := familyPrimaryPackItemCount(pack)
	if rarityWeighted {
		anchorDF = familyPrimaryAnchorDocumentFrequency(pack, anchors)
	}
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
			}
			if rarityWeighted {
				entry.score = familyPrimaryScoreV2(item, anchors, anchorDF, totalItems)
			} else {
				entry.score = familyPrimaryScore(item, anchors)
			}
			out[key] = entry
		}
	}
	return out
}

func familyPrimaryPackItemCount(pack RoleGroupedPack) int {
	total := 0
	for _, group := range pack.Groups {
		total += len(group.Items)
	}
	if total <= 0 {
		return 1
	}
	return total
}

func selectFamilyPrimaryEntries(entries map[string]*familyPrimaryEntry, cap int, protectExact bool) map[string]bool {
	if cap <= 0 {
		cap = familyPrimaryCap
	}
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
		if len(selected) >= cap || entry == nil {
			return
		}
		key := packBoundaryItemKey(entry.item)
		if key != "" {
			selected[key] = true
		}
	}

	if protectExact {
		for _, entry := range familyPrimaryProtectedEntries(entries) {
			add(entry)
		}
	}
	for i, family := range families {
		if i >= familyPrimaryStrongFamily || len(selected) >= cap {
			break
		}
		add(firstFamilyPrimaryClass(byFamily[family], "source"))
		add(firstFamilyPrimaryClass(byFamily[family], "test"))
	}
	for _, family := range families {
		if len(selected) >= cap {
			break
		}
		for _, entry := range byFamily[family] {
			if entry.score >= 7.0 {
				add(entry)
			}
			if len(selected) >= cap {
				break
			}
		}
	}
	for _, family := range families {
		if len(selected) >= minInt(cap, 4) {
			break
		}
		for _, entry := range byFamily[family] {
			add(entry)
			if len(selected) >= minInt(cap, 4) {
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

func selectFamilyPrimaryEntriesV2(entries map[string]*familyPrimaryEntry, cap int, protectExact bool) map[string]bool {
	if cap <= 0 {
		cap = familyPrimaryCapV2
	}
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
			if left.score != right.score {
				return left.score > right.score
			}
			if left.item.OriginalRank != right.item.OriginalRank {
				return left.item.OriginalRank < right.item.OriginalRank
			}
			return left.item.Path < right.item.Path
		})
	}
	sort.SliceStable(families, func(i, j int) bool {
		left := byFamily[families[i]][0]
		right := byFamily[families[j]][0]
		if left.score != right.score {
			return left.score > right.score
		}
		return families[i] < families[j]
	})

	selected := map[string]bool{}
	add := func(entry *familyPrimaryEntry) {
		if len(selected) >= cap || entry == nil {
			return
		}
		key := packBoundaryItemKey(entry.item)
		if key != "" {
			selected[key] = true
		}
	}
	replaceWeakest := func(entry *familyPrimaryEntry) {
		if entry == nil {
			return
		}
		key := packBoundaryItemKey(entry.item)
		if key == "" || selected[key] {
			return
		}
		if len(selected) < cap {
			selected[key] = true
			return
		}
		var weakestKey string
		var weakest *familyPrimaryEntry
		for candidateKey := range selected {
			candidate := entries[candidateKey]
			if candidate == nil || familyPrimaryProtectedEntry(candidate) {
				continue
			}
			if weakest == nil ||
				candidate.score < weakest.score ||
				(candidate.score == weakest.score && candidate.item.OriginalRank > weakest.item.OriginalRank) {
				weakest = candidate
				weakestKey = candidateKey
			}
		}
		if weakest != nil && entry.score >= weakest.score+1.25 {
			delete(selected, weakestKey)
			selected[key] = true
		}
	}

	if protectExact {
		for _, entry := range familyPrimaryProtectedEntriesV2(entries) {
			add(entry)
		}
	}
	familyPrimaryAddPairClosure(selected, entries, byFamily, cap, replaceWeakest)
	for i, family := range families {
		if i >= familyPrimaryStrongFamily || len(selected) >= cap {
			break
		}
		if byFamily[family][0].score < 3.0 {
			continue
		}
		add(firstFamilyPrimaryClass(byFamily[family], "source"))
		familyPrimaryAddPairClosure(selected, entries, byFamily, cap, replaceWeakest)
		add(firstFamilyPrimaryClass(byFamily[family], "test"))
	}
	for _, family := range families {
		if len(selected) >= cap {
			break
		}
		for _, entry := range byFamily[family] {
			if entry.score >= 6.0 {
				add(entry)
				familyPrimaryAddPairClosure(selected, entries, byFamily, cap, replaceWeakest)
			}
			if len(selected) >= cap {
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

func familyPrimaryProtectedEntriesV2(entries map[string]*familyPrimaryEntry) []*familyPrimaryEntry {
	var protected []*familyPrimaryEntry
	for _, entry := range entries {
		if !familyPrimaryProtectedEntry(entry) {
			continue
		}
		protected = append(protected, entry)
	}
	sort.SliceStable(protected, func(i, j int) bool {
		left := protected[i]
		right := protected[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if left.item.OriginalRank != right.item.OriginalRank {
			return left.item.OriginalRank < right.item.OriginalRank
		}
		return left.item.Path < right.item.Path
	})
	if len(protected) > 6 {
		protected = protected[:6]
	}
	return protected
}

func familyPrimaryAddPairClosure(selected map[string]bool, entries map[string]*familyPrimaryEntry, byFamily map[string][]*familyPrimaryEntry, cap int, add func(*familyPrimaryEntry)) {
	selectedFamilies := map[string]map[string]bool{}
	for key := range selected {
		entry := entries[key]
		if entry == nil || (entry.class != "source" && entry.class != "test") {
			continue
		}
		classes := selectedFamilies[entry.family]
		if classes == nil {
			classes = map[string]bool{}
			selectedFamilies[entry.family] = classes
		}
		classes[entry.class] = true
	}
	for family, classes := range selectedFamilies {
		if classes["source"] && !classes["test"] {
			add(firstFamilyPrimaryClass(byFamily[family], "test"))
		}
		if classes["test"] && !classes["source"] {
			add(firstFamilyPrimaryClass(byFamily[family], "source"))
		}
		if len(selected) >= cap {
			return
		}
	}
}

func familyPrimaryProtectedEntries(entries map[string]*familyPrimaryEntry) []*familyPrimaryEntry {
	var protected []*familyPrimaryEntry
	for _, entry := range entries {
		if !familyPrimaryProtectedEntry(entry) {
			continue
		}
		protected = append(protected, entry)
	}
	sort.SliceStable(protected, func(i, j int) bool {
		left := protected[i]
		right := protected[j]
		leftScout := familyPrimaryScoutAnchorAdmission(left)
		rightScout := familyPrimaryScoutAnchorAdmission(right)
		if leftScout != rightScout {
			return leftScout
		}
		if left.item.OriginalRank != right.item.OriginalRank {
			return left.item.OriginalRank < right.item.OriginalRank
		}
		if left.score != right.score {
			return left.score > right.score
		}
		return left.item.Path < right.item.Path
	})
	if len(protected) > 4 {
		protected = protected[:4]
	}
	return protected
}

func familyPrimaryProtectedEntry(entry *familyPrimaryEntry) bool {
	if entry == nil || (entry.class != "source" && entry.class != "test") {
		return false
	}
	if familyPrimaryWeakPath(entry.item.Path) {
		return false
	}
	rank := entry.item.OriginalRank
	if rank > 0 && rank <= 2 {
		return true
	}
	if entry.score >= 10 {
		return true
	}
	reasons := strings.ToLower(strings.Join(entry.item.Reasons, "\n"))
	return strings.Contains(reasons, "source-family ranking") ||
		strings.Contains(reasons, "scout_anchor_admission") ||
		strings.Contains(reasons, "source_manifest_family_recovery") ||
		strings.Contains(reasons, "source_manifest_consumption_recovery") ||
		strings.Contains(reasons, "source_manifest_loss_safe_preserved") ||
		strings.Contains(reasons, "same_stem_source_recovery")
}

func familyPrimaryScoutAnchorAdmission(entry *familyPrimaryEntry) bool {
	if entry == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.Join(entry.item.Reasons, "\n")), "scout_anchor_admission")
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

func familyPrimaryQueryAnchorsV2(query string) ([]string, []string) {
	anchors, suppressed := familyPrimaryQueryAnchors(query)
	seen := map[string]bool{}
	for _, anchor := range anchors {
		seen[anchor] = true
	}
	add := func(anchor string) {
		anchor = familyPrimaryCompact(anchor)
		if len(anchor) < 3 || familyPrimaryPresentationStopTerms[anchor] || seen[anchor] {
			return
		}
		seen[anchor] = true
		anchors = append(anchors, anchor)
	}
	for _, anchor := range append([]string(nil), anchors...) {
		if strings.HasPrefix(anchor, "synchron") {
			add("sync")
		}
		switch {
		case strings.HasSuffix(anchor, "ies") && len(anchor) > 5:
			add(strings.TrimSuffix(anchor, "ies") + "y")
		case strings.HasSuffix(anchor, "ed") && len(anchor) > 5:
			add(strings.TrimSuffix(anchor, "d"))
			add(strings.TrimSuffix(anchor, "ed"))
		case strings.HasSuffix(anchor, "es") && len(anchor) > 5:
			add(strings.TrimSuffix(anchor, "s"))
			add(strings.TrimSuffix(anchor, "es"))
		case strings.HasSuffix(anchor, "s") && len(anchor) > 4:
			add(strings.TrimSuffix(anchor, "s"))
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

func familyPrimaryAnchorDocumentFrequency(pack RoleGroupedPack, anchors []string) map[string]int {
	df := map[string]int{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			for _, anchor := range anchors {
				if familyPrimaryAnchorStrength(item, anchor) > 0 {
					df[anchor]++
				}
			}
		}
	}
	return df
}

func familyPrimaryScoreV2(item PackItem, anchors []string, df map[string]int, total int) float64 {
	if total <= 0 {
		total = 1
	}
	var score float64
	for _, anchor := range anchors {
		strength := familyPrimaryAnchorStrength(item, anchor)
		if strength <= 0 {
			continue
		}
		score += strength * familyPrimaryAnchorRarityWeight(df[anchor], total)
	}
	rank := item.OriginalRank
	if rank <= 0 {
		rank = 99
	}
	rankBonus := 2.8 - float64(rank)*0.10
	if rankBonus > 0 {
		score += rankBonus
	}
	if familyPrimaryClass(item.Role, item) == "test" {
		score += 0.75
	}
	if strings.Contains(strings.ToLower(strings.Join(item.Reasons, "\n")), "source_manifest_loss_safe_preserved") {
		score += 0.75
	}
	return score
}

func familyPrimaryAnchorStrength(item PackItem, anchor string) float64 {
	anchor = familyPrimaryCompact(anchor)
	if anchor == "" {
		return 0
	}
	path := filepath.ToSlash(strings.ToLower(item.Path))
	title := strings.ToLower(item.Title)
	reasons := strings.ToLower(strings.Join(item.Reasons, "\n"))
	pathCompact := familyPrimaryCompact(path)
	titleCompact := familyPrimaryCompact(title)
	reasonCompact := familyPrimaryCompact(reasons)
	tokens := familyPrimaryPathTokenSet(path)
	stem := familyPrimaryStem(path)
	switch {
	case anchor == stem:
		return 8
	case tokens[anchor]:
		return 4.5
	case strings.Contains(pathCompact, anchor):
		return 3.5
	case strings.Contains(titleCompact, anchor):
		return 3
	case strings.Contains(reasonCompact, anchor):
		return 1.25
	default:
		return 0
	}
}

func familyPrimaryAnchorRarityWeight(df, total int) float64 {
	if df <= 0 || total <= 0 {
		return 1
	}
	ratio := float64(df) / float64(total)
	switch {
	case df <= 1:
		return 2.4
	case df <= 2:
		return 2.0
	case df <= 4:
		return 1.6
	case ratio <= 0.35:
		return 1.25
	case ratio >= 0.70:
		return 0.45
	default:
		return 0.8
	}
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

func familyPrimaryWeakPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(strings.SplitN(path, "#", 2)[0]))
	if path == "" {
		return false
	}
	return strings.Contains(path, "/docs_src/") ||
		strings.HasPrefix(path, "docs_src/") ||
		strings.Contains(path, "/tutorial") ||
		strings.Contains(path, "/examples/") ||
		strings.HasPrefix(path, "examples/") ||
		strings.Contains(path, "/fixtures/") ||
		strings.Contains(path, "/testdata/") ||
		strings.Contains(path, "/vendor/") ||
		strings.Contains(path, "/node_modules/") ||
		strings.Contains(path, "/generated/") ||
		strings.Contains(path, "/dist/")
}

func familyPrimaryMetadata(pack RoleGroupedPack, anchors []string, primaryCount, relatedCount int, cap int, protectExact bool) map[string]string {
	metadata := map[string]string{}
	for k, v := range pack.Metadata {
		metadata[k] = v
	}
	metadata["family_primary"] = "true"
	metadata["family_primary_cap"] = strconv.Itoa(cap)
	metadata["family_primary_count"] = strconv.Itoa(primaryCount)
	metadata["family_related_count"] = strconv.Itoa(relatedCount)
	if protectExact {
		metadata["family_primary_exact_protection"] = "true"
	}
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
