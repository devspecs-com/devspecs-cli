package retrieval

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type AnchorKind string

const (
	AnchorCompactIdentifier AnchorKind = "compact_identifier"
	AnchorProperOrRare      AnchorKind = "proper_or_rare_term"
	AnchorQuotedPhrase      AnchorKind = "quoted_phrase"
	AnchorPathLike          AnchorKind = "path_like"
	AnchorArtifactRoleTerm  AnchorKind = "artifact_role_term"
	AnchorGenericTaskWord   AnchorKind = "generic_task_word"
)

type AnchorTerm struct {
	Term     string     `json:"term"`
	Original string     `json:"original,omitempty"`
	Kind     AnchorKind `json:"kind"`
}

type AnchorProfile struct {
	Anchors     []AnchorTerm `json:"anchors"`
	HasSpecific bool         `json:"has_specific"`
}

const (
	AnchorFirstModeV1           = "v1"
	AnchorFirstModeRerankOnly   = "rerank_only"
	AnchorFirstModeSelectedOnly = "selected_only"
	AnchorFirstModeStrongField  = "strong_field"
	AnchorFirstModeStrict       = "strict"
	DefaultAnchorFirstMode      = AnchorFirstModeSelectedOnly
)

func NormalizeAnchorFirstMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return DefaultAnchorFirstMode
	case AnchorFirstModeV1:
		return AnchorFirstModeV1
	case "rerank-only", AnchorFirstModeRerankOnly:
		return AnchorFirstModeRerankOnly
	case "selected-only", AnchorFirstModeSelectedOnly:
		return AnchorFirstModeSelectedOnly
	case "strong-field", AnchorFirstModeStrongField:
		return AnchorFirstModeStrongField
	case AnchorFirstModeStrict:
		return AnchorFirstModeStrict
	default:
		return ""
	}
}

func ValidAnchorFirstModes() []string {
	return []string{AnchorFirstModeV1, AnchorFirstModeRerankOnly, AnchorFirstModeSelectedOnly, AnchorFirstModeStrongField, AnchorFirstModeStrict}
}

type RepoVocabulary struct {
	Terms         map[string]TermStats `json:"terms"`
	DocumentCount int                  `json:"document_count"`
}

type TermStats struct {
	TotalCount    int     `json:"total_count"`
	DocumentCount int     `json:"document_count"`
	PathCount     int     `json:"path_count"`
	TitleCount    int     `json:"title_count"`
	HeadingCount  int     `json:"heading_count"`
	TestNameCount int     `json:"test_name_count"`
	SymbolCount   int     `json:"symbol_count"`
	RoleCount     int     `json:"role_count"`
	IDF           float64 `json:"idf"`
}

type anchorScoreResult struct {
	score          float64
	matches        []string
	fields         []string
	kinds          []string
	frequencies    []string
	strongSpecific bool
	maxIDF         float64
}

type anchorFieldTerms struct {
	path     map[string]int
	title    map[string]int
	heading  map[string]int
	testName map[string]int
	symbol   map[string]int
	role     map[string]int
	body     map[string]int
}

func BuildAnchorProfile(query string) AnchorProfile {
	seen := map[string]bool{}
	var anchors []AnchorTerm
	hasSpecific := false
	add := func(term, original string, kind AnchorKind) {
		term = normalizeAnchorTerm(term)
		if term == "" {
			return
		}
		key := string(kind) + "\x00" + term
		if seen[key] {
			return
		}
		seen[key] = true
		anchors = append(anchors, AnchorTerm{Term: term, Original: original, Kind: kind})
		if kind == AnchorCompactIdentifier || kind == AnchorProperOrRare || kind == AnchorQuotedPhrase || kind == AnchorPathLike {
			hasSpecific = true
		}
	}

	for _, phrase := range quotedPhrases(query) {
		add(phrase, phrase, AnchorQuotedPhrase)
	}
	for _, token := range tokenizeAnchorOriginal(query) {
		lower := strings.ToLower(token)
		switch {
		case anchorGenericTaskWords[lower]:
			add(lower, token, AnchorGenericTaskWord)
			continue
		case anchorRoleTerms[lower]:
			add(lower, token, AnchorArtifactRoleTerm)
			continue
		}
		if isPathLikeAnchorToken(token) {
			add(lower, token, AnchorPathLike)
			if compact := compactIdentifier(token); compact != "" && compact != lower {
				add(compact, token, AnchorCompactIdentifier)
			}
			continue
		}
		if looksLikeCompactIdentifierToken(token) || looksLikeCompactTestIdentifier(lower) {
			add(compactIdentifier(token), token, AnchorCompactIdentifier)
			if len(lower) >= 4 && !anchorWeakBroadTerms[lower] && !anchorRoleTerms[lower] {
				add(lower, token, AnchorProperOrRare)
			}
			continue
		}
		if len(lower) >= 4 && !anchorWeakBroadTerms[lower] {
			add(lower, token, AnchorProperOrRare)
		} else if anchorWeakBroadTerms[lower] {
			add(lower, token, AnchorArtifactRoleTerm)
		}
	}
	return AnchorProfile{Anchors: anchors, HasSpecific: hasSpecific}
}

