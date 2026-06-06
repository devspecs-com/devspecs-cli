package retrieval

import (
	"fmt"
	"sort"
	"strings"
)

const (
	packScoutAnchorAdmissionCap = 6
)

type scoutAnchorAdmission struct {
	candidate Candidate
	score     float64
	terms     []string
	class     string
	pathKey   string
}

// AddScoutAnchorAdmissionCandidates appends a few indexed source/test
// candidates when the selected set missed a clustered dominant query anchor. This is
// beta/scout-only plumbing: it does not scan the filesystem and does not affect
// default retrieval.
func AddScoutAnchorAdmissionCandidates(selected, universe []Candidate, query string) []Candidate {
	if len(selected) == 0 || len(universe) == 0 {
		return selected
	}
	mode := AnchorFirstModeCodeTaskFamilyV2
	profile := BuildAnchorProfile(query)
	if !profile.HasSpecific {
		return selected
	}
	vocab := BuildRepoVocabulary(universe)
	classes := codeTaskFamilyV2AnchorClasses(profile, vocab, scoredCandidatesFromCandidates(universe), mode)
	if !codeTaskFamilyV2HasDominantAnchor(classes) {
		return selected
	}
	missingDominant := scoutMissingDominantAnchorTerms(selected, classes, mode)
	if len(missingDominant) == 0 {
		return selected
	}
	admissibleDominant := scoutAdmissibleDominantAnchorTerms(universe, missingDominant, query, mode)
	if len(admissibleDominant) == 0 {
		return selected
	}

	seen := map[string]bool{}
	for _, candidate := range selected {
		seen[candidateIdentity(candidate)] = true
		if candidate.Path != "" {
			seen[normalizeCodeTaskFamilyPath(candidate.Path)] = true
		}
	}

	var admissions []scoutAnchorAdmission
	queryLower := strings.ToLower(query)
	testIntent := hasTestBehaviorIntent(queryLower)
	for _, candidate := range universe {
		pathKey := normalizeCodeTaskFamilyPath(candidate.Path)
		if seen[candidateIdentity(candidate)] || seen[pathKey] {
			continue
		}
		class := "source"
		if isTestCaseCandidate(candidate) {
			class = "test"
		} else if !IsSourceContextCandidate(candidate) {
			continue
		}
		if codeTaskFamilyV2LowPrioritySupportPath(candidate.Path) && !codeTaskQueryRequestsDocsExample(queryLower) {
			continue
		}
		match := codeTaskFamilyV2CandidateAnchorMatch(candidate, classes, mode)
		if !match.dominantSpecific {
			continue
		}
		terms := scoutAdmissionFieldTerms(candidate, match.matchedDominant, admissibleDominant, mode)
		if len(terms) == 0 {
			continue
		}
		result := scoreAnchorFirstCandidate(candidate, profile, vocab, mode)
		if result.score < 7.5 {
			continue
		}
		score := result.score + float64(codeTaskFamilyV2TestAffinityScore(candidate))*0.35
		candidate = withAnchorFirstMetadata(candidate, clampFloat(result.score, 0, 24), result, mode)
		candidate = withScoutAnchorAdmissionMetadata(candidate, terms, score)
		admissions = append(admissions, scoutAnchorAdmission{candidate: candidate, score: score, terms: terms, class: class, pathKey: pathKey})
	}
	if len(admissions) == 0 {
		return selected
	}
	sort.SliceStable(admissions, func(i, j int) bool {
		if !testIntent && admissions[i].class != admissions[j].class {
			return admissions[i].class == "source"
		}
		if admissions[i].score != admissions[j].score {
			return admissions[i].score > admissions[j].score
		}
		leftTerms := strings.Join(admissions[i].terms, ",")
		rightTerms := strings.Join(admissions[j].terms, ",")
		if leftTerms != rightTerms {
			return leftTerms < rightTerms
		}
		return admissions[i].candidate.Path < admissions[j].candidate.Path
	})
	admissions = dedupeScoutAnchorAdmissions(admissions)
	if len(admissions) > packScoutAnchorAdmissionCap {
		admissions = admissions[:packScoutAnchorAdmissionCap]
	}

	out := append([]Candidate(nil), selected...)
	for _, admission := range admissions {
		out = append(out, admission.candidate)
	}
	return out
}

func dedupeScoutAnchorAdmissions(admissions []scoutAnchorAdmission) []scoutAnchorAdmission {
	out := make([]scoutAnchorAdmission, 0, len(admissions))
	seen := map[string]bool{}
	for _, admission := range admissions {
		key := admission.pathKey
		if key == "" {
			key = candidateIdentity(admission.candidate)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, admission)
	}
	return out
}

func scoutMissingDominantAnchorTerms(selected []Candidate, classes []codeTaskFamilyV2AnchorClass, mode string) map[string]bool {
	missing := map[string]bool{}
	for _, class := range classes {
		if class.class == "dominant" {
			missing[class.term] = true
		}
	}
	for _, candidate := range selected {
		match := codeTaskFamilyV2CandidateAnchorMatch(candidate, classes, mode)
		if !match.dominantSpecific {
			continue
		}
		for _, term := range match.matchedDominant {
			delete(missing, term)
		}
	}
	return missing
}

func scoutAdmissionTerms(terms []string, allowed map[string]bool) []string {
	var out []string
	for _, term := range terms {
		if allowed[term] {
			out = append(out, term)
		}
	}
	return uniqueStrings(out)
}

func scoutAdmissionFieldTerms(candidate Candidate, terms []string, allowed map[string]bool, mode string) []string {
	fields := metadataAnchorFieldTerms(candidate, false)
	var out []string
	for _, term := range scoutAdmissionTerms(terms, allowed) {
		if anchorFieldTermCount(term, fields.path, mode) > 0 ||
			anchorFieldTermCount(term, fields.title, mode) > 0 ||
			anchorFieldTermCount(term, fields.testName, mode) > 0 {
			out = append(out, term)
		}
	}
	return uniqueStrings(out)
}

func scoutAdmissibleDominantAnchorTerms(universe []Candidate, missing map[string]bool, query string, mode string) map[string]bool {
	queryLower := strings.ToLower(query)
	counts := map[string]int{}
	for _, candidate := range universe {
		if !IsSourceContextCandidate(candidate) || isTestCaseCandidate(candidate) {
			continue
		}
		if codeTaskFamilyV2LowPrioritySupportPath(candidate.Path) && !codeTaskQueryRequestsDocsExample(queryLower) {
			continue
		}
		fields := metadataAnchorFieldTerms(candidate, false)
		for term := range missing {
			if anchorFieldTermCount(term, fields.path, mode) > 0 ||
				anchorFieldTermCount(term, fields.title, mode) > 0 {
				counts[term]++
			}
		}
	}
	out := map[string]bool{}
	for term, count := range counts {
		if count >= 3 {
			out[term] = true
		}
	}
	return out
}

func withScoutAnchorAdmissionMetadata(candidate Candidate, terms []string, score float64) Candidate {
	if candidate.Metadata == nil {
		candidate.Metadata = map[string]string{}
	} else {
		candidate.Metadata = copyMetadata(candidate.Metadata)
	}
	terms = uniqueStrings(terms)
	candidate.Metadata["retrieval_expansion_reason"] = "scout_anchor_admission"
	candidate.Metadata["pack_tier"] = PackTierPrimary
	candidate.Metadata["pack_tier_reason"] = "dominant query anchor admission"
	candidate.Metadata["scout_anchor_terms"] = strings.Join(terms, ", ")
	candidate.Metadata["scout_anchor_admission_score"] = fmt.Sprintf("%.3f", score)
	return candidate
}
