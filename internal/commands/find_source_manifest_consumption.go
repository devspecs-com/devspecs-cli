package commands

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const findSourceManifestConsumptionMaxVisible = 8

func applyFindSourceManifestConsumptionScout(query string, matches, all []retrieval.Candidate) []retrieval.Candidate {
	if len(matches) == 0 || len(all) == 0 {
		return matches
	}
	terms := findSourceManifestConsumptionTerms(query)
	if len(terms) == 0 {
		return matches
	}
	out := append([]retrieval.Candidate(nil), matches...)
	seen := map[string]bool{}
	for _, c := range out {
		seen[findSourceManifestConsumptionPath(c)] = true
	}

	replacements := findSourceManifestReplacementPool(all, seen, terms)
	replacementIndex := 0
	for i, c := range out {
		if replacementIndex >= len(replacements) {
			break
		}
		if !findSourceManifestWeakSourceNoise(c) {
			continue
		}
		if replacements[replacementIndex].score < 10 {
			break
		}
		replacement := annotateFindSourceManifestConsumptionCandidate(replacements[replacementIndex].candidate, "source_manifest_consumption_replacement", retrieval.PackTierPrimary)
		out[i] = replacement
		seen[findSourceManifestConsumptionPath(replacement)] = true
		replacementIndex++
	}

	testReservations := findSourceManifestTestReservationPool(all, seen, terms)
	reserved := 0
	for _, ranked := range testReservations {
		if ranked.score < 10 || reserved >= 2 {
			break
		}
		testCandidate := annotateFindSourceManifestConsumptionCandidate(ranked.candidate, "source_manifest_test_reservation", retrieval.PackTierRelated)
		replaced := false
		if len(out) >= findSourceManifestConsumptionMaxVisible {
			for i := len(out) - 1; i >= 0; i-- {
				if findSourceManifestWeakSourceNoise(out[i]) {
					out[i] = testCandidate
					replaced = true
					break
				}
			}
		}
		if !replaced && len(out) < findSourceManifestConsumptionMaxVisible {
			out = append(out, testCandidate)
		} else if !replaced {
			continue
		}
		seen[findSourceManifestConsumptionPath(testCandidate)] = true
		reserved++
	}
	return out
}

type findSourceManifestRankedCandidate struct {
	candidate retrieval.Candidate
	score     int
}

func findSourceManifestReplacementPool(all []retrieval.Candidate, seen map[string]bool, terms []string) []findSourceManifestRankedCandidate {
	var ranked []findSourceManifestRankedCandidate
	for _, c := range all {
		if !findSourceManifestCandidate(c) || findSourceManifestTestCandidate(c) {
			continue
		}
		if seen[findSourceManifestConsumptionPath(c)] || findSourceManifestWeakSourceNoise(c) {
			continue
		}
		score := findSourceManifestConsumptionScore(c, terms)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, findSourceManifestRankedCandidate{candidate: c, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].candidate.Path < ranked[j].candidate.Path
		}
		return ranked[i].score > ranked[j].score
	})
	return ranked
}

func findSourceManifestTestReservationPool(all []retrieval.Candidate, seen map[string]bool, terms []string) []findSourceManifestRankedCandidate {
	var ranked []findSourceManifestRankedCandidate
	for _, c := range all {
		if !findSourceManifestCandidate(c) || !findSourceManifestTestCandidate(c) {
			continue
		}
		if seen[findSourceManifestConsumptionPath(c)] || findSourceManifestWeakSourceNoise(c) {
			continue
		}
		score := findSourceManifestConsumptionScore(c, terms)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, findSourceManifestRankedCandidate{candidate: c, score: score + 4})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].candidate.Path < ranked[j].candidate.Path
		}
		return ranked[i].score > ranked[j].score
	})
	return ranked
}