func BuildRepoVocabulary(candidates []Candidate) RepoVocabulary {
	vocab := RepoVocabulary{Terms: map[string]TermStats{}, DocumentCount: len(candidates)}
	for _, c := range candidates {
		fields := metadataAnchorFieldTerms(c, false)
		documentTerms := map[string]bool{}
		addField := func(terms map[string]int, apply func(*TermStats, int)) {
			for term, count := range terms {
				if !vocabularyTermAllowed(term) {
					continue
				}
				stats := vocab.Terms[term]
				stats.TotalCount += count
				apply(&stats, count)
				vocab.Terms[term] = stats
				documentTerms[term] = true
			}
		}
		addField(fields.path, func(stats *TermStats, count int) { stats.PathCount += count })
		addField(fields.title, func(stats *TermStats, count int) { stats.TitleCount += count })
		addField(fields.heading, func(stats *TermStats, count int) { stats.HeadingCount += count })
		addField(fields.testName, func(stats *TermStats, count int) { stats.TestNameCount += count })
		addField(fields.symbol, func(stats *TermStats, count int) { stats.SymbolCount += count })
		addField(fields.role, func(stats *TermStats, count int) { stats.RoleCount += count })
		for term := range documentTerms {
			stats := vocab.Terms[term]
			stats.DocumentCount++
			vocab.Terms[term] = stats
		}
	}
	totalDocs := float64(maxInt(1, vocab.DocumentCount))
	for term, stats := range vocab.Terms {
		stats.IDF = math.Log((1.0+totalDocs)/(1.0+float64(stats.DocumentCount))) + 1.0
		vocab.Terms[term] = stats
	}
	return vocab
}

func applyAnchorFirstRanking(candidates []scoredCandidate, universe []Candidate, query, mode string) []scoredCandidate {
	mode = NormalizeAnchorFirstMode(mode)
	if mode == "" {
		mode = AnchorFirstModeV1
	}
	profile := BuildAnchorProfile(query)
	if !profile.HasSpecific || len(candidates) == 0 {
		return candidates
	}
	vocab := BuildRepoVocabulary(universe)
	changed := false
	seen := map[string]bool{}
	for i := range candidates {
		seen[candidateIdentity(candidates[i].candidate)] = true
		if !anchorFirstCandidateEligible(candidates[i].candidate, profile) {
			continue
		}
		result := scoreAnchorFirstCandidate(candidates[i].candidate, profile, vocab, mode)
		if !anchorFirstPrimaryBoostAllowed(candidates[i].candidate, result, profile, mode) {
			continue
		}
		delta := clampFloat(result.score, 0, 24)
		candidates[i].score += delta
		candidates[i].candidate = withAnchorFirstMetadata(candidates[i].candidate, delta, result, mode)
		changed = true
	}
	if mode == AnchorFirstModeRerankOnly || mode == AnchorFirstModeSelectedOnly {
		if changed {
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].score == candidates[j].score {
					return candidates[i].candidate.Path < candidates[j].candidate.Path
				}
				return candidates[i].score > candidates[j].score
			})
		}
		return candidates
	}
	backfilled := 0
	for _, c := range universe {
		if seen[candidateIdentity(c)] {
			continue
		}
		if !anchorFirstCandidateEligible(c, profile) {
			continue
		}
		result := scoreAnchorFirstCandidate(c, profile, vocab, mode)
		if !anchorFirstBackfillAllowed(result, mode) {
			continue
		}
		role := candidateRole(c)
		prior := authorityPrior(c, role, strings.ToLower(query))
		delta := clampFloat(result.score, 0, 24)
		c = withAnchorFirstMetadata(c, delta, result, mode)
		c.Metadata["anchor_first_backfill"] = "true"
		candidates = append(candidates, scoredCandidate{
			candidate:  c,
			score:      delta + prior.score,
			baseScore:  0,
			authority:  prior,
			profile:    candidateMatchProfile(c, strings.ToLower(query)),
			role:       role,
			sourceFile: IsSourceContextCandidate(c),
		})
		seen[candidateIdentity(c)] = true
		changed = true
		backfilled++
		if mode != AnchorFirstModeV1 && backfilled >= 1 {
			break
		}
	}
	if changed {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].score == candidates[j].score {
				return candidates[i].candidate.Path < candidates[j].candidate.Path
			}
			return candidates[i].score > candidates[j].score
		})
	}
	return candidates
}

