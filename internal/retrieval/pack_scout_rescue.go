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
	packScoutSourceRescueCountKey = "pack_scout_source_rescue_count"
	packScoutSourceRescueCap      = 6
)

// ApplyScoutSourceTestRescueForQuery promotes a small number of related source
// rows when already-primary tests and the query point at the same source stem.
// It never admits new candidates and only moves related rows to primary.
func ApplyScoutSourceTestRescueForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	queryRoots := scoutRootSet(scoutQueryTokens(query))
	if len(queryRoots) == 0 {
		return pack
	}
	primaryCount := scoutPrimaryCount(pack)
	if primaryCount >= packScoutSourceRescueCap {
		return pack
	}
	testRoots := scoutPrimaryTestRoots(pack)
	if len(testRoots) == 0 {
		return pack
	}

	type promotion struct {
		groupIdx int
		itemIdx  int
		score    int
		reasons  []string
	}
	var promotions []promotion
	for groupIdx, group := range pack.Groups {
		for itemIdx, item := range group.Items {
			if !PackItemIsRelated(item) || familyPrimaryClass(group.Role, item) != "source" || familyPrimaryWeakPath(item.Path) {
				continue
			}
			score, reasons := scoutSourceRescueScore(item, queryRoots, testRoots)
			if score >= 5 {
				promotions = append(promotions, promotion{groupIdx: groupIdx, itemIdx: itemIdx, score: score, reasons: reasons})
			}
		}
	}
	if len(promotions) == 0 {
		return pack
	}
	sort.SliceStable(promotions, func(i, j int) bool {
		if promotions[i].score != promotions[j].score {
			return promotions[i].score > promotions[j].score
		}
		left := pack.Groups[promotions[i].groupIdx].Items[promotions[i].itemIdx]
		right := pack.Groups[promotions[j].groupIdx].Items[promotions[j].itemIdx]
		if left.OriginalRank != right.OriginalRank {
			return left.OriginalRank < right.OriginalRank
		}
		return left.Path < right.Path
	})

	promoted := 0
	for _, promotion := range promotions {
		if primaryCount >= packScoutSourceRescueCap {
			break
		}
		item := &pack.Groups[promotion.groupIdx].Items[promotion.itemIdx]
		if !PackItemIsRelated(*item) {
			continue
		}
		item.PackTier = PackTierPrimary
		item.Reasons = appendUniqueString(item.Reasons, "scout source rescue: "+strings.Join(promotion.reasons, "; "))
		promoted++
		primaryCount++
	}
	if promoted == 0 {
		return pack
	}
	for groupIdx := range pack.Groups {
		sort.SliceStable(pack.Groups[groupIdx].Items, func(i, j int) bool {
			left := pack.Groups[groupIdx].Items[i]
			right := pack.Groups[groupIdx].Items[j]
			leftRelated := PackItemIsRelated(left)
			rightRelated := PackItemIsRelated(right)
			if leftRelated != rightRelated {
				return !leftRelated
			}
			if left.OriginalRank != right.OriginalRank {
				return left.OriginalRank < right.OriginalRank
			}
			return left.Path < right.Path
		})
	}
	if pack.Metadata == nil {
		pack.Metadata = map[string]string{}
	}
	pack.Metadata[packScoutSourceRescueCountKey] = strconv.Itoa(promoted)
	pack.Notes = appendUniqueString(pack.Notes, fmt.Sprintf("Scout source/test rescue promoted %d related source row(s).", promoted))
	return pack
}

func scoutPrimaryCount(pack RoleGroupedPack) int {
	count := 0
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if !PackItemIsRelated(item) {
				count++
			}
		}
	}
	return count
}

func scoutPrimaryTestRoots(pack RoleGroupedPack) map[string]bool {
	roots := map[string]bool{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if PackItemIsRelated(item) || familyPrimaryClass(group.Role, item) != "test" {
				continue
			}
			for _, token := range scoutBasenameTokens(item.Path) {
				for root := range scoutTokenRoots(token) {
					roots[root] = true
				}
			}
		}
	}
	return roots
}

