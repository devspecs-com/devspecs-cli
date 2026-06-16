package retrieval

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const exactAnchorBackfillLimit = 4

type exactQueryAnchorProfile struct {
	Specific []string
	Generic  []string
}

type exactAnchorScore struct {
	score          float64
	matches        []string
	fields         []string
	genericMatches []string
}

type exactIntentIDProfile struct {
	IDs []exactIntentID
}

type exactIntentID struct {
	Display string
	Norm    string
	Compact string
	Parts   []string
}

type exactIntentIDMatch struct {
	PathTitle bool
	Body      bool
	PartsOnly bool
	Matched   []string
}

var exactAnchorGenericTerms = map[string]bool{
	"call": true, "case": true, "cases": true, "checker": true, "cookie": true, "cookies": true,
	"engine": true, "file": true, "files": true,
	"format": true, "formatting": true, "header": true, "headers": true,
	"hierarchy": true,
	"go":        true, "js": true, "md": true, "py": true, "rs": true, "ts": true,
	"helper": true, "helpers": true, "implementation": true, "parser": true,
	"parsers": true, "regex": true, "regular": true, "release": true,
	"source": true, "support": true, "test": true, "tests": true,
	"type": true,
}

func applyExactAnchorDiscipline(candidates []scoredCandidate, universe []Candidate, query string, queryLower string, terms map[string]float64, limit int) []scoredCandidate {
	profile := buildExactQueryAnchorProfile(query)
	if len(profile.Specific) == 0 || len(candidates) == 0 {
		return candidates
	}
	seen := map[string]bool{}
	changed := false
	for i := range candidates {
		seen[candidateIdentity(candidates[i].candidate)] = true
		score := scoreExactAnchorCandidate(candidates[i].candidate, profile)
		if score.score > 0 {
			candidates[i].score += score.score
			candidates[i].candidate = withExactAnchorMetadata(candidates[i].candidate, score)
			changed = true
			continue
		}
		if len(score.genericMatches) > 0 && candidateCanBeGenericDilution(candidates[i], queryLower) {
			candidates[i].score -= 36.0
			changed = true
		}
	}

	backfills := exactAnchorBackfillCandidates(universe, seen, profile, queryLower, terms)
	if len(backfills) > 0 {
		candidates = append(candidates, backfills...)
		changed = true
	}
	intentChanged := false
	candidates, intentChanged = applyExactIntentIDDiscipline(candidates, universe, query)
	if intentChanged {
		changed = true
	}
	if changed {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].score == candidates[j].score {
				return candidates[i].candidate.Path < candidates[j].candidate.Path
			}
			return candidates[i].score > candidates[j].score
		})
	}
	return candidates
}

func applyExactIntentIDDiscipline(candidates []scoredCandidate, universe []Candidate, query string) ([]scoredCandidate, bool) {
	profile := buildExactIntentIDProfile(query)
	if len(profile.IDs) == 0 || len(candidates) == 0 || !exactIntentIDPresent(universe, profile) {
		return candidates, false
	}
	changed := false
	for i := range candidates {
		match := exactIntentIDCandidateMatch(candidates[i].candidate, profile)
		delta := 0.0
		tier := ""
		tierReason := ""
		var reasons []string
		switch {
		case match.PathTitle:
			delta = 18.0
			tier = PackTierPrimary
			tierReason = "direct exact plan/track ID match"
			reasons = append(reasons, "direct path/title match")
		case match.Body:
			delta = 8.0
			tier = PackTierRelated
			tierReason = "explicit exact plan/track ID reference"
			reasons = append(reasons, "explicit body reference")
		case exactIntentIDDampensCandidate(candidates[i], match):
			delta = -18.0
			tier = PackTierRelated
			tierReason = "query contains exact plan/track ID but this artifact does not"
			reasons = append(reasons, "no direct ID match")
			if match.PartsOnly {
				reasons = append(reasons, "partial ID-token overlap only")
			}
		default:
			continue
		}
		candidates[i].score += delta
		candidates[i].candidate = withExactIntentIDMetadata(candidates[i].candidate, delta, match, tier, tierReason, reasons)
		changed = true
	}
	return candidates, changed
}

func buildExactIntentIDProfile(query string) exactIntentIDProfile {
	seen := map[string]bool{}
	var profile exactIntentIDProfile
	for _, raw := range tokenizeAnchorOriginal(query) {
		id, ok := exactIntentIDFromToken(raw)
		if !ok || seen[id.Norm] {
			continue
		}
		seen[id.Norm] = true
		profile.IDs = append(profile.IDs, id)
	}
	return profile
}

func exactIntentIDFromToken(raw string) (exactIntentID, bool) {
	display := strings.Trim(raw, "`'\".,:;()[]{}")
	norm := normalizeExactIntentID(display)
	if norm == "" || !looksLikeExactIntentID(display, norm) {
		return exactIntentID{}, false
	}
	parts := exactIntentIDParts(norm)
	return exactIntentID{
		Display: display,
		Norm:    norm,
		Compact: compactExactIntentID(norm),
		Parts:   parts,
	}, true
}