func anchorFirstCandidateEligible(c Candidate, profile AnchorProfile) bool {
	if !agentOrProtocolInstructionCandidate(c) {
		return true
	}
	return anchorProfileProtocolOriented(profile)
}

func anchorProfileProtocolOriented(profile AnchorProfile) bool {
	for _, anchor := range profile.Anchors {
		switch anchor.Term {
		case "agent", "agents", "boilerplate", "claude", "codex", "command", "commands", "convention", "conventions", "cursor", "governance", "instruction", "instructions", "maintainer", "maintainers", "policy", "policies", "procedure", "procedures", "prompt", "prompts", "protocol", "rule", "rules", "scaffold", "scaffolding", "skill", "skills", "standard", "standards", "template", "templates":
			return true
		}
	}
	return false
}

func agentOrProtocolInstructionCandidate(c Candidate) bool {
	path := strings.ToLower(filepath.ToSlash(c.Path))
	base := filepath.Base(path)
	switch base {
	case "agents.md", "claude.md", "codeowners", "governance.md", "maintainers.md", "skill.md":
		return true
	}
	if strings.HasSuffix(base, ".agent.md") {
		return true
	}
	if strings.Contains(path, "/.claude/") ||
		strings.Contains(path, "/.codex/skills/") ||
		strings.Contains(path, "/.cursor/rules/") ||
		strings.Contains(path, "/.github/issue_template/") {
		return true
	}
	return false
}

func anchorFirstPrimaryBoostAllowed(c Candidate, result anchorScoreResult, profile AnchorProfile, mode string) bool {
	if result.score < 1.0 {
		return false
	}
	if !anchorFirstCandidateEligible(c, profile) {
		return false
	}
	if result.strongSpecific {
		return true
	}
	if resultHasAnyAnchorField(result, "path", "title", "test_name") && resultHasAnyAnchorKind(result,
		string(AnchorCompactIdentifier),
		string(AnchorPathLike),
		string(AnchorQuotedPhrase),
	) {
		return true
	}
	if mode == AnchorFirstModeV1 && resultHasAnyAnchorKind(result, string(AnchorCompactIdentifier), string(AnchorPathLike), string(AnchorQuotedPhrase)) {
		return result.maxIDF >= 1.8 && resultHasAnyAnchorField(result, "heading", "symbol")
	}
	return false
}