func scoutSourceRescueScore(item PackItem, queryRoots, testRoots map[string]bool) (int, []string) {
	score := 0
	var queryMatches []string
	var testMatches []string
	pathRoots := scoutRootSet(append(scoutPathTokens(item.Path), scoutBasenameTokens(item.Path)...))
	for root := range pathRoots {
		if queryRoots[root] && !scoutGenericRoot(root) {
			queryMatches = append(queryMatches, root)
		}
		if testRoots[root] && !scoutGenericRoot(root) {
			testMatches = append(testMatches, root)
		}
	}
	sort.Strings(queryMatches)
	sort.Strings(testMatches)
	if len(queryMatches) > 0 {
		score += minInt(2, len(queryMatches)) * 2
	}
	if len(testMatches) > 0 {
		score += 3
	}
	var reasons []string
	if len(queryMatches) > 0 {
		reasons = append(reasons, "query roots "+strings.Join(firstScoutStrings(queryMatches, 4), ","))
	}
	if len(testMatches) > 0 {
		reasons = append(reasons, "primary test roots "+strings.Join(firstScoutStrings(testMatches, 4), ","))
	}
	return score, reasons
}

func scoutQueryTokens(query string) []string {
	var out []string
	for _, token := range meaningfulTerms(strings.ToLower(query)) {
		if token != "" && !familyPrimaryPresentationStopTerms[token] {
			out = append(out, token)
		}
	}
	for _, raw := range tokenizePreservingIdentifiers(query) {
		for _, part := range splitIdentifierParts(raw) {
			part = strings.ToLower(part)
			if part != "" && !familyPrimaryPresentationStopTerms[part] {
				out = append(out, part)
			}
		}
	}
	return out
}

func scoutPathTokens(path string) []string {
	path = strings.ToLower(filepath.ToSlash(strings.SplitN(path, "#", 2)[0]))
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	var out []string
	for _, part := range parts {
		for _, token := range splitIdentifierParts(part) {
			token = strings.ToLower(strings.TrimSpace(token))
			if len(token) >= 3 && !familyPrimaryPresentationStopTerms[token] {
				out = append(out, token)
			}
		}
	}
	return out
}

func scoutBasenameTokens(path string) []string {
	path = filepath.ToSlash(strings.SplitN(path, "#", 2)[0])
	base := strings.ToLower(filepath.Base(path))
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	for _, suffix := range []string{".test", ".spec", "_test", "-test"} {
		base = strings.TrimSuffix(base, suffix)
	}
	base = strings.TrimPrefix(base, "test_")
	return scoutPathTokens(base)
}

func scoutRootSet(tokens []string) map[string]bool {
	roots := map[string]bool{}
	for _, token := range tokens {
		for root := range scoutTokenRoots(token) {
			roots[root] = true
		}
	}
	return roots
}

func scoutTokenRoots(token string) map[string]bool {
	token = strings.ToLower(strings.TrimSpace(token))
	out := map[string]bool{}
	if len(token) < 3 {
		return out
	}
	add := func(value string) {
		if len(value) >= 3 {
			out[value] = true
		}
	}
	add(token)
	for _, suffix := range []string{"ation", "ator", "ing", "ed", "er", "or", "s"} {
		if len(token) > len(suffix)+3 && strings.HasSuffix(token, suffix) {
			add(strings.TrimSuffix(token, suffix))
		}
	}
	if len(token) > 7 {
		add(token[:6])
	}
	if len(token) >= 6 {
		add(token[:5])
	}
	return out
}

func scoutGenericRoot(root string) bool {
	switch strings.ToLower(strings.TrimSpace(root)) {
	case "", "api", "app", "apps", "lib", "node", "pack", "pkg", "src", "test", "tests":
		return true
	default:
		return false
	}
}

func firstScoutStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}