func looksLikeExactIntentID(raw, norm string) bool {
	if len(norm) < 2 || len(norm) > 28 {
		return false
	}
	if strings.ContainsAny(raw, "/\\") {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range norm {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return false
	}
	if strings.ContainsAny(norm, "-_.") {
		return len(exactIntentIDParts(norm)) >= 2
	}
	runes := []rune(norm)
	if len(runes) < 3 || len(runes) > 8 {
		return false
	}
	letterPrefix := 0
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letterPrefix++
			continue
		}
		break
	}
	if letterPrefix == 0 || letterPrefix > 4 || letterPrefix == len(runes) {
		return false
	}
	for _, r := range runes[letterPrefix:] {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func exactIntentIDPresent(candidates []Candidate, profile exactIntentIDProfile) bool {
	for _, candidate := range candidates {
		match := exactIntentIDCandidateMatch(candidate, profile)
		if match.PathTitle || match.Body {
			return true
		}
	}
	return false
}

func exactIntentIDCandidateMatch(c Candidate, profile exactIntentIDProfile) exactIntentIDMatch {
	pathTitle := normalizeExactIntentIDText(c.Path + "\n" + c.Title)
	pathTitleCompact := compactExactIntentID(pathTitle)
	body := normalizeExactIntentIDText(c.Body)
	var match exactIntentIDMatch
	for _, id := range profile.IDs {
		if id.Norm == "" {
			continue
		}
		switch {
		case strings.Contains(pathTitle, id.Norm) || (id.Compact != "" && strings.Contains(pathTitleCompact, id.Compact)):
			match.PathTitle = true
			match.Matched = appendUniqueString(match.Matched, id.Display)
		case strings.Contains(body, id.Norm):
			match.Body = true
			match.Matched = appendUniqueString(match.Matched, id.Display)
		case exactIntentIDPartsMatch(pathTitle+"\n"+body, id.Parts):
			match.PartsOnly = true
		}
	}
	return match
}

func exactIntentIDDampensCandidate(candidate scoredCandidate, match exactIntentIDMatch) bool {
	if match.PathTitle || match.Body {
		return false
	}
	role := candidate.role
	if isIntentAuthorityRole(role) || IsPlanningIntentPath(candidate.candidate.Path) || isMarkdownCandidatePath(candidate.candidate.Path) {
		return true
	}
	return false
}

func withExactIntentIDMetadata(c Candidate, delta float64, match exactIntentIDMatch, tier, tierReason string, reasons []string) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["exact_intent_id_score"] = fmt.Sprintf("%.3f", delta)
	if len(match.Matched) > 0 {
		c.Metadata["exact_intent_id_matches_json"] = jsonStringList(match.Matched)
	}
	if len(reasons) > 0 {
		c.Metadata["exact_intent_id_reasons"] = strings.Join(uniqueStrings(reasons), "\n")
	}
	if tier != "" {
		c.Metadata["pack_tier"] = tier
	}
	if tierReason != "" {
		c.Metadata["pack_tier_reason"] = tierReason
	}
	return c
}

func normalizeExactIntentID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, "`'\".,:;()[]{}")
	value = strings.ReplaceAll(value, "_", "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}

func normalizeExactIntentIDText(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, "\\", "/")
	return value
}

func compactExactIntentID(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func exactIntentIDParts(value string) []string {
	parts := splitIdentifierParts(value)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeAnchorTerm(part)
		if len(part) < 2 {
			continue
		}
		out = append(out, part)
	}
	return uniqueStrings(out)
}

func exactIntentIDPartsMatch(text string, parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	matched := 0
	for _, part := range parts {
		if strings.Contains(text, part) {
			matched++
		}
	}
	return matched > 0 && matched < len(parts)
}