func scoreAnchorFirstCandidate(c Candidate, profile AnchorProfile, vocab RepoVocabulary, mode string) anchorScoreResult {
	fields := metadataAnchorFieldTerms(c, true)
	result := anchorScoreResult{}
	hasSpecificMatch := false
	for _, anchor := range profile.Anchors {
		modifier := anchorDictionaryModifier(anchor, profile.HasSpecific, mode)
		if modifier <= 0 {
			continue
		}
		tf, matchedFields := fieldWeightedTF(anchor.Term, fields, anchor.Kind, mode)
		if tf <= 0 {
			continue
		}
		stats := vocab.Terms[anchor.Term]
		idf := stats.IDF
		if idf <= 0 {
			idf = 1.0
		}
		idfWeight := idf - 1.0
		if anchor.Kind == AnchorCompactIdentifier && idfWeight < 1.0 {
			idfWeight = 1.0
		} else if idfWeight < 0 {
			idfWeight = 0
		}
		score := tf * idfWeight * modifier
		if anchor.Kind == AnchorArtifactRoleTerm && !hasSpecificMatch {
			score *= 0.25
		}
		if score <= 0 {
			continue
		}
		if anchor.Kind != AnchorArtifactRoleTerm {
			hasSpecificMatch = true
			if anchorStrongSpecificMatch(anchor, stats, matchedFields, mode) {
				result.strongSpecific = true
			}
		}
		if idf > result.maxIDF {
			result.maxIDF = idf
		}
		result.score += score
		result.matches = append(result.matches, anchor.Term)
		result.kinds = append(result.kinds, string(anchor.Kind))
		result.fields = append(result.fields, matchedFields...)
		result.frequencies = append(result.frequencies, fmt.Sprintf("%s:df=%d:idf=%.2f", anchor.Term, stats.DocumentCount, idf))
	}
	if result.score > 0 {
		result.matches = uniqueStrings(result.matches)
		result.kinds = uniqueStrings(result.kinds)
		result.fields = uniqueStrings(result.fields)
		result.frequencies = uniqueStrings(result.frequencies)
	}
	return result
}

func withAnchorFirstMetadata(c Candidate, score float64, result anchorScoreResult, mode string) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["anchor_first_mode"] = mode
	c.Metadata["anchor_first_score"] = fmt.Sprintf("%.3f", score)
	c.Metadata["anchor_matches_json"] = jsonStringList(result.matches)
	c.Metadata["anchor_fields_json"] = jsonStringList(result.fields)
	c.Metadata["anchor_types_json"] = jsonStringList(result.kinds)
	c.Metadata["anchor_term_frequency_json"] = jsonStringList(result.frequencies)
	return c
}

func fieldWeightedTF(term string, fields anchorFieldTerms, kind AnchorKind, mode string) (float64, []string) {
	if strings.Contains(term, " ") {
		return phraseFieldWeightedTF(term, fields, mode)
	}
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
	add("path", fields.path, 4.5, 2)
	add("title", fields.title, 4.0, 2)
	add("test_name", fields.testName, 5.0, 2)
	add("heading", fields.heading, 3.0, 3)
	add("symbol", fields.symbol, 2.5, 3)
	roleWeight := 1.2
	bodyWeight := 0.2
	if mode == AnchorFirstModeStrongField {
		roleWeight = 0.35
		bodyWeight = 0
	}
	if mode == AnchorFirstModeStrict {
		roleWeight = 0
		bodyWeight = 0
	}
	add("role", fields.role, roleWeight, 2)
	add("body", fields.body, bodyWeight, 3)
	if kind == AnchorCompactIdentifier {
		score *= 1.25
	}
	return score, matched
}

func phraseFieldWeightedTF(phrase string, fields anchorFieldTerms, mode string) (float64, []string) {
	var score float64
	var matched []string
	add := func(name string, terms map[string]int, weight float64) {
		compactPhrase := normalizeAnchorTerm(phrase)
		if terms[compactPhrase] <= 0 {
			return
		}
		score += float64(terms[compactPhrase]) * weight
		matched = append(matched, name)
	}
	add("path", fields.path, 4.5)
	add("title", fields.title, 4.0)
	add("test_name", fields.testName, 5.0)
	add("heading", fields.heading, 3.0)
	if mode == AnchorFirstModeV1 || mode == AnchorFirstModeRerankOnly || mode == AnchorFirstModeSelectedOnly {
		add("body", fields.body, 0.45)
	}
	return score, matched
}

