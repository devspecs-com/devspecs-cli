package retrieval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	PackScoutUncertaintyKey               = "pack_scout_uncertainty"
	PackScoutUncertaintyReasonsKey        = "pack_scout_uncertainty_reasons"
	PackScoutUncertaintyMissingAnchorsKey = "pack_scout_uncertainty_missing_anchors"
)

// ApplyScoutUncertaintyForQuery adds beta/scout-only caution receipts when the
// visible first working set looks exploratory rather than source/test complete.
func ApplyScoutUncertaintyForQuery(pack RoleGroupedPack, query string) RoleGroupedPack {
	sourcePrimary, testPrimary := scoutPrimarySourceTestCounts(pack)
	var reasons []string
	switch {
	case sourcePrimary == 0:
		reasons = append(reasons, "no primary implementation surface is visible")
	case sourcePrimary <= 1 && testPrimary >= 3:
		reasons = append(reasons, fmt.Sprintf("implementation surface is thin relative to tests (%d source, %d tests)", sourcePrimary, testPrimary))
	case sourcePrimary >= 4 && testPrimary == 0:
		reasons = append(reasons, fmt.Sprintf("behavior-test surface is thin relative to source (%d source, %d tests)", sourcePrimary, testPrimary))
	case sourcePrimary >= 3 && testPrimary == 1:
		if gaps := scoutSingleTestSourceFamilyGaps(pack, query); len(gaps) > 0 {
			reasons = append(reasons, "visible behavior test is not backed by a primary source family: "+strings.Join(firstScoutStrings(gaps, 4), ", "))
		}
	case testPrimary == 0 && sourcePrimary > 0:
		reasons = append(reasons, "no primary behavior tests are visible")
	}

	missing := scoutMissingPrimaryAnchorRoots(pack, query)
	if len(reasons) > 0 && len(missing) > 0 {
		reasons = append(reasons, "primary rows do not show query anchor(s): "+strings.Join(firstScoutStrings(missing, 4), ", "))
	}
	if len(reasons) == 0 {
		return pack
	}
	if pack.Metadata == nil {
		pack.Metadata = map[string]string{}
	}
	pack.Metadata[PackScoutUncertaintyKey] = "true"
	pack.Metadata[PackScoutUncertaintyReasonsKey] = strings.Join(reasons, "\n")
	if len(missing) > 0 {
		pack.Metadata[PackScoutUncertaintyMissingAnchorsKey] = strings.Join(missing, ",")
	}
	pack.Metadata["pack_scout_primary_source_count"] = strconv.Itoa(sourcePrimary)
	pack.Metadata["pack_scout_primary_test_count"] = strconv.Itoa(testPrimary)
	return pack
}

func scoutPrimarySourceTestCounts(pack RoleGroupedPack) (int, int) {
	sourcePrimary := 0
	testPrimary := 0
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if PackItemIsRelated(item) {
				continue
			}
			switch familyPrimaryClass(group.Role, item) {
			case "source":
				sourcePrimary++
			case "test":
				testPrimary++
			}
		}
	}
	return sourcePrimary, testPrimary
}

func scoutSingleTestSourceFamilyGaps(pack RoleGroupedPack, query string) []string {
	queryRoots := scoutRootSet(scoutQueryTokens(query))
	if len(queryRoots) == 0 {
		return nil
	}
	sourceRoots := map[string]bool{}
	testRoots := map[string]bool{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if PackItemIsRelated(item) {
				continue
			}
			roots := scoutPrimaryItemRoots(item)
			switch familyPrimaryClass(group.Role, item) {
			case "source":
				for root := range roots {
					sourceRoots[root] = true
				}
			case "test":
				for root := range roots {
					testRoots[root] = true
				}
			}
		}
	}

	var gaps []string
	for root := range testRoots {
		if queryRoots[root] && !sourceRoots[root] && !scoutGenericRoot(root) {
			gaps = append(gaps, root)
		}
	}
	sort.Strings(gaps)
	return gaps
}

func scoutPrimaryItemRoots(item PackItem) map[string]bool {
	tokens := append(scoutPathTokens(item.Path), scoutBasenameTokens(item.Path)...)
	tokens = append(tokens, scoutPathTokens(item.Title)...)
	return scoutRootSet(tokens)
}

func scoutMissingPrimaryAnchorRoots(pack RoleGroupedPack, query string) []string {
	covered := map[string]bool{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if PackItemIsRelated(item) {
				continue
			}
			tokens := append(scoutPathTokens(item.Path), scoutBasenameTokens(item.Path)...)
			tokens = append(tokens, scoutPathTokens(item.Title)...)
			for root := range scoutRootSet(tokens) {
				covered[root] = true
			}
		}
	}

	var missing []string
	seen := map[string]bool{}
	for _, token := range scoutQueryTokens(query) {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" || seen[token] || scoutGenericRoot(token) || familyPrimaryPresentationStopTerms[token] {
			continue
		}
		seen[token] = true
		roots := scoutTokenRoots(token)
		if len(roots) == 0 {
			continue
		}
		hasCover := false
		for root := range roots {
			if covered[root] {
				hasCover = true
				break
			}
		}
		if !hasCover {
			missing = append(missing, token)
		}
	}
	return missing
}
