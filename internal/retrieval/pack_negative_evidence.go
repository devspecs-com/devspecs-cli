package retrieval

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const packNegativeEvidenceCountKey = "negative_evidence_count"

// ApplyDemotionOnlyNegativeEvidence moves likely low-signal rows out of the
// working set without admitting new candidates. It is intended for explicit
// scout/beta pack modes, not default pack behavior.
func ApplyDemotionOnlyNegativeEvidence(pack RoleGroupedPack, query string) RoleGroupedPack {
	queryLower := strings.ToLower(query)
	var demoted []PackItem
	var groups []PackGroup
	for _, group := range pack.Groups {
		kept := group.Items[:0]
		for _, item := range group.Items {
			reason := demotionOnlyNegativeEvidenceReason(item, queryLower)
			if reason == "" {
				kept = append(kept, item)
				continue
			}
			item.Role = PackRoleExcludedNoise
			item.RoleReason = reason
			item.PackTier = ""
			item.Reasons = appendUniqueString(item.Reasons, "negative evidence: "+reason)
			demoted = append(demoted, item)
		}
		group.Items = kept
		if len(group.Items) > 0 {
			group.OverflowCount = maxInt(0, len(group.Items)-group.Budget)
			groups = append(groups, group)
		}
	}
	if len(demoted) == 0 {
		return pack
	}

	pack.Groups = groups
	pack.ExcludedNoise = append(pack.ExcludedNoise, demoted...)
	if budget := packRoleBudgets[PackRoleExcludedNoise]; budget > 0 && len(pack.ExcludedNoise) > budget {
		pack.ExcludedNoise = pack.ExcludedNoise[:budget]
	}
	pack.Counts = recomputePackCounts(pack.Groups, pack.ExcludedNoise)
	pack.Summary = BuildPackSummary(pack.Groups, pack.ExcludedNoise)
	pack.Notes = appendUniqueString(pack.Notes, fmt.Sprintf("Demotion-only negative evidence moved %d low-signal row(s) out of the working set.", len(demoted)))
	if pack.Metadata == nil {
		pack.Metadata = map[string]string{}
	}
	pack.Metadata[packNegativeEvidenceCountKey] = strconv.Itoa(len(demoted))
	return pack
}

func demotionOnlyNegativeEvidenceReason(item PackItem, queryLower string) string {
	category := lowSignalPathFamily(item.Path)
	if category == "" || queryRequestsLowSignalFamily(queryLower, category) {
		return ""
	}
	switch category {
	case "playground":
		return "playground path; query does not ask for playground context"
	case "fixture":
		return "fixture or testdata path; query does not ask for fixture data"
	case "example":
		return "example path; query does not ask for examples"
	case "docs":
		if item.Role == PackRoleBackgroundDecisions || item.Role == PackRoleOpenWork || item.Role == PackRoleSupportingContext {
			return ""
		}
		return "docs path; query does not ask for documentation context"
	case "changelog":
		return "changelog or changeset path; query does not ask for release/change notes"
	default:
		return ""
	}
}

func lowSignalPathFamily(path string) string {
	path = strings.ToLower(filepath.ToSlash(strings.SplitN(strings.TrimSpace(path), "#", 2)[0]))
	if path == "" {
		return ""
	}
	if hasPathSegment(path, "playground") {
		return "playground"
	}
	if hasPathSegment(path, "fixture") || hasPathSegment(path, "fixtures") || hasPathSegment(path, "testdata") {
		return "fixture"
	}
	if hasPathSegment(path, "example") || hasPathSegment(path, "examples") {
		return "example"
	}
	if hasPathSegment(path, "docs") || hasPathSegment(path, "docs_src") {
		return "docs"
	}
	base := filepath.Base(path)
	if strings.Contains(base, "changelog") || strings.Contains(path, "/.changeset/") || strings.HasPrefix(path, ".changeset/") {
		return "changelog"
	}
	return ""
}

func queryRequestsLowSignalFamily(queryLower, category string) bool {
	switch category {
	case "playground":
		return hasQueryWord(queryLower, "playground")
	case "fixture":
		return queryRequestsFixtureSample(queryLower)
	case "example":
		return queryRequestsFixtureSample(queryLower)
	case "docs":
		return queryRequestsDocumentation(queryLower)
	case "changelog":
		return containsAny(queryLower, "changelog", "change log", "changeset", "change set", "release note", "release notes")
	default:
		return false
	}
}

func queryRequestsDocumentation(queryLower string) bool {
	return containsAny(queryLower, "docs", "documentation", "guide", "guides", "readme", "adr", "design", "proposal", "rfc")
}

func recomputePackCounts(groups []PackGroup, excluded []PackItem) map[string]int {
	counts := map[string]int{}
	for _, group := range groups {
		if len(group.Items) > 0 {
			counts[group.Role] += len(group.Items)
		}
	}
	if len(excluded) > 0 {
		counts[PackRoleExcludedNoise] = len(excluded)
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}