func metadataAnchorFieldTerms(c Candidate, includeBody bool) anchorFieldTerms {
	fields := anchorFieldTerms{
		path:     map[string]int{},
		title:    map[string]int{},
		heading:  map[string]int{},
		testName: map[string]int{},
		symbol:   map[string]int{},
		role:     map[string]int{},
		body:     map[string]int{},
	}
	addPathTerms(fields.path, c.Path)
	addTextTerms(fields.title, c.Title)
	for _, section := range c.Sections {
		addTextTerms(fields.heading, section.HeadingPath)
		addTextTerms(fields.heading, section.Title)
	}
	if c.Metadata != nil {
		addTextTerms(fields.testName, c.Metadata["test_name"])
		addTextTerms(fields.testName, c.Metadata["parent_title"])
		for _, heading := range metadataJSONList(c, "indexed_section_match_headings_json") {
			addTextTerms(fields.heading, heading)
		}
		for _, key := range []string{"symbols", "source_symbols", "concept_symbols"} {
			addTextTerms(fields.symbol, c.Metadata[key])
		}
	}
	if isTestCaseCandidate(c) {
		for _, anchor := range testNameAnchors(c) {
			addTerm(fields.testName, anchor.compact)
			addTerm(fields.testName, anchor.withoutTestPrefix)
			for _, part := range anchor.parts {
				addTerm(fields.testName, part)
			}
		}
	}
	addTextTerms(fields.role, c.Kind)
	addTextTerms(fields.role, c.Subtype)
	addTextTerms(fields.role, candidateRole(c))
	if mode := nonIntentCandidateMode(c); mode != "" {
		addTextTerms(fields.role, mode)
	}
	if includeBody {
		body := c.Body
		if len(body) > 16000 {
			body = body[:16000]
		}
		addTextTerms(fields.body, body)
	}
	return fields
}

func addPathTerms(out map[string]int, path string) {
	path = filepath.ToSlash(path)
	addTextTerms(out, path)
	base := filepath.Base(path)
	addTextTerms(out, strings.TrimSuffix(base, filepath.Ext(base)))
	for _, segment := range strings.Split(path, "/") {
		addTextTerms(out, strings.TrimSuffix(segment, filepath.Ext(segment)))
	}
}

func addTextTerms(out map[string]int, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	lowerFull := normalizeAnchorTerm(text)
	addTerm(out, lowerFull)
	for _, token := range tokenizeAnchorOriginal(text) {
		addTerm(out, strings.ToLower(token))
		for _, part := range splitIdentifierParts(token) {
			addTerm(out, part)
		}
		if compact := compactIdentifier(token); compact != "" {
			addTerm(out, compact)
		}
	}
}

func addTerm(out map[string]int, term string) {
	term = normalizeAnchorTerm(term)
	if term == "" {
		return
	}
	out[term]++
}

func anchorDictionaryModifier(anchor AnchorTerm, hasSpecific bool, mode string) float64 {
	switch anchor.Kind {
	case AnchorGenericTaskWord:
		return 0
	case AnchorArtifactRoleTerm:
		if mode == AnchorFirstModeStrict {
			return 0
		}
		if !hasSpecific {
			return 0.05
		}
		if mode == AnchorFirstModeStrongField {
			if anchorWeakBroadTerms[anchor.Term] {
				return 0
			}
			return 0.08
		}
		if anchorWeakBroadTerms[anchor.Term] {
			return 0.08
		}
		return 0.18
	case AnchorCompactIdentifier:
		return 1.8
	case AnchorPathLike:
		return 1.35
	case AnchorQuotedPhrase:
		return 1.4
	case AnchorProperOrRare:
		if mode == AnchorFirstModeStrict {
			return 0.45
		}
		return 1.05
	default:
		return 1
	}
}

func anchorStrongSpecificMatch(anchor AnchorTerm, stats TermStats, fields []string, mode string) bool {
	if anchor.Kind == AnchorArtifactRoleTerm || anchor.Kind == AnchorGenericTaskWord {
		return false
	}
	if anchorWeakBroadTerms[anchor.Term] {
		return false
	}
	if mode == AnchorFirstModeStrict && anchor.Kind == AnchorProperOrRare {
		return false
	}
	if stats.IDF > 0 && stats.IDF < 1.8 && anchor.Kind != AnchorCompactIdentifier {
		return false
	}
	for _, field := range fields {
		switch field {
		case "path", "title", "test_name":
			return true
		}
	}
	return false
}

func anchorFirstBackfillAllowed(result anchorScoreResult, mode string) bool {
	if !result.strongSpecific {
		return false
	}
	switch mode {
	case AnchorFirstModeV1:
		return result.score >= 10.0
	case AnchorFirstModeStrongField:
		return result.score >= 12.0 && result.maxIDF >= 1.8 && resultHasAnyAnchorField(result, "path", "title", "test_name")
	case AnchorFirstModeStrict:
		return result.score >= 14.0 && result.maxIDF >= 1.8 && resultHasAnyAnchorField(result, "path", "title", "test_name") && resultHasAnyAnchorKind(result, string(AnchorCompactIdentifier), string(AnchorPathLike), string(AnchorQuotedPhrase))
	default:
		return false
	}
}