func findSourceManifestConsumptionScore(c retrieval.Candidate, terms []string) int {
	path := strings.ToLower(filepath.ToSlash(c.Path + "\n" + c.Source + "\n" + c.Title))
	body := strings.ToLower(c.Body)
	if len(body) > 12000 {
		body = body[:12000]
	}
	if c.Metadata != nil {
		body += "\n" + strings.ToLower(c.Metadata["source_symbols"]) + "\n" + strings.ToLower(c.Metadata["test_name"])
	}
	score := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(path, term) {
			score += 5 + findSourceManifestTermSpecificity(term)
		}
		if strings.Contains(body, term) {
			score += 2 + findSourceManifestTermSpecificity(term)/2
		}
		compact := strings.ReplaceAll(strings.ReplaceAll(term, "_", ""), "-", "")
		if compact != term && compact != "" && strings.Contains(body, compact) {
			score += 3 + findSourceManifestTermSpecificity(term)
		}
	}
	pathOnly := strings.ToLower(filepath.ToSlash(c.Path))
	if c.Metadata != nil {
		switch c.Metadata["source_root_kind"] {
		case "module_root", "common_root", "implementation_root":
			score += 3
		}
		switch c.Metadata["source_role"] {
		case "implementation", "source":
			score += 2
		case "test":
			score += 2
		}
	}
	if strings.Contains(pathOnly, "/docs_src/") || strings.HasPrefix(pathOnly, "docs_src/") || strings.Contains(pathOnly, "/tutorial") {
		score -= 8
	}
	if strings.Contains(pathOnly, "/examples/") || strings.HasPrefix(pathOnly, "examples/") {
		score -= 5
	}
	return score
}

func findSourceManifestTermSpecificity(term string) int {
	score := 0
	if len(term) >= 8 {
		score += 2
	}
	if strings.ContainsAny(term, "_.-0123456789") {
		score += 5
	}
	switch term {
	case "docs", "custom", "default", "client", "headers", "source", "files", "using", "users":
		score -= 4
	case "make", "serve", "allow", "load", "need", "when", "from":
		score -= 3
	}
	return score
}

func findSourceManifestCandidate(c retrieval.Candidate) bool {
	return c.Metadata != nil && c.Metadata["retrieval_candidate"] == "source_manifest"
}

func findSourceManifestTestCandidate(c retrieval.Candidate) bool {
	if strings.EqualFold(c.Subtype, "test_case") {
		return true
	}
	path := strings.ToLower(filepath.ToSlash(c.Path))
	return strings.Contains(path, "/tests/") || strings.HasPrefix(path, "tests/")
}

func findSourceManifestWeakSourceNoise(c retrieval.Candidate) bool {
	path := strings.ToLower(filepath.ToSlash(c.Path))
	if findSourceManifestTestCandidate(c) {
		return false
	}
	return strings.Contains(path, "/docs_src/") ||
		strings.HasPrefix(path, "docs_src/") ||
		strings.Contains(path, "/docs/en/docs/js/") ||
		strings.Contains(path, "/tutorial") ||
		strings.Contains(path, "/examples/") ||
		strings.HasPrefix(path, "examples/")
}

func annotateFindSourceManifestConsumptionCandidate(c retrieval.Candidate, reason, tier string) retrieval.Candidate {
	metadata := map[string]string{}
	for key, value := range c.Metadata {
		metadata[key] = value
	}
	metadata["retrieval_expansion_reason"] = reason
	metadata["source_manifest_consumption"] = "true"
	if tier != "" {
		metadata["pack_tier"] = tier
	}
	switch reason {
	case "source_manifest_test_reservation":
		metadata["pack_tier_reason"] = "reserved manifest test with direct query evidence"
	case "source_manifest_consumption_replacement":
		metadata["pack_tier_reason"] = "manifest implementation candidate replaced weaker docs/tutorial source"
	}
	c.Metadata = metadata
	return c
}

func findSourceManifestConsumptionPath(c retrieval.Candidate) string {
	path := filepath.ToSlash(c.Path)
	if path == "" {
		path = filepath.ToSlash(c.Source)
	}
	return strings.ToLower(path)
}

func findSourceManifestConsumptionTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		term = strings.ToLower(strings.Trim(term, "_-."))
		if len(term) < 3 || seen[term] || findSourceManifestConsumptionStopWord(term) {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		raw := b.String()
		add(raw)
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '_' || r == '-' || r == '.' }) {
			add(part)
		}
		b.Reset()
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return terms
}

func findSourceManifestConsumptionStopWord(term string) bool {
	switch term {
	case "the", "and", "for", "with", "that", "this", "from", "into", "what", "where", "when", "which", "but", "let":
		return true
	default:
		return false
	}
}