func exactAnchorBackfillCandidates(universe []Candidate, seen map[string]bool, profile exactQueryAnchorProfile, queryLower string, terms map[string]float64) []scoredCandidate {
	var scored []scoredCandidate
	for _, c := range universe {
		if seen[candidateIdentity(c)] {
			continue
		}
		score := scoreExactAnchorCandidate(c, profile)
		if !exactAnchorBackfillAllowed(c, score) {
			continue
		}
		role := candidateRole(c)
		prior := authorityPrior(c, role, queryLower)
		c = withExactAnchorMetadata(c, score)
		c.Metadata["exact_anchor_backfill"] = "true"
		baseScore := scoreCandidate(c, terms, queryLower)
		scored = append(scored, scoredCandidate{
			candidate:  c,
			score:      maxFloat(baseScore, 0) + score.score + prior.score,
			baseScore:  baseScore,
			authority:  prior,
			profile:    candidateMatchProfile(c, queryLower),
			role:       role,
			sourceFile: IsSourceContextCandidate(c),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].candidate.Path < scored[j].candidate.Path
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > exactAnchorBackfillLimit {
		scored = scored[:exactAnchorBackfillLimit]
	}
	return scored
}

func exactAnchorBackfillAllowed(c Candidate, score exactAnchorScore) bool {
	if score.score < 10.0 || len(score.matches) == 0 {
		return false
	}
	if resultFieldsContain(score.fields, "path", "title", "test_name", "symbol", "heading") {
		return true
	}
	return score.score >= 14.0 && packSpecificCandidateOverlap(c, strings.Join(score.matches, " ")) > 0
}

func candidateCanBeGenericDilution(candidate scoredCandidate, queryLower string) bool {
	if candidate.role == "test_case" || candidate.role == "code_comment" {
		return false
	}
	if hasExplicitSourceIntent(queryLower) && candidate.sourceFile {
		return false
	}
	return true
}

func buildExactQueryAnchorProfile(query string) exactQueryAnchorProfile {
	seenSpecific := map[string]bool{}
	seenGeneric := map[string]bool{}
	var profile exactQueryAnchorProfile
	addSpecific := func(term string) {
		term = normalizeAnchorTerm(term)
		if term == "" || seenSpecific[term] {
			return
		}
		seenSpecific[term] = true
		profile.Specific = append(profile.Specific, term)
	}
	addGeneric := func(term string) {
		term = normalizeAnchorTerm(term)
		if term == "" || seenGeneric[term] {
			return
		}
		seenGeneric[term] = true
		profile.Generic = append(profile.Generic, term)
	}
	for _, raw := range tokenizeAnchorOriginal(query) {
		term := normalizeAnchorTerm(raw)
		if term == "" || anchorGenericTaskWords[term] || anchorRoleTerms[term] {
			continue
		}
		if exactAnchorGenericTerms[term] || anchorWeakBroadTerms[term] {
			addGeneric(term)
			continue
		}
		if exactQueryTokenSpecific(raw, term) {
			addSpecific(term)
		}
		for _, part := range splitIdentifierParts(raw) {
			part = normalizeAnchorTerm(part)
			if exactAnchorGenericTerms[part] || anchorWeakBroadTerms[part] {
				addGeneric(part)
				continue
			}
			if exactQueryTokenSpecific(part, part) {
				addSpecific(part)
			}
		}
	}
	return profile
}

func exactQueryTokenSpecific(raw, term string) bool {
	if len(term) < 2 {
		return false
	}
	if len(term) == 2 {
		return allLetters(term) && !exactAnchorGenericTerms[term]
	}
	if strings.ContainsAny(term, "0123456789") {
		return true
	}
	if strings.ContainsAny(raw, "_-.\\/") {
		return true
	}
	if len(term) <= 6 {
		return true
	}
	return looksLikeCompactIdentifierToken(raw)
}

func scoreExactAnchorCandidate(c Candidate, profile exactQueryAnchorProfile) exactAnchorScore {
	fields := metadataAnchorFieldTerms(c, true)
	var result exactAnchorScore
	for _, term := range profile.Specific {
		score, matched := exactAnchorFieldScore(term, fields)
		if score <= 0 {
			continue
		}
		result.score += score
		result.matches = appendUniqueString(result.matches, term)
		result.fields = append(result.fields, matched...)
	}
	if result.score == 0 {
		for _, term := range profile.Generic {
			if score, _ := exactAnchorFieldScore(term, fields); score > 0 {
				result.genericMatches = appendUniqueString(result.genericMatches, term)
			}
		}
	}
	if len(result.fields) > 0 {
		result.fields = uniqueStrings(result.fields)
	}
	if result.score > 24.0 {
		result.score = 24.0
	}
	return result
}

func exactAnchorFieldScore(term string, fields anchorFieldTerms) (float64, []string) {
	var score float64
	var matched []string
	add := func(name string, terms map[string]int, weight float64, capCount int) {
		count := terms[term]
		if count <= 0 {
			return
		}
		if capCount > 0 && count > capCount {
			count = capCount
		}
		score += float64(count) * weight
		matched = append(matched, name)
	}
	add("path", fields.path, 9.0, 3)
	add("title", fields.title, 7.0, 2)
	add("test_name", fields.testName, 8.0, 2)
	add("symbol", fields.symbol, 6.0, 3)
	add("heading", fields.heading, 5.0, 3)
	add("role", fields.role, 1.0, 1)
	add("body", fields.body, 2.8, 5)
	if resultFieldsContain(matched, "path", "title", "test_name", "symbol") {
		score += 3.0
	}
	return score, matched
}

func withExactAnchorMetadata(c Candidate, score exactAnchorScore) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["exact_anchor_score"] = fmt.Sprintf("%.3f", score.score)
	c.Metadata["exact_anchor_matches_json"] = jsonStringList(score.matches)
	c.Metadata["exact_anchor_fields_json"] = jsonStringList(score.fields)
	return c
}

func resultFieldsContain(fields []string, wants ...string) bool {
	want := map[string]bool{}
	for _, value := range wants {
		want[value] = true
	}
	for _, field := range fields {
		if want[field] {
			return true
		}
	}
	return false
}

func allLetters(value string) bool {
	for _, r := range value {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return value != ""
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