func resultHasAnyAnchorField(result anchorScoreResult, fields ...string) bool {
	want := map[string]bool{}
	for _, field := range fields {
		want[field] = true
	}
	for _, field := range result.fields {
		if want[field] {
			return true
		}
	}
	return false
}

func resultHasAnyAnchorKind(result anchorScoreResult, kinds ...string) bool {
	want := map[string]bool{}
	for _, kind := range kinds {
		want[kind] = true
	}
	for _, kind := range result.kinds {
		if want[kind] {
			return true
		}
	}
	return false
}

func vocabularyTermAllowed(term string) bool {
	term = normalizeAnchorTerm(term)
	if term == "" {
		return false
	}
	if len(term) < 3 && !anchorRoleTerms[term] {
		return false
	}
	if anchorGenericTaskWords[term] {
		return false
	}
	return true
}

func normalizeAnchorTerm(term string) string {
	term = strings.ToLower(strings.TrimSpace(term))
	term = strings.Trim(term, "`'\"")
	return term
}

func compactIdentifier(value string) string {
	parts := splitIdentifierParts(value)
	if len(parts) == 0 {
		return normalizeAnchorTerm(value)
	}
	return strings.Join(filterEmptyStrings(parts), "")
}

func isPathLikeAnchorToken(token string) bool {
	return strings.ContainsAny(token, "/\\_.-") || strings.Contains(filepath.Base(token), ".")
}

func looksLikeCompactIdentifierToken(token string) bool {
	if len(token) < 8 {
		return false
	}
	hasLower := false
	hasUpper := false
	hasDigit := false
	for _, r := range token {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case r == '_' || r == '-' || r == '.':
			return true
		}
	}
	return (hasLower && hasUpper) || hasDigit
}

func tokenizeAnchorOriginal(s string) []string {
	var terms []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		terms = append(terms, b.String())
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/' || r == '\\' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return terms
}

func quotedPhrases(s string) []string {
	var phrases []string
	var b strings.Builder
	inQuote := false
	var quote rune
	for _, r := range s {
		if r == '"' || r == '\'' {
			if inQuote && r == quote {
				phrase := strings.TrimSpace(b.String())
				if phrase != "" {
					phrases = append(phrases, phrase)
				}
				b.Reset()
				inQuote = false
				quote = 0
				continue
			}
			if !inQuote {
				inQuote = true
				quote = r
				continue
			}
		}
		if inQuote {
			b.WriteRune(r)
		}
	}
	return phrases
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var anchorGenericTaskWords = map[string]bool{
	"a": true, "add": true, "an": true, "and": true, "are": true,
	"build": true, "change": true, "check": true, "continue": true,
	"cover": true, "covers": true, "covered": true, "context": true, "do": true,
	"fix": true, "for": true, "from": true, "generate": true, "get": true,
	"help": true, "how": true, "implement": true, "improve": true, "into": true,
	"make": true, "need": true, "next": true, "or": true, "run": true,
	"same": true, "share": true, "shared": true, "show": true, "so": true,
	"that": true, "the": true, "these": true, "this": true, "those": true,
	"to": true, "update": true, "use": true, "what": true, "when": true,
	"where": true, "which": true, "who": true, "why": true, "with": true,
	"without": true, "work": true, "working": true, "behavior": true,
	"behaviour": true,
}

var anchorRoleTerms = map[string]bool{
	"adr": true, "adrs": true, "architecture": true, "decision": true,
	"design": true, "document": true, "documents": true, "plan": true,
	"plans": true, "prd": true, "prds": true, "proposal": true,
	"requirements": true, "requirement": true, "rfc": true, "rfcs": true,
	"skill": true, "skills": true, "spec": true, "template": true,
	"templates": true, "test": true, "tests": true,
}

var anchorWeakBroadTerms = map[string]bool{
	"api": true, "architecture": true, "config": true, "context": true,
	"design": true, "docs": true, "document": true, "integration": true,
	"mode": true, "requirements": true, "service": true, "spec": true, "system": true,
	"template": true, "test": true,
}
