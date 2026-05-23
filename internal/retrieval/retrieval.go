package retrieval

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
)

// Candidate is the shared retrieval unit used by eval and live CLI commands.
// Path and Body are the minimum fields required by the v0 file retriever; the
// remaining fields give indexed commands room to preserve artifact metadata.
type Candidate struct {
	ID       string
	Path     string
	Kind     string
	Subtype  string
	Title    string
	Status   string
	Body     string
	Source   string
	Metadata map[string]string
	Sections []IndexedSection `json:"sections,omitempty"`
}

type IndexedSection struct {
	ID            string            `json:"id,omitempty"`
	ArtifactID    string            `json:"artifact_id,omitempty"`
	RevisionID    string            `json:"revision_id,omitempty"`
	SourcePath    string            `json:"source_path,omitempty"`
	HeadingPath   string            `json:"heading_path,omitempty"`
	HeadingDepth  int               `json:"heading_depth,omitempty"`
	StartLine     int               `json:"start_line,omitempty"`
	EndLine       int               `json:"end_line,omitempty"`
	Title         string            `json:"title,omitempty"`
	Body          string            `json:"body,omitempty"`
	TokenEstimate int               `json:"token_estimate,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type Retriever interface {
	Name() string
	Retrieve(candidates []Candidate, query string) []Candidate
}

const EvidenceModeBalanced = "balanced"

const (
	PackTierPrimary    = "primary"
	PackTierRelated    = "related"
	PackTierDiagnostic = "diagnostic"
)

type WeightedFilesRetrieverV0 struct {
	DisableSectionAware bool
	EvidenceMode        string
	ConceptBackfill     bool
	GlossaryConcepts    bool
	TieredConceptOutput bool
}

func CandidatePackTier(c Candidate) string {
	if c.Metadata != nil {
		if tier := strings.TrimSpace(c.Metadata["pack_tier"]); tier != "" {
			return strings.ToLower(tier)
		}
	}
	return PackTierPrimary
}

func (r WeightedFilesRetrieverV0) Name() string {
	suffix := ""
	if r.ConceptBackfill {
		suffix = "_concept_backfill"
	}
	if r.GlossaryConcepts {
		suffix += "_glossary"
	}
	if r.TieredConceptOutput {
		suffix += "_tiered"
	}
	if strings.EqualFold(r.EvidenceMode, EvidenceModeBalanced) {
		if r.DisableSectionAware {
			return "eval_weighted_files_v0_evidence_balanced_no_section_retrieval" + suffix
		}
		return "eval_weighted_files_v0_evidence_balanced" + suffix
	}
	if r.DisableSectionAware {
		return "eval_weighted_files_v0_no_section_retrieval" + suffix
	}
	return "eval_weighted_files_v0" + suffix
}

func (r WeightedFilesRetrieverV0) Retrieve(candidates []Candidate, query string) []Candidate {
	if !r.DisableSectionAware {
		candidates = EnrichCandidatesWithSectionMatches(candidates, query)
	}
	return retrieveWeightedFilesV0(candidates, query, r.EvidenceMode, r.ConceptBackfill, r.GlossaryConcepts, r.TieredConceptOutput)
}

type Reason struct {
	Path    string   `json:"path"`
	Reasons []string `json:"reasons"`
}

func CandidatePaths(candidates []Candidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.Path
	}
	return out
}

func QueryBaseline(candidates []Candidate, query string) []Candidate {
	terms := meaningfulTerms(query)
	var out []Candidate
	for _, c := range candidates {
		haystack := strings.ToLower(c.Path + "\n" + c.Body)
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func ExplainCandidates(candidates []Candidate, query string) []Reason {
	out := make([]Reason, 0, len(candidates))
	terms := expandedTerms(query)
	queryLower := strings.ToLower(query)
	for _, c := range candidates {
		out = append(out, Reason{
			Path:    c.Path,
			Reasons: reasonsForCandidate(c, terms, queryLower),
		})
	}
	return out
}

func IsPlanningIntentPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	if isOpenSpecPath(path) {
		return true
	}
	for _, prefix := range []string{"openspec/", "docs/adr/", "docs/adrs/", "docs/plans/", "docs/prd/", "docs/prds/", "docs/rfcs/", "docs/rfc/", "rfcs/", "rfc/", ".cursor/", ".claude/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, segment := range []string{"/docs/adr/", "/docs/adrs/", "/docs/plans/", "/docs/prd/", "/docs/prds/", "/docs/rfcs/", "/docs/rfc/", "/rfcs/", "/rfc/", "/docs/specs/", "/docs/design/", "/docs/technical/"} {
		if strings.Contains(path, segment) {
			return true
		}
	}
	return false
}

func IsSourceContextCandidate(c Candidate) bool {
	if isTestCaseCandidate(c) || isCodeCommentCandidate(c) {
		return false
	}
	return !strings.EqualFold(filepath.Ext(c.Path), ".md") && !IsPlanningIntentPath(c.Path)
}

func retrieveWeightedFilesV0(candidates []Candidate, query string, evidenceMode string, conceptBackfill, glossaryConcepts, tieredConceptOutput bool) []Candidate {
	terms := expandedTerms(query)
	queryLower := strings.ToLower(query)
	var scoredCandidates []scoredCandidate
	for _, c := range candidates {
		baseScore := scoreCandidate(c, terms, queryLower)
		if baseScore >= 4.0 {
			role := candidateRole(c)
			prior := authorityPrior(c, role, queryLower)
			scoredCandidates = append(scoredCandidates, scoredCandidate{
				candidate:  c,
				score:      baseScore + prior.score,
				baseScore:  baseScore,
				authority:  prior,
				profile:    candidateMatchProfile(c, queryLower),
				role:       role,
				sourceFile: IsSourceContextCandidate(c),
			})
		}
	}
	evidenceBalanced := strings.EqualFold(evidenceMode, EvidenceModeBalanced)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score == scoredCandidates[j].score {
			return scoredCandidates[i].candidate.Path < scoredCandidates[j].candidate.Path
		}
		return scoredCandidates[i].score > scoredCandidates[j].score
	})
	limit := retrievalLimit(queryLower, terms)
	scoredCandidates = collapseVariantCandidates(scoredCandidates, queryLower)
	scoredCandidates = suppressRawTestSourceCandidates(scoredCandidates, queryLower)
	scoredCandidates = selectScoredCandidates(scoredCandidates, queryLower, limit)
	scoredCandidates = enforceSupportingArtifactBudgets(scoredCandidates, queryLower, terms)
	if evidenceBalanced {
		scoredCandidates = applyBalancedEvidence(scoredCandidates, queryLower, terms)
		sort.Slice(scoredCandidates, func(i, j int) bool {
			if scoredCandidates[i].score == scoredCandidates[j].score {
				return scoredCandidates[i].candidate.Path < scoredCandidates[j].candidate.Path
			}
			return scoredCandidates[i].score > scoredCandidates[j].score
		})
	}
	out := make([]Candidate, 0, len(scoredCandidates))
	for _, sf := range scoredCandidates {
		out = append(out, sf.candidate)
	}
	out = expandOpenSpecLinks(out, candidates, queryLower, limit)
	if conceptBackfill {
		out = applyConceptBackfill(out, candidates, query, queryLower, terms, limit, glossaryConcepts, tieredConceptOutput)
	}
	out = packCandidateSections(out, queryLower, terms)
	if !evidenceBalanced {
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	}
	return out
}

type scoredCandidate struct {
	candidate  Candidate
	score      float64
	baseScore  float64
	authority  authorityPriorResult
	profile    matchProfile
	role       string
	sourceFile bool
}

// ConceptRank is an inspectable deterministic score for the experimental
// concept backfill lane. It is intentionally sparse: the retriever only uses it
// for a few high-confidence anchor recoveries after the primary ranked pack.
type ConceptRank struct {
	Candidate        Candidate `json:"-"`
	Path             string    `json:"path"`
	Score            float64   `json:"score"`
	MatchedCompacts  []string  `json:"matched_compacts,omitempty"`
	MatchedPhrases   []string  `json:"matched_phrases,omitempty"`
	MatchedPathTerms []string  `json:"matched_path_terms,omitempty"`
	GlossaryMatches  []string  `json:"glossary_matches,omitempty"`
	GlossaryEvidence []string  `json:"glossary_evidence,omitempty"`
}

type conceptQueryProfile struct {
	queryLower string
	compacts   []string
	phrases    []string
	words      []string
	testIntent bool
}

// RankConceptCandidates exposes the deterministic concept scorer for eval
// diagnostics. It should remain a backfill/inspection aid, not a replacement
// for the weighted retriever.
func RankConceptCandidates(candidates []Candidate, query string) []ConceptRank {
	profile := buildConceptQueryProfile(query)
	profile = filterNoisyConceptProfile(profile, candidates)
	if !conceptQueryIsUseful(profile) {
		return nil
	}
	return rankConceptCandidatesWithProfile(candidates, profile)
}

// RankConceptCandidatesWithGlossary applies the experimental local glossary
// gate before ranking. It is primarily used by eval diagnostics.
func RankConceptCandidatesWithGlossary(candidates []Candidate, query string) []ConceptRank {
	profile := buildConceptQueryProfile(query)
	profile = filterNoisyConceptProfile(profile, candidates)
	glossary := buildLocalGlossary(candidates)
	profile = applyGlossaryToConceptProfile(profile, glossary)
	if !conceptQueryIsUseful(profile) {
		return nil
	}
	ranks := rankConceptCandidatesWithProfile(candidates, profile)
	for i := range ranks {
		ranks[i] = annotateConceptRankWithGlossary(ranks[i], glossary)
	}
	return filterGlossaryRanks(ranks)
}

func rankConceptCandidatesWithProfile(candidates []Candidate, profile conceptQueryProfile) []ConceptRank {
	ranks := make([]ConceptRank, 0, len(candidates))
	for _, c := range candidates {
		rank := scoreConceptCandidate(c, profile)
		if rank.Score <= 0 {
			continue
		}
		ranks = append(ranks, rank)
	}
	sort.Slice(ranks, func(i, j int) bool {
		if ranks[i].Score == ranks[j].Score {
			return ranks[i].Path < ranks[j].Path
		}
		return ranks[i].Score > ranks[j].Score
	})
	return ranks
}

func applyConceptBackfill(selected []Candidate, universe []Candidate, query, queryLower string, terms map[string]float64, limit int, glossaryConcepts, tieredConceptOutput bool) []Candidate {
	_ = limit
	if len(universe) == 0 {
		return selected
	}
	profile := buildConceptQueryProfile(query)
	profile = filterNoisyConceptProfile(profile, universe)
	var glossary localGlossary
	if glossaryConcepts {
		glossary = buildLocalGlossary(universe)
		profile = applyGlossaryToConceptProfile(profile, glossary)
	}
	if !conceptQueryIsUseful(profile) {
		return selected
	}
	slots := conceptBackfillSlots(profile, queryLower, terms)
	if slots <= 0 {
		return selected
	}
	seen := map[string]bool{}
	for _, c := range selected {
		seen[candidateIdentity(c)] = true
		if c.Path != "" {
			seen[filepath.ToSlash(c.Path)] = true
		}
	}
	ranks := rankConceptCandidatesWithProfile(universe, profile)
	out := append([]Candidate(nil), selected...)
	for _, rank := range ranks {
		if len(out)-len(selected) >= slots {
			break
		}
		if seen[candidateIdentity(rank.Candidate)] || seen[filepath.ToSlash(rank.Path)] {
			continue
		}
		if !passesConceptBackfillThreshold(rank, profile, queryLower) {
			continue
		}
		if candidateIsNoisyConceptBackfill(rank.Candidate, profile, queryLower) {
			continue
		}
		if glossaryConcepts {
			rank = annotateConceptRankWithGlossary(rank, glossary)
			if len(rank.GlossaryMatches) == 0 {
				continue
			}
		}
		tier, tierReason := conceptBackfillTier(rank, profile, queryLower, tieredConceptOutput)
		c := withConceptBackfillMetadata(rank.Candidate, rank, tier, tierReason)
		seen[candidateIdentity(c)] = true
		seen[filepath.ToSlash(c.Path)] = true
		out = append(out, c)
	}
	return out
}

func conceptBackfillTier(rank ConceptRank, profile conceptQueryProfile, queryLower string, tieredConceptOutput bool) (string, string) {
	if !tieredConceptOutput {
		return PackTierPrimary, "tiering disabled"
	}
	role := candidateRole(rank.Candidate)
	if profile.testIntent && isExactConceptTestNameMatch(rank.Candidate, rank.MatchedCompacts) {
		return PackTierPrimary, "exact test-name concept anchor"
	}
	if isTestCaseCandidate(rank.Candidate) || IsSourceContextCandidate(rank.Candidate) {
		if profile.testIntent && len(rank.MatchedCompacts) > 0 && rank.Score >= 42 {
			return PackTierPrimary, "strong test/source identifier concept"
		}
		return PackTierRelated, "supporting test/source concept"
	}
	if len(rank.GlossaryMatches) > 0 && (len(rank.MatchedCompacts) > 0 || len(rank.MatchedPhrases) > 0 || len(rank.MatchedPathTerms) >= 2) {
		return PackTierPrimary, "glossary-supported concept"
	}
	if hasOpenSpecStructureIntent(queryLower) && strings.HasPrefix(role, "openspec_") {
		return PackTierPrimary, "requested OpenSpec concept"
	}
	if hasProductBackgroundIntent(queryLower) && role == "prd" {
		return PackTierPrimary, "requested product concept"
	}
	if hasRFCIntent(queryLower) && (role == "rfc" || role == "design") {
		return PackTierPrimary, "requested RFC/design concept"
	}
	if hasNonIntentModeIntent(queryLower, "protocol") && (role == "agent_instruction" || role == "skill" || role == "protocol") {
		return PackTierPrimary, "requested protocol concept"
	}
	if hasNonIntentModeIntent(queryLower, "template") && role == "template" {
		return PackTierPrimary, "requested template concept"
	}
	if len(rank.MatchedCompacts) > 0 && rank.Score >= 48 {
		return PackTierPrimary, "strong compact concept"
	}
	return PackTierRelated, "plausible concept backfill"
}

func isExactConceptTestNameMatch(c Candidate, matchedCompacts []string) bool {
	if !isTestCaseCandidate(c) || len(matchedCompacts) == 0 {
		return false
	}
	matched := stringSetFromSlice(matchedCompacts)
	for _, anchor := range testNameAnchors(c) {
		if anchor.compact != "" && matched[anchor.compact] {
			return true
		}
		if anchor.withoutTestPrefix != "" && matched[anchor.withoutTestPrefix] {
			return true
		}
	}
	return false
}

func buildConceptQueryProfile(query string) conceptQueryProfile {
	queryLower := strings.ToLower(query)
	words := conceptWords(query)
	profile := conceptQueryProfile{
		queryLower: queryLower,
		compacts:   conceptCompactsFromText(query),
		phrases:    conceptPhrases(words),
		words:      words,
		testIntent: hasTestBehaviorIntent(queryLower),
	}
	return profile
}

func conceptQueryIsUseful(profile conceptQueryProfile) bool {
	if len(profile.compacts) > 0 {
		return true
	}
	if len(profile.phrases) > 0 && len(profile.words) >= 2 {
		return true
	}
	return len(profile.words) >= 3
}

func filterNoisyConceptProfile(profile conceptQueryProfile, candidates []Candidate) conceptQueryProfile {
	if len(candidates) < 8 {
		return profile
	}
	compactDF := map[string]int{}
	wordPathDF := map[string]int{}
	for _, c := range candidates {
		pathText := c.Path + "\n" + c.Source + "\n" + c.Title + "\n" + conceptMetadataText(c) + "\n" + conceptSectionText(c)
		bodyText := c.Body
		if len(bodyText) > 4000 {
			bodyText = bodyText[:4000]
		}
		pathNorm := normalizeConceptText(pathText)
		bodyNorm := normalizeConceptText(bodyText)
		candidateCompacts := stringSetFromSlice(conceptCompactsFromText(pathText + "\n" + bodyText))
		for _, compact := range profile.compacts {
			for _, alt := range conceptCompactAlternates(compact) {
				if candidateCompacts[alt] || strings.Contains(pathNorm, alt) || strings.Contains(bodyNorm, alt) {
					compactDF[compact]++
					break
				}
			}
		}
		for _, word := range profile.words {
			if strings.Contains(pathNorm, word) {
				wordPathDF[word]++
			}
		}
	}
	noisyWord := map[string]bool{}
	var compacts []string
	for _, compact := range profile.compacts {
		if noisyConceptDF(compactDF[compact], len(candidates)) {
			for _, part := range splitIdentifierParts(compact) {
				noisyWord[part] = true
			}
			continue
		}
		compacts = append(compacts, compact)
	}
	var words []string
	for _, word := range profile.words {
		if noisyWord[word] || noisyConceptDF(wordPathDF[word], len(candidates)) {
			continue
		}
		words = append(words, word)
	}
	profile.compacts = compacts
	profile.words = words
	profile.phrases = conceptPhrases(words)
	return profile
}

func noisyConceptDF(df, total int) bool {
	if total <= 0 {
		return false
	}
	return df >= 8 && float64(df)/float64(total) >= 0.20
}

func scoreConceptCandidate(c Candidate, profile conceptQueryProfile) ConceptRank {
	pathText := c.Path + "\n" + c.Source + "\n" + c.Title + "\n" + conceptMetadataText(c) + "\n" + conceptSectionText(c)
	bodyText := c.Body
	if len(bodyText) > 12000 {
		bodyText = bodyText[:12000]
	}
	pathNorm := normalizeConceptText(pathText)
	bodyNorm := normalizeConceptText(bodyText)
	pathCompactSet := stringSetFromSlice(conceptCompactsFromText(pathText))
	bodyCompactSet := stringSetFromSlice(conceptCompactsFromText(bodyText))

	rank := ConceptRank{Candidate: c, Path: c.Path}
	role := candidateRole(c)
	pathOrTitleSignal := false
	bodyOnlyScore := 0.0
	for _, compact := range profile.compacts {
		for _, alt := range conceptCompactAlternates(compact) {
			switch {
			case pathCompactSet[alt] || strings.Contains(pathNorm, alt):
				rank.Score += 45.0
				rank.MatchedCompacts = append(rank.MatchedCompacts, compact)
				pathOrTitleSignal = true
			case bodyCompactSet[alt] || strings.Contains(bodyNorm, alt):
				add := 34.0
				if isTestCaseCandidate(c) || IsSourceContextCandidate(c) {
					add += 8.0
				}
				bodyOnlyScore += add
				rank.MatchedCompacts = append(rank.MatchedCompacts, compact)
			}
		}
	}
	if bodyOnlyScore > 55.0 {
		bodyOnlyScore = 55.0
	}
	rank.Score += bodyOnlyScore

	for _, phrase := range profile.phrases {
		phraseScore := 0.0
		if strings.Contains(pathNorm, phrase) {
			phraseScore += conceptPhraseScore(phrase, true)
			pathOrTitleSignal = true
		}
		if strings.Contains(bodyNorm, phrase) {
			phraseScore += conceptPhraseScore(phrase, false)
		}
		if phraseScore > 0 {
			rank.Score += phraseScore
			rank.MatchedPhrases = append(rank.MatchedPhrases, phrase)
		}
	}

	pathTermMatches := 0
	for _, word := range profile.words {
		if strings.Contains(pathNorm, word) {
			pathTermMatches++
			rank.MatchedPathTerms = append(rank.MatchedPathTerms, word)
			pathOrTitleSignal = true
			continue
		}
		if strings.Contains(bodyNorm, word) {
			rank.Score += 0.7
		}
	}
	if pathTermMatches > 0 {
		termScore := float64(pathTermMatches) * 5.0
		if termScore > 24.0 {
			termScore = 24.0
		}
		rank.Score += termScore
	}

	switch {
	case profile.testIntent && (isTestCaseCandidate(c) || IsSourceContextCandidate(c)):
		rank.Score += 10.0
	case hasProductBackgroundIntent(profile.queryLower) && role == "prd":
		rank.Score += 7.0
	case hasRFCIntent(profile.queryLower) && (role == "rfc" || role == "design"):
		rank.Score += 7.0
	case hasOpenSpecStructureIntent(profile.queryLower) && strings.HasPrefix(role, "openspec_"):
		rank.Score += 7.0
	case hasNonIntentModeIntent(profile.queryLower, "protocol") && (role == "agent_instruction" || role == "skill" || role == "protocol"):
		rank.Score += 7.0
	case hasNonIntentModeIntent(profile.queryLower, "template") && role == "template":
		rank.Score += 7.0
	}
	if mode := nonIntentCandidateMode(c); mode != "" && !hasNonIntentModeIntent(profile.queryLower, mode) {
		rank.Score -= 15.0
	}
	if !pathOrTitleSignal && len(rank.MatchedCompacts) == 0 {
		rank.Score -= 12.0
	}
	rank.MatchedCompacts = uniqueStrings(rank.MatchedCompacts)
	rank.MatchedPhrases = uniqueStrings(rank.MatchedPhrases)
	rank.MatchedPathTerms = uniqueStrings(rank.MatchedPathTerms)
	return rank
}

func conceptBackfillSlots(profile conceptQueryProfile, queryLower string, terms map[string]float64) int {
	switch {
	case hasIdentifierTerm(terms) || len(profile.compacts) > 0:
		return 2
	case profile.testIntent:
		return 1
	case hasOpenSpecStructureIntent(queryLower) || hasProductBackgroundIntent(queryLower) || hasRFCIntent(queryLower):
		return 2
	case hasNonIntentModeIntent(queryLower, "protocol") || hasNonIntentModeIntent(queryLower, "template") || hasNonIntentModeIntent(queryLower, "model"):
		return 2
	default:
		return 1
	}
}

func passesConceptBackfillThreshold(rank ConceptRank, profile conceptQueryProfile, queryLower string) bool {
	threshold := 38.0
	if profile.testIntent || len(rank.MatchedCompacts) > 0 {
		threshold = 32.0
	}
	if len(rank.MatchedPhrases) > 0 && len(rank.MatchedPathTerms) >= 2 {
		threshold = 27.0
	}
	if conceptQueryIsBroad(profile, queryLower) {
		threshold += 10.0
	}
	return rank.Score >= threshold
}

func candidateIsNoisyConceptBackfill(c Candidate, profile conceptQueryProfile, queryLower string) bool {
	role := candidateRole(c)
	if role == "model" && !hasNonIntentModeIntent(queryLower, "model") {
		return true
	}
	if (role == "template" || strings.Contains(strings.ToLower(c.Path), "template")) && !hasNonIntentModeIntent(queryLower, "template") {
		return true
	}
	if (role == "agent_instruction" || role == "skill" || role == "protocol") && !hasNonIntentModeIntent(queryLower, "protocol") {
		return true
	}
	if !profile.testIntent && (isTestCaseCandidate(c) || IsSourceContextCandidate(c)) && !hasExplicitSourceIntent(queryLower) && !hasCodeCommentIntent(queryLower) {
		return true
	}
	if len(profile.words) < 2 && len(profile.compacts) == 0 {
		return true
	}
	return false
}

func withConceptBackfillMetadata(c Candidate, rank ConceptRank, tier, tierReason string) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["retrieval_expansion_reason"] = "concept_backfill"
	c.Metadata["concept_backfill_score"] = fmt.Sprintf("%.1f", rank.Score)
	c.Metadata["concept_backfill_matched_compacts_json"] = jsonStringList(rank.MatchedCompacts)
	c.Metadata["concept_backfill_matched_phrases_json"] = jsonStringList(rank.MatchedPhrases)
	c.Metadata["concept_backfill_matched_path_terms_json"] = jsonStringList(rank.MatchedPathTerms)
	if tier != "" {
		c.Metadata["pack_tier"] = tier
		c.Metadata["pack_tier_source"] = "concept_backfill"
	}
	if tierReason != "" {
		c.Metadata["pack_tier_reason"] = tierReason
	}
	if len(rank.GlossaryMatches) > 0 {
		c.Metadata["concept_glossary_enabled"] = "true"
		c.Metadata["concept_glossary_matched_json"] = jsonStringList(rank.GlossaryMatches)
		c.Metadata["concept_glossary_evidence_json"] = jsonStringList(rank.GlossaryEvidence)
	}
	return c
}

func conceptWords(text string) []string {
	stop := map[string]bool{}
	for _, term := range []string{
		"a", "about", "agent", "all", "an", "and", "architecture", "artifact",
		"behavior", "behaviour", "case", "cases", "code", "context", "cover",
		"covers", "decision", "design", "doc", "docs", "document", "documents",
		"find", "for", "how", "implementation", "instructions", "plan", "plans",
		"product", "proposal", "requirement", "requirements", "source", "spec",
		"template", "test", "tests", "the", "to", "what", "when", "where",
		"which", "with",
	} {
		stop[term] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range splitIdentifierParts(text) {
		part = strings.ToLower(strings.TrimSpace(part))
		if len(part) < 3 || stop[part] || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return out
}

func conceptCompactsFromText(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, token := range conceptRawTokens(text) {
		parts := splitIdentifierParts(token)
		if len(parts) == 0 {
			continue
		}
		compact := strings.Join(parts, "")
		if len(compact) < 8 || seen[compact] || conceptCompactIsGeneric(compact) {
			continue
		}
		if !conceptTokenHasIdentifierShape(token, parts) {
			continue
		}
		seen[compact] = true
		out = append(out, compact)
	}
	return out
}

func conceptRawTokens(text string) []string {
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func conceptTokenHasIdentifierShape(token string, parts []string) bool {
	if looksLikeCompactTestIdentifier(strings.ToLower(token)) {
		return true
	}
	if strings.ContainsAny(token, "_.-") || len(parts) >= 3 {
		return true
	}
	for _, r := range token {
		if unicode.IsUpper(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func conceptCompactIsGeneric(compact string) bool {
	switch compact {
	case "architecture", "architecturedecision", "architecturedecisionrecord",
		"documentation", "implementation", "productrequirements",
		"productrequirementsdocument", "requirements", "repositoryinstructions":
		return true
	default:
		return false
	}
}

func conceptCompactAlternates(compact string) []string {
	alts := []string{compact}
	if strings.HasPrefix(compact, "test") && len(compact) > 8 {
		alts = append(alts, strings.TrimPrefix(compact, "test"))
	}
	return uniqueStrings(alts)
}

func conceptPhrases(words []string) []string {
	if len(words) < 2 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for n := 3; n >= 2; n-- {
		if len(words) < n {
			continue
		}
		for i := 0; i+n <= len(words); i++ {
			phrase := strings.Join(words[i:i+n], " ")
			if seen[phrase] {
				continue
			}
			seen[phrase] = true
			out = append(out, phrase)
		}
	}
	return out
}

func conceptPhraseScore(phrase string, pathSignal bool) float64 {
	parts := strings.Count(phrase, " ") + 1
	if pathSignal {
		if parts >= 3 {
			return 18.0
		}
		return 12.0
	}
	if parts >= 3 {
		return 7.0
	}
	return 4.0
}

func normalizeConceptText(text string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func conceptMetadataText(c Candidate) string {
	if c.Metadata == nil {
		return ""
	}
	keys := []string{
		"classifier_kind", "classifier_model", "classifier_subtype",
		"openspec_role", "parent_title", "source_type", "test_name",
	}
	var parts []string
	for _, key := range keys {
		if value := strings.TrimSpace(c.Metadata[key]); value != "" {
			parts = append(parts, value)
		}
	}
	for _, key := range []string{"symbols_json", "assertion_terms_json", "indexed_section_match_headings_json"} {
		parts = append(parts, metadataJSONList(c, key)...)
	}
	return strings.Join(parts, "\n")
}

func conceptSectionText(c Candidate) string {
	var parts []string
	for _, section := range c.Sections {
		if section.HeadingPath != "" {
			parts = append(parts, section.HeadingPath)
		}
		if section.Title != "" {
			parts = append(parts, section.Title)
		}
		if section.Metadata != nil {
			for _, key := range []string{"role", "kind", "subtype"} {
				if value := strings.TrimSpace(section.Metadata[key]); value != "" {
					parts = append(parts, value)
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}

func conceptQueryIsBroad(profile conceptQueryProfile, queryLower string) bool {
	if len(profile.compacts) > 0 {
		return false
	}
	if len(profile.words) <= 1 {
		return true
	}
	broadSignals := 0
	for _, term := range []string{"architecture", "instructions", "requirements", "template", "video"} {
		if strings.Contains(queryLower, term) {
			broadSignals++
		}
	}
	return broadSignals >= 2 && len(profile.words) < 3
}

func stringSetFromSlice(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

func jsonStringList(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

type balancedEvidenceStats struct {
	candidateCount   int
	termDF           map[string]int
	siblingCounts    map[string]int
	siblingBestScore map[string]float64
}

type balancedEvidenceResult struct {
	score       float64
	anchorScore float64
	reasons     []string
}

func applyBalancedEvidence(candidates []scoredCandidate, queryLower string, terms map[string]float64) []scoredCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	stats := buildBalancedEvidenceStats(candidates, queryLower)
	results := make([]balancedEvidenceResult, len(candidates))
	for i, candidate := range candidates {
		result := balancedEvidenceForCandidate(candidate, stats, queryLower, terms)
		results[i] = result
		if key := balancedSiblingKey(candidate.candidate); key != "" && result.anchorScore > stats.siblingBestScore[key] {
			stats.siblingBestScore[key] = result.anchorScore
		}
	}
	for i, candidate := range candidates {
		result := results[i]
		key := balancedSiblingKey(candidate.candidate)
		if key != "" && stats.siblingCounts[key] >= 2 {
			best := stats.siblingBestScore[key]
			switch {
			case result.anchorScore > 0 && result.anchorScore == best:
				result.score += 0.9
				result.reasons = append(result.reasons, "sibling contrast winner")
			case result.anchorScore == 0 && best >= 3.0 && isWeakBodyOnlyBackfill(candidate, queryLower):
				result.score -= 2.0
				result.reasons = append(result.reasons, "sibling contrast body-only dampening")
			case result.anchorScore > 0 && best >= 3.0 && result.anchorScore < best*0.55:
				result.score -= 0.8
				result.reasons = append(result.reasons, "sibling contrast weaker sibling")
			}
		}
		if result.score != 0 {
			candidate.score += result.score
			candidate.candidate = withBalancedEvidenceMetadata(candidate.candidate, result.score, result.reasons)
		}
		candidates[i] = candidate
	}
	return candidates
}

func buildBalancedEvidenceStats(candidates []scoredCandidate, queryLower string) balancedEvidenceStats {
	stats := balancedEvidenceStats{
		candidateCount:   len(candidates),
		termDF:           map[string]int{},
		siblingCounts:    map[string]int{},
		siblingBestScore: map[string]float64{},
	}
	terms := balancedEvidenceTerms(queryLower)
	for _, candidate := range candidates {
		text := balancedStructuredText(candidate.candidate)
		seen := map[string]bool{}
		for _, term := range terms {
			if term == "" || seen[term] {
				continue
			}
			if strings.Contains(text, term) {
				stats.termDF[term]++
				seen[term] = true
			}
		}
		if key := balancedSiblingKey(candidate.candidate); key != "" {
			stats.siblingCounts[key]++
		}
	}
	return stats
}

func balancedEvidenceForCandidate(candidate scoredCandidate, stats balancedEvidenceStats, queryLower string, terms map[string]float64) balancedEvidenceResult {
	c := candidate.candidate
	pathTitleLower := strings.ToLower(c.Path + "\n" + c.Title)
	result := balancedEvidenceResult{}
	add := func(delta float64, anchor bool, reason string) {
		if delta == 0 {
			return
		}
		result.score += delta
		if anchor {
			result.anchorScore += delta
		}
		if reason != "" {
			result.reasons = append(result.reasons, reason)
		}
	}

	pathTitleScore := 0.0
	for _, term := range balancedEvidenceTerms(queryLower) {
		if term == "" || !strings.Contains(pathTitleLower, term) {
			continue
		}
		pathTitleScore += balancedLocalIDF(stats, term)
	}
	if pathTitleScore > 0 {
		add(clampFloat(pathTitleScore*0.9, 0, 4.5), true, "rare path/title match")
	}

	roleScore := balancedRoleEvidenceScore(candidate.role, queryLower)
	if roleScore > 0 {
		add(roleScore, true, "query/artifact role alignment")
	}

	sectionScore := indexedSectionScore(c, terms, queryLower)
	if sectionScore > 0 {
		add(clampFloat(sectionScore*0.25, 0, 2.0), true, "indexed section corroboration")
	}

	if candidate.profile.identifierMatches > 0 && result.anchorScore > 0 {
		add(clampFloat(float64(candidate.profile.identifierMatches)*0.8, 0, 2.0), true, "anchored identifier match")
	}
	if candidate.role == "test_case" {
		if anchor := testNameAnchorScore(c, queryLower); anchor.score > 0 {
			add(clampFloat(anchor.score*0.35, 0, 2.5), true, anchor.reason)
		}
	}

	if candidate.profile.coreTerms >= 3 && result.anchorScore == 0 && !candidate.sourceFile {
		add(-2.5, false, "body-only evidence dampening")
	}
	if mode := nonIntentCandidateMode(c); mode != "" && !hasNonIntentModeIntent(queryLower, mode) {
		add(-2.0, false, "unrequested non-intent lane")
	}
	if result.anchorScore > 0 {
		pathLower := strings.ToLower(filepath.ToSlash(c.Path))
		bodyLower := strings.ToLower(c.Body)
		switch {
		case candidateIsStale(c, pathLower, bodyLower) && !hasLifecycleIntent(queryLower):
			add(-0.7, false, "stale lifecycle cue")
		case hasArchivePath(pathLower) && !containsAny(queryLower, "archive", "archived", "historical", "history"):
			add(-0.5, false, "archive lifecycle cue")
		case metadataLower(c, "classifier_authority") != "":
			add(0.3, false, "classifier authority cue")
		}
	}

	result.score = clampFloat(result.score, -6.0, 8.0)
	result.reasons = uniqueStrings(result.reasons)
	if len(result.reasons) > 4 {
		result.reasons = result.reasons[:4]
	}
	return result
}

func balancedRoleEvidenceScore(role, queryLower string) float64 {
	switch role {
	case "adr":
		if containsAny(queryLower, "adr", "decision", "why", "rationale", "architecture") {
			return 1.6
		}
	case "prd":
		if hasProductBackgroundIntent(queryLower) {
			return 1.6
		}
	case "rfc":
		if hasRFCIntent(queryLower) {
			return 1.6
		}
	case "design":
		if containsAny(queryLower, "design", "architecture", "technical") {
			return 1.4
		}
	case "plan", "agent_note":
		if hasPlanIntent(queryLower) {
			return 1.2
		}
	case "openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		if hasOpenSpecStructureIntent(queryLower) || hasOpenSpecChildRoleIntent(queryLower) {
			return 1.4
		}
	case "agent_instruction", "protocol", "skill":
		if hasNonIntentModeIntent(queryLower, "protocol") {
			return 1.2
		}
	case "template":
		if hasNonIntentModeIntent(queryLower, "template") {
			return 1.2
		}
	case "model":
		if hasNonIntentModeIntent(queryLower, "model") {
			return 1.2
		}
	case "test_case":
		if hasTestBehaviorIntent(queryLower) {
			return 1.4
		}
	case "code_comment":
		if hasCodeCommentIntent(queryLower) {
			return 1.2
		}
	}
	return 0
}

func balancedEvidenceTerms(queryLower string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, term := range coreQueryTerms(queryLower) {
		add(term)
	}
	for _, term := range identifierQueryTerms(queryLower) {
		add(term)
		for _, part := range splitIdentifier(term) {
			if len(part) >= 4 {
				add(part)
			}
		}
	}
	if len(out) == 0 {
		for _, term := range meaningfulTerms(queryLower) {
			if !genericSectionTerm(term) {
				add(term)
			}
		}
	}
	return out
}

func balancedStructuredText(c Candidate) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(filepath.ToSlash(c.Path)))
	b.WriteByte('\n')
	b.WriteString(strings.ToLower(c.Title))
	if c.Metadata != nil {
		for _, key := range []string{
			"indexed_section_match_headings_json",
			"section_pack_headings",
			"openspec_role",
			"source_standard",
			"layout_group",
			"test_name",
			"parent_title",
		} {
			if value := strings.TrimSpace(c.Metadata[key]); value != "" {
				b.WriteByte('\n')
				b.WriteString(strings.ToLower(value))
			}
		}
	}
	return b.String()
}

func balancedLocalIDF(stats balancedEvidenceStats, term string) float64 {
	if stats.candidateCount <= 0 {
		return 1
	}
	df := stats.termDF[term]
	if df <= 0 {
		df = 1
	}
	return clampFloat(1.0+math.Log(float64(stats.candidateCount+1)/float64(df+1)), 0.7, 3.0)
}

func balancedSiblingKey(c Candidate) string {
	path := strings.ToLower(filepath.ToSlash(c.Path))
	if idx := strings.Index(path, "#"); idx >= 0 {
		path = path[:idx]
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	if isOpenSpecChangePath(path) {
		parts := strings.Split(path, "/")
		for i := 0; i+2 < len(parts); i++ {
			if parts[i] == "changes" {
				return "openspec_change|" + strings.Join(parts[:i+2], "/")
			}
		}
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	if dir == "." || dir == "" {
		return ""
	}
	role := candidateRole(c)
	if role == "" {
		role = strings.ToLower(filepath.Ext(path))
	}
	return role + "|" + dir
}

func withBalancedEvidenceMetadata(c Candidate, score float64, reasons []string) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["retrieval_evidence_mode"] = EvidenceModeBalanced
	c.Metadata["retrieval_evidence_score"] = fmt.Sprintf("%.3f", score)
	if len(reasons) > 0 {
		c.Metadata["retrieval_evidence_reasons"] = strings.Join(reasons, "\n")
	}
	return c
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

type authorityPriorResult struct {
	score   float64
	reasons []string
}

// AuthorityCues returns short trust/caution labels that are safe to show in
// human or JSON output. It intentionally exposes less detail than retrieval
// reasons; the detailed factors remain available through ExplainCandidates.
func AuthorityCues(c Candidate) []string {
	var cues []string
	switch metadataLower(c, "classifier_authority") {
	case "high_current_intent":
		cues = append(cues, "current intent")
	case "high_decision":
		cues = append(cues, "decision authority")
	case "design_proposal":
		cues = append(cues, "design/proposal")
	case "product_background":
		cues = append(cues, "product background")
	case "working_plan":
		cues = append(cues, "working plan")
	case "handoff_note":
		cues = append(cues, "handoff note")
	}

	status := strings.ToLower(strings.TrimSpace(c.Status))
	if status == "" {
		status = metadataLower(c, "classifier_status")
	}
	lifecycle := metadataLower(c, "classifier_lifecycle")
	switch {
	case status == "accepted":
		cues = append(cues, "accepted")
	case status == "superseded" || lifecycle == "superseded":
		cues = append(cues, "superseded")
	case status == "stale" || lifecycle == "stale":
		cues = append(cues, "stale")
	case status == "deprecated" || lifecycle == "deprecated" || lifecycle == "obsolete":
		cues = append(cues, "deprecated")
	case lifecycle == "archived":
		cues = append(cues, "archived")
	}

	switch metadataLower(c, "artifact_scope") {
	case "bundle":
		cues = append(cues, "bundle")
	case "collection":
		cues = append(cues, "collection")
	}
	if isTestCaseCandidate(c) {
		cues = append(cues, "test behavior")
	}
	if isCodeCommentCandidate(c) {
		cues = append(cues, "code rationale")
	}
	if count := metadataLower(c, "variant_collapsed_count"); count != "" && count != "0" {
		cues = append(cues, "variants collapsed")
	}
	if len(cues) > 2 {
		cues = cues[:2]
	}
	return uniqueStrings(cues)
}

func authorityPrior(c Candidate, role, queryLower string) authorityPriorResult {
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	status := strings.ToLower(strings.TrimSpace(c.Status))
	if status == "" {
		status = metadataLower(c, "classifier_status")
	}
	lifecycle := metadataLower(c, "classifier_lifecycle")
	var result authorityPriorResult
	add := func(delta float64, reason string) {
		if delta == 0 || reason == "" {
			return
		}
		result.score += delta
		result.reasons = append(result.reasons, reason)
	}

	switch role {
	case "adr":
		if hasPathSegment(pathLower, "adr") || hasPathSegment(pathLower, "adrs") || strings.Contains(pathLower, "/architecture/decisions/") {
			add(1.2, "authority prior: canonical ADR path")
		}
		if status == "accepted" || strings.Contains(strings.ToLower(c.Body), "status: accepted") {
			add(0.5, "authority prior: accepted decision")
		}
	case "prd":
		if strings.Contains(pathLower, "/product-specs/") ||
			hasPathSegment(pathLower, "prd") ||
			hasPathSegment(pathLower, "prds") ||
			strings.Contains(pathLower, "product-requirement") ||
			strings.Contains(pathLower, "product_requirement") {
			add(1.2, "authority prior: canonical product requirements path")
		}
	case "rfc":
		if hasPathSegment(pathLower, "rfc") ||
			hasPathSegment(pathLower, "rfcs") ||
			hasPathSegment(pathLower, "proposals") ||
			hasPathSegment(pathLower, "enhancements") {
			add(0.9, "authority prior: canonical RFC/proposal path")
		}
	case "design":
		if hasPathSegment(pathLower, "architecture") ||
			hasPathSegment(pathLower, "design") ||
			hasPathSegment(pathLower, "design-docs") ||
			strings.Contains(pathLower, "/docs/design") {
			add(0.8, "authority prior: canonical design path")
		}
	case "plan":
		if hasPathSegment(pathLower, "plans") ||
			hasPathSegment(pathLower, "roadmaps") ||
			strings.HasSuffix(pathLower, "roadmap.md") ||
			strings.HasSuffix(pathLower, "plan.md") {
			add(0.7, "authority prior: canonical plan path")
		}
		if status == "active" || status == "in_progress" || status == "draft" {
			add(0.3, "authority prior: active planning status")
		}
	case "openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		if isOpenSpecPath(pathLower) {
			add(1.0, "authority prior: OpenSpec structure")
		}
		if c.Metadata != nil && c.Metadata["artifact_scope"] != "" {
			add(0.3, "authority prior: structured artifact scope")
		}
	case "agent_instruction", "protocol", "skill":
		if hasNonIntentModeIntent(queryLower, "protocol") {
			if pathDepth(pathLower) == 0 {
				add(0.8, "authority prior: repository-level protocol")
			} else if queryMentionsInstructionPathSubject(queryLower, pathLower) {
				add(0.4, "authority prior: named module protocol")
			}
		}
	case "template":
		if hasNonIntentModeIntent(queryLower, "template") {
			add(0.7, "authority prior: requested template artifact")
		}
	case "model":
		if hasNonIntentModeIntent(queryLower, "model") {
			add(0.7, "authority prior: requested model artifact")
		}
	case "test_case":
		if hasTestBehaviorIntent(queryLower) {
			add(0.6, "authority prior: behavioral test signal")
		}
	case "code_comment":
		if hasCodeCommentIntent(queryLower) {
			add(0.45, "authority prior: implementation rationale signal")
		}
	}

	if conf := classifierConfidence(c); conf >= 0.75 {
		add(0.7, "authority prior: high classifier confidence")
	} else if conf >= 0.55 {
		add(0.35, "authority prior: classifier confidence")
	}
	switch metadataLower(c, "classifier_authority") {
	case "high_current_intent":
		add(0.45, "authority prior: classifier high-current intent")
	case "high_decision":
		add(0.45, "authority prior: classifier high-decision authority")
	case "design_proposal":
		add(0.3, "authority prior: classifier design/proposal authority")
	case "product_background":
		add(0.25, "authority prior: classifier product background authority")
	case "working_plan":
		add(0.25, "authority prior: classifier working-plan authority")
	case "handoff_note":
		if role == "agent_note" || containsAny(queryLower, "agent", "resume", "continue", "handoff", "followup", "follow-up") {
			add(0.2, "authority prior: classifier handoff-note authority")
		}
	}
	if role != "" && role != "agent_note" {
		switch role {
		case "adr", "prd", "rfc", "design", "plan",
			"openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
			add(0.4, "authority prior: recognized artifact role "+role)
		}
	}

	if status == "superseded" || status == "stale" || status == "deprecated" ||
		lifecycle == "superseded" || lifecycle == "stale" || lifecycle == "deprecated" || lifecycle == "obsolete" {
		if !hasLifecycleIntent(queryLower) {
			add(-0.8, "authority prior: stale or superseded")
		}
	}
	if lifecycle == "archived" && !containsAny(queryLower, "archive", "archived", "historical", "history") {
		add(-0.5, "authority prior: archived lifecycle")
	}
	if hasArchivePath(pathLower) && !containsAny(queryLower, "archive", "archived", "historical", "history") {
		add(-0.7, "authority prior: archive path")
	}
	if hasGeneratedPath(pathLower) && !containsAny(queryLower, "generated", "reference") {
		add(-0.6, "authority prior: generated/reference path")
	}
	if hasTemplatePath(pathLower) && !hasNonIntentModeIntent(queryLower, "template") && !strings.Contains(queryLower, "template") {
		add(-0.6, "authority prior: template path")
	}
	if hasExamplePath(pathLower) && !containsAny(queryLower, "example", "sample", "demo") {
		add(-0.5, "authority prior: example path")
	}
	if isLikelyUnrequestedLocalizedPath(pathLower, queryLower) {
		add(-0.5, "authority prior: unrequested language mirror")
	}
	if strings.Contains(filepath.Base(pathLower), "readme") && !strings.Contains(queryLower, "readme") {
		add(-0.25, "authority prior: broad README surface")
	}

	result.reasons = uniqueStrings(result.reasons)
	return result
}

func classifierConfidence(c Candidate) float64 {
	value := metadataLower(c, "classifier_confidence")
	if value == "" {
		return 0
	}
	confidence, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return confidence
}

func metadataLower(c Candidate, key string) string {
	if c.Metadata == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(c.Metadata[key]))
}

func metadataJSONList(c Candidate, key string) []string {
	if c.Metadata == nil || strings.TrimSpace(c.Metadata[key]) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(c.Metadata[key]), &out); err != nil {
		return nil
	}
	return out
}

func hasPathSegment(path, segment string) bool {
	path = strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/")
	segment = strings.ToLower(strings.Trim(segment, "/"))
	if path == "" || segment == "" {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == segment {
			return true
		}
	}
	return false
}

func hasArchivePath(pathLower string) bool {
	return hasPathSegment(pathLower, "archive") ||
		hasPathSegment(pathLower, "archives") ||
		hasPathSegment(pathLower, "archived")
}

func hasGeneratedPath(pathLower string) bool {
	return hasPathSegment(pathLower, "generated") ||
		hasPathSegment(pathLower, "gen") ||
		hasPathSegment(pathLower, "references") ||
		hasPathSegment(pathLower, "reference") ||
		strings.Contains(pathLower, "documentation-full")
}

func hasTemplatePath(pathLower string) bool {
	return hasPathSegment(pathLower, "template") ||
		hasPathSegment(pathLower, "templates") ||
		strings.Contains(pathLower, "template")
}

func hasExamplePath(pathLower string) bool {
	return hasPathSegment(pathLower, "example") ||
		hasPathSegment(pathLower, "examples") ||
		hasPathSegment(pathLower, "sample") ||
		hasPathSegment(pathLower, "samples") ||
		hasPathSegment(pathLower, "demo") ||
		hasPathSegment(pathLower, "demos")
}

type groupedVariantCandidate struct {
	candidate scoredCandidate
	variant   variantInfo
}

func collapseVariantCandidates(candidates []scoredCandidate, queryLower string) []scoredCandidate {
	if len(candidates) <= 1 {
		return candidates
	}
	groups := map[string][]groupedVariantCandidate{}
	for _, candidate := range candidates {
		variant := variantFingerprint(candidate.candidate, queryLower)
		if variant.key == "" {
			continue
		}
		groups[variant.key] = append(groups[variant.key], groupedVariantCandidate{candidate: candidate, variant: variant})
	}
	collapsedPaths := map[string]bool{}
	keepers := map[string]scoredCandidate{}
	for key, group := range groups {
		if len(group) < 2 || !groupHasVariant(group) {
			continue
		}
		keepIndex := 0
		keepScore := variantSelectionScore(group[0].candidate, group[0].variant, queryLower)
		for i := 1; i < len(group); i++ {
			score := variantSelectionScore(group[i].candidate, group[i].variant, queryLower)
			if score > keepScore || (score == keepScore && group[i].candidate.candidate.Path < group[keepIndex].candidate.candidate.Path) {
				keepIndex = i
				keepScore = score
			}
		}
		collapsed := make([]string, 0, len(group)-1)
		reasons := make([]string, 0, len(group)-1)
		for i, item := range group {
			path := item.candidate.candidate.Path
			if i == keepIndex {
				continue
			}
			if preserveVariantCandidate(item.candidate, item.variant, queryLower) {
				continue
			}
			collapsedPaths[path] = true
			collapsed = append(collapsed, path)
			reasons = append(reasons, item.variant.reason)
		}
		if len(collapsed) == 0 {
			continue
		}
		keeper := group[keepIndex].candidate
		keeper.candidate = withVariantMetadata(keeper.candidate, key, group[keepIndex].variant, collapsed, reasons)
		keepers[keeper.candidate.Path] = keeper
	}
	if len(collapsedPaths) == 0 {
		return candidates
	}
	out := make([]scoredCandidate, 0, len(candidates)-len(collapsedPaths))
	for _, candidate := range candidates {
		if collapsedPaths[candidate.candidate.Path] {
			continue
		}
		if keeper, ok := keepers[candidate.candidate.Path]; ok {
			candidate = keeper
		}
		out = append(out, candidate)
	}
	return out
}

type variantInfo struct {
	key    string
	role   string
	reason string
	exact  bool
}

func variantFingerprint(c Candidate, queryLower string) variantInfo {
	path := strings.Trim(strings.ToLower(filepath.ToSlash(c.Path)), "/")
	if path == "" {
		return variantInfo{}
	}
	segments := strings.Split(path, "/")
	normalized := make([]string, 0, len(segments))
	roles := map[string]bool{}
	for _, segment := range segments {
		switch {
		case isLocaleSegment(segment):
			roles["translation"] = true
			continue
		case isArchiveSegment(segment):
			roles["archive"] = true
			continue
		case isGeneratedSegment(segment):
			roles["generated"] = true
			continue
		case isTemplateSegment(segment):
			roles["template"] = true
			continue
		case isExampleSegment(segment):
			roles["example"] = true
			continue
		default:
			normalized = append(normalized, segment)
		}
	}
	role := variantRoleFromMarkers(roles)
	if role == "" && isAgentInstructionPath(path) && hasRepositoryInstructionIntent(queryLower) {
		role = "nested-module"
		normalized = []string{filepath.Base(path)}
		if pathDepth(path) == 0 {
			role = "canonical"
		}
	}
	if role == "" {
		return variantInfo{key: variantBaseKey(c, path), role: "canonical", exact: variantHasExactQueryAnchor(c, queryLower)}
	}
	if len(normalized) == 0 {
		normalized = []string{filepath.Base(path)}
	}
	return variantInfo{
		key:    variantBaseKey(c, strings.Join(normalized, "/")),
		role:   role,
		reason: variantReason(role),
		exact:  variantHasExactQueryAnchor(c, queryLower),
	}
}

func variantBaseKey(c Candidate, normalizedPath string) string {
	role := candidateRole(c)
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(c.Kind + ":" + c.Subtype))
	}
	if role == "" {
		role = "unknown"
	}
	return role + "|" + normalizedPath
}

func variantRoleFromMarkers(roles map[string]bool) string {
	for _, role := range []string{"archive", "generated", "template", "example", "translation"} {
		if roles[role] {
			return role
		}
	}
	return ""
}

func groupHasVariant(group []groupedVariantCandidate) bool {
	for _, item := range group {
		if item.variant.role != "canonical" {
			return true
		}
	}
	return false
}

func variantSelectionScore(candidate scoredCandidate, variant variantInfo, queryLower string) float64 {
	score := candidate.score
	if variant.exact {
		score += 100
	}
	switch variant.role {
	case "canonical":
		score += 8
	case "nested-module":
		if !queryMentionsInstructionPathSubject(queryLower, strings.ToLower(filepath.ToSlash(candidate.candidate.Path))) {
			score -= 4
		}
	case "translation":
		if isLikelyUnrequestedLocalizedPath(strings.ToLower(filepath.ToSlash(candidate.candidate.Path)), queryLower) {
			score -= 3
		}
	case "archive":
		if !hasLifecycleIntent(queryLower) && !containsAny(queryLower, "archive", "archived", "historical", "history") {
			score -= 5
		}
	case "template":
		if !hasNonIntentModeIntent(queryLower, "template") && !strings.Contains(queryLower, "template") {
			score -= 4
		}
	case "example":
		if !containsAny(queryLower, "example", "sample", "demo") {
			score -= 3
		}
	case "generated":
		if !containsAny(queryLower, "generated", "reference") {
			score -= 3
		}
	}
	return score
}

func preserveVariantCandidate(candidate scoredCandidate, variant variantInfo, queryLower string) bool {
	if variant.exact {
		return true
	}
	pathLower := strings.ToLower(filepath.ToSlash(candidate.candidate.Path))
	switch variant.role {
	case "canonical":
		return true
	case "archive":
		return hasLifecycleIntent(queryLower) || containsAny(queryLower, "archive", "archived", "historical", "history")
	case "template":
		return hasNonIntentModeIntent(queryLower, "template") || strings.Contains(queryLower, "template")
	case "example":
		return containsAny(queryLower, "example", "sample", "demo")
	case "generated":
		return containsAny(queryLower, "generated", "reference")
	case "translation":
		return !isLikelyUnrequestedLocalizedPath(pathLower, queryLower)
	case "nested-module":
		return queryMentionsInstructionPathSubject(queryLower, pathLower)
	default:
		return false
	}
}

func withVariantMetadata(c Candidate, groupID string, kept variantInfo, collapsed, reasons []string) Candidate {
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["variant_group_id"] = groupID
	c.Metadata["variant_role"] = kept.role
	c.Metadata["variant_collapsed_count"] = strconv.Itoa(len(collapsed))
	c.Metadata["variant_collapsed_paths"] = strings.Join(uniqueStrings(collapsed), "\n")
	c.Metadata["variant_reason"] = strings.Join(uniqueStrings(reasons), "; ")
	return c
}

func variantReason(role string) string {
	switch role {
	case "archive":
		return "archive/current variant"
	case "generated":
		return "generated/reference variant"
	case "template":
		return "template/instance variant"
	case "example":
		return "example/sample variant"
	case "translation":
		return "localized mirror"
	case "nested-module":
		return "nested instruction variant"
	default:
		return "variant"
	}
}

func variantHasExactQueryAnchor(c Candidate, queryLower string) bool {
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	titleLower := strings.ToLower(c.Title)
	if pathLower != "" && strings.Contains(queryLower, pathLower) {
		return true
	}
	if titleLower != "" && len(meaningfulTerms(titleLower)) >= 3 && strings.Contains(queryLower, titleLower) {
		return true
	}
	base := strings.ToLower(filepath.Base(pathLower))
	baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	if len(baseNoExt) >= 8 && strings.Contains(queryLower, baseNoExt) {
		return true
	}
	return false
}

func isLocaleSegment(segment string) bool {
	switch segment {
	case "en", "en-us", "en-gb", "zh", "zh-cn", "zh-tw", "ja", "ko", "fr", "de", "es", "pt", "pt-br", "ru":
		return true
	default:
		return false
	}
}

func isArchiveSegment(segment string) bool {
	switch segment {
	case "archive", "archives", "archived", "legacy", "old", "deprecated":
		return true
	default:
		return false
	}
}

func isGeneratedSegment(segment string) bool {
	switch segment {
	case "generated", "gen", "reference", "references", "docs-generated":
		return true
	default:
		return false
	}
}

func isTemplateSegment(segment string) bool {
	return segment == "template" || segment == "templates"
}

func isExampleSegment(segment string) bool {
	switch segment {
	case "example", "examples", "sample", "samples", "demo", "demos", "fixture", "fixtures":
		return true
	default:
		return false
	}
}

type markdownSection = docsections.Section

type scoredMarkdownSection struct {
	section markdownSection
	score   float64
}

type scoredIndexedSection struct {
	section          IndexedSection
	score            float64
	specificEvidence bool
}

// EnrichCandidatesWithSectionMatches adds conservative query-specific section
// evidence to candidates that were loaded from the SQLite section index.
func EnrichCandidatesWithSectionMatches(candidates []Candidate, query string) []Candidate {
	if len(candidates) == 0 {
		return candidates
	}
	terms := expandedTerms(query)
	queryLower := strings.ToLower(query)
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, enrichCandidateWithSectionMatches(candidate, queryLower, terms))
	}
	return out
}

func enrichCandidateWithSectionMatches(c Candidate, queryLower string, terms map[string]float64) Candidate {
	if len(c.Sections) == 0 || IsSourceContextCandidate(c) {
		return c
	}
	if mode := nonIntentCandidateMode(c); mode != "" && !hasNonIntentModeIntent(queryLower, mode) {
		return c
	}
	if !sectionRetrievalAllowedForCandidate(c, queryLower) {
		return c
	}
	scored := make([]scoredIndexedSection, 0, len(c.Sections))
	for _, section := range c.Sections {
		score, specific := scoreIndexedSectionEvidence(section, queryLower, terms)
		if !specific || score < 4.8 {
			continue
		}
		scored = append(scored, scoredIndexedSection{section: section, score: score, specificEvidence: specific})
	}
	if len(scored) == 0 {
		return c
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].section.StartLine < scored[j].section.StartLine
		}
		return scored[i].score > scored[j].score
	})
	if scored[0].score < 6.0 && !sectionHasIdentifierEvidence(scored[0].section, queryLower) {
		return c
	}
	limit := 2
	if containsAny(queryLower, "agent context", "implement", "implementation", "resume", "continue") {
		limit = 3
	}
	if limit > len(scored) {
		limit = len(scored)
	}
	threshold := scored[0].score * 0.55
	if threshold < 4.8 {
		threshold = 4.8
	}
	selected := make([]scoredIndexedSection, 0, limit)
	for _, section := range scored {
		if len(selected) >= limit {
			break
		}
		if len(selected) > 0 && section.score < threshold {
			continue
		}
		selected = append(selected, section)
	}
	if len(selected) == 0 {
		return c
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].section.StartLine < selected[j].section.StartLine
	})
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	c.Metadata["indexed_section_retrieval_mode"] = "section_aware"
	c.Metadata["indexed_section_match_count"] = strconv.Itoa(len(selected))
	c.Metadata["indexed_section_total"] = strconv.Itoa(len(c.Sections))
	c.Metadata["indexed_section_match_score"] = fmt.Sprintf("%.3f", boundedSectionBoost(selected))
	c.Metadata["indexed_section_match_source"] = "candidate_sections"
	ids := make([]string, 0, len(selected))
	headings := make([]string, 0, len(selected))
	ranges := make([]string, 0, len(selected))
	bodies := make([]string, 0, len(selected))
	for _, hit := range selected {
		ids = append(ids, hit.section.ID)
		headings = append(headings, hit.section.HeadingPath)
		ranges = append(ranges, fmt.Sprintf("%d-%d", hit.section.StartLine, hit.section.EndLine))
		bodies = append(bodies, sectionBodyPreview(hit.section.Body, 2200))
	}
	putMetadataJSONList(c.Metadata, "indexed_section_match_ids_json", ids)
	putMetadataJSONList(c.Metadata, "indexed_section_match_headings_json", headings)
	putMetadataJSONList(c.Metadata, "indexed_section_match_ranges_json", ranges)
	putMetadataJSONList(c.Metadata, "indexed_section_match_bodies_json", bodies)
	return c
}

func sectionRetrievalAllowedForCandidate(c Candidate, queryLower string) bool {
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	if isRoadmapPath(pathLower) && !hasRoadmapIntent(queryLower) {
		return false
	}
	return true
}

func scoreIndexedSectionEvidence(section IndexedSection, queryLower string, terms map[string]float64) (float64, bool) {
	headingLower := strings.ToLower(section.HeadingPath)
	titleLower := strings.ToLower(section.Title)
	bodyLower := strings.ToLower(section.Body)
	score := 0.0
	specific := false
	identifierEvidence := false
	coreTerms := coreQueryTerms(queryLower)
	for _, term := range coreTerms {
		if term == "" {
			continue
		}
		if strings.Contains(headingLower, term) || strings.Contains(titleLower, term) {
			score += 5.0
			specific = true
		}
		hits := strings.Count(bodyLower, term)
		if hits > 3 {
			hits = 3
		}
		if hits > 0 {
			score += float64(hits) * 1.4
			specific = true
		}
	}
	for _, term := range identifierQueryTerms(queryLower) {
		if term == "" {
			continue
		}
		if strings.Contains(headingLower, term) || strings.Contains(titleLower, term) {
			score += 6.0
			specific = true
			identifierEvidence = true
		}
		if strings.Contains(bodyLower, term) {
			score += 5.0
			specific = true
			identifierEvidence = true
		}
		for _, part := range splitIdentifier(term) {
			if len(part) < 4 {
				continue
			}
			if strings.Contains(headingLower, part) || strings.Contains(bodyLower, part) {
				score += 0.8
				specific = true
			}
		}
	}
	if !specific {
		for term, weight := range terms {
			if term == "" || genericSectionTerm(term) {
				continue
			}
			if strings.Contains(headingLower, term) || strings.Contains(titleLower, term) {
				score += 3.0 * weight
				specific = true
			}
		}
	}
	if !specific {
		return 0, false
	}
	if genericSectionHeading(headingLower) && !identifierEvidence {
		bodySpecific := false
		for _, term := range coreTerms {
			if term != "" && strings.Contains(bodyLower, term) {
				bodySpecific = true
				break
			}
		}
		if !bodySpecific {
			return 0, false
		}
		score -= 2.0
	}
	switch {
	case containsAny(queryLower, "decision", "adr", "why", "rationale") && containsAny(headingLower, "decision", "rationale", "consequences"):
		score += 2.0
	case containsAny(queryLower, "design", "architecture") && containsAny(headingLower, "design", "architecture"):
		score += 2.0
	case containsAny(queryLower, "scope", "requirement", "requirements", "acceptance") && containsAny(headingLower, "scope", "requirement", "requirements", "acceptance"):
		score += 1.6
	case containsAny(queryLower, "task", "tasks", "todo", "implement", "implementation") && containsAny(headingLower, "task", "tasks", "todo", "implementation"):
		score += 1.4
	}
	if sectionLooksMostlyCode(section.Body) && !hasExplicitSourceIntent(queryLower) && !identifierEvidence {
		score -= 3.0
	}
	return score, score >= 4.0
}

func boundedSectionBoost(selected []scoredIndexedSection) float64 {
	if len(selected) == 0 {
		return 0
	}
	score := selected[0].score
	for _, extra := range selected[1:] {
		score += extra.score * 0.25
	}
	if score > 8.0 {
		return 8.0
	}
	return score
}

func sectionHasIdentifierEvidence(section IndexedSection, queryLower string) bool {
	text := strings.ToLower(section.HeadingPath + "\n" + section.Title + "\n" + section.Body)
	for _, term := range identifierQueryTerms(queryLower) {
		if term != "" && strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func genericSectionHeading(headingLower string) bool {
	parts := strings.Split(headingLower, ">")
	leaf := strings.TrimSpace(parts[len(parts)-1])
	switch leaf {
	case "overview", "introduction", "background", "summary", "notes", "details", "misc", "appendix":
		return true
	default:
		return false
	}
}

func genericSectionTerm(term string) bool {
	switch term {
	case "agent", "context", "document", "documents", "implementation", "implement", "plan", "spec", "requirements", "design", "task", "tasks":
		return true
	default:
		return false
	}
}

func isRoadmapPath(pathLower string) bool {
	return strings.HasSuffix(pathLower, "roadmap.md") ||
		strings.Contains(pathLower, "/roadmap.") ||
		hasPathSegment(pathLower, "roadmap") ||
		hasPathSegment(pathLower, "roadmaps")
}

func hasRoadmapIntent(queryLower string) bool {
	return containsAny(queryLower,
		"roadmap", "roadmaps", "timeline", "milestone", "milestones",
		"release plan", "release planning", "future work", "planned work",
		"quarterly plan", "q1", "q2", "q3", "q4")
}

func sectionLooksMostlyCode(body string) bool {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return false
	}
	codeLines := 0
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isMarkdownFence(trimmed) {
			inFence = !inFence
			codeLines++
			continue
		}
		if inFence || strings.HasPrefix(trimmed, "    ") || strings.HasPrefix(trimmed, "\t") {
			codeLines++
		}
	}
	return len(lines) >= 8 && float64(codeLines)/float64(len(lines)) > 0.65
}

func sectionBodyPreview(body string, maxChars int) string {
	body = strings.TrimSpace(body)
	if maxChars <= 0 || len(body) <= maxChars {
		return body
	}
	return strings.TrimSpace(body[:maxChars]) + "\n..."
}

func putMetadataJSONList(metadata map[string]string, key string, values []string) {
	if len(values) == 0 {
		return
	}
	b, err := json.Marshal(values)
	if err != nil {
		return
	}
	metadata[key] = string(b)
}

func packCandidateSections(candidates []Candidate, queryLower string, terms map[string]float64) []Candidate {
	if len(candidates) == 0 {
		return candidates
	}
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, packCandidateSection(candidate, queryLower, terms))
	}
	return out
}

func packCandidateSection(c Candidate, queryLower string, terms map[string]float64) Candidate {
	if !isMarkdownCandidatePath(c.Path) || len(c.Body) < 2400 || IsSourceContextCandidate(c) {
		return c
	}
	sections := extractMarkdownSections(c.Body)
	if len(sections) < 3 {
		return c
	}
	selected := selectMarkdownSections(sections, queryLower, terms)
	if len(selected) == 0 || len(selected) >= len(sections) {
		return c
	}
	packedBody := renderPackedSections(c, selected, len(sections), terms)
	if len(packedBody) == 0 || len(packedBody) >= int(float64(len(c.Body))*0.88) {
		return c
	}
	if c.Metadata == nil {
		c.Metadata = map[string]string{}
	} else {
		c.Metadata = copyMetadata(c.Metadata)
	}
	headings := make([]string, 0, len(selected))
	for _, selectedSection := range selected {
		headings = append(headings, selectedSection.section.HeadingPath)
	}
	c.Metadata["section_pack_mode"] = "sections"
	c.Metadata["section_pack_count"] = strconv.Itoa(len(selected))
	c.Metadata["section_pack_total"] = strconv.Itoa(len(sections))
	c.Metadata["section_pack_headings"] = strings.Join(headings, "\n")
	c.Body = packedBody
	return c
}

func isMarkdownCandidatePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdx":
		return true
	default:
		return false
	}
}

func extractMarkdownSections(body string) []markdownSection {
	return docsections.ExtractMarkdown(body)
}

func isMarkdownFence(trimmed string) bool {
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func selectMarkdownSections(sections []markdownSection, queryLower string, terms map[string]float64) []scoredMarkdownSection {
	scored := make([]scoredMarkdownSection, 0, len(sections))
	for _, section := range sections {
		score := scoreMarkdownSection(section, queryLower, terms)
		if score > 0 {
			scored = append(scored, scoredMarkdownSection{section: section, score: score})
		}
	}
	if len(scored) == 0 {
		return nil
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].section.StartLine < scored[j].section.StartLine
		}
		return scored[i].score > scored[j].score
	})
	if scored[0].score < 2.2 {
		return nil
	}
	limit := 3
	if containsAny(queryLower, "agent context", "implement", "implementation", "resume", "continue") {
		limit = 4
	}
	if limit > len(scored) {
		limit = len(scored)
	}
	threshold := scored[0].score * 0.35
	if threshold < 1.2 {
		threshold = 1.2
	}
	selected := make([]scoredMarkdownSection, 0, limit)
	for _, section := range scored {
		if len(selected) >= limit {
			break
		}
		if len(selected) > 0 && section.score < threshold {
			continue
		}
		selected = append(selected, section)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].section.StartLine < selected[j].section.StartLine
	})
	return selected
}

func scoreMarkdownSection(section markdownSection, queryLower string, terms map[string]float64) float64 {
	headingLower := strings.ToLower(section.HeadingPath)
	bodyLower := strings.ToLower(section.Body)
	score := 0.0
	for term, weight := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(headingLower, term) {
			score += 4.0 * weight
		}
		hits := strings.Count(bodyLower, term)
		if hits > 4 {
			hits = 4
		}
		score += float64(hits) * weight
	}
	if len(section.Tasks) > 0 && containsAny(queryLower, "task", "tasks", "todo", "implement", "implementation", "resume", "continue") {
		score += 1.4
	}
	if len(section.AcceptanceCriteria) > 0 && containsAny(queryLower, "acceptance", "criteria", "requirement", "requirements", "prd", "product") {
		score += 1.2
	}
	switch {
	case containsAny(queryLower, "decision", "adr", "why", "rationale") && containsAny(headingLower, "decision", "rationale", "consequences"):
		score += 1.5
	case containsAny(queryLower, "design", "architecture") && containsAny(headingLower, "design", "architecture", "overview"):
		score += 1.5
	case containsAny(queryLower, "alternative", "alternatives", "rfc") && containsAny(headingLower, "alternative", "alternatives", "drawback", "drawbacks"):
		score += 1.2
	case containsAny(queryLower, "scope", "requirement", "requirements") && containsAny(headingLower, "scope", "requirement", "requirements"):
		score += 1.2
	}
	return score
}

func renderPackedSections(c Candidate, selected []scoredMarkdownSection, totalSections int, terms map[string]float64) string {
	source := c.Source
	if source == "" {
		source = c.Path
	}
	var b strings.Builder
	fmtSectionHeader := func(s scoredMarkdownSection) {
		fmt.Fprintf(&b, "### %s\n", s.section.HeadingPath)
		fmt.Fprintf(&b, "Source: %s\n", source)
		fmt.Fprintf(&b, "Lines: %d-%d\n\n", s.section.StartLine, s.section.EndLine)
	}
	fmt.Fprintf(&b, "Section-packed artifact\n")
	fmt.Fprintf(&b, "Source: %s\n", source)
	fmt.Fprintf(&b, "Selected sections: %d/%d\n", len(selected), totalSections)
	if fm := frontmatterSummary(selected); fm != "" {
		fmt.Fprintf(&b, "Frontmatter: %s\n", fm)
	}
	for _, section := range selected {
		fmt.Fprintln(&b)
		fmtSectionHeader(section)
		fmt.Fprintf(&b, "%s\n", sectionExcerpt(section.section, terms, 2200))
	}
	return strings.TrimRight(b.String(), "\r\n")
}

func sectionExcerpt(section markdownSection, terms map[string]float64, maxChars int) string {
	body := strings.TrimSpace(section.Body)
	if len(body) <= maxChars {
		return body
	}
	lines := strings.Split(body, "\n")
	selected := map[int]bool{}
	for i, line := range lines {
		lineLower := strings.ToLower(line)
		if sectionLineMatches(lineLower, terms) || isTaskLikeLine(lineLower) {
			for j := i - 1; j <= i+1; j++ {
				if j >= 0 && j < len(lines) {
					selected[j] = true
				}
			}
		}
	}
	if len(selected) == 0 {
		return strings.TrimSpace(body[:maxChars]) + "\n..."
	}
	var indexes []int
	for idx := range selected {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	var b strings.Builder
	last := -2
	for _, idx := range indexes {
		if b.Len() >= maxChars {
			break
		}
		if last >= 0 && idx > last+1 {
			b.WriteString("...\n")
		}
		b.WriteString(lines[idx])
		b.WriteByte('\n')
		last = idx
	}
	out := strings.TrimSpace(b.String())
	if len(out) > maxChars {
		out = strings.TrimSpace(out[:maxChars]) + "\n..."
	}
	return out
}

func sectionLineMatches(lineLower string, terms map[string]float64) bool {
	for term := range terms {
		if term != "" && strings.Contains(lineLower, term) {
			return true
		}
	}
	return false
}

func isTaskLikeLine(lineLower string) bool {
	lineLower = strings.TrimSpace(lineLower)
	return strings.HasPrefix(lineLower, "- [ ]") ||
		strings.HasPrefix(lineLower, "- [x]") ||
		strings.HasPrefix(lineLower, "* [ ]") ||
		strings.HasPrefix(lineLower, "* [x]")
}

func frontmatterSummary(selected []scoredMarkdownSection) string {
	if len(selected) == 0 || len(selected[0].section.Frontmatter) == 0 {
		return ""
	}
	keys := make([]string, 0, len(selected[0].section.Frontmatter))
	for key := range selected[0].section.Frontmatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 4 {
		keys = keys[:4]
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+selected[0].section.Frontmatter[key])
	}
	return strings.Join(parts, ", ")
}

func selectScoredCandidates(candidates []scoredCandidate, queryLower string, limit int) []scoredCandidate {
	if len(candidates) <= limit {
		if len(candidates) <= 3 {
			return candidates
		}
		if hasAnchoredCandidate(candidates) {
			return filterWeakBodyOnlyBackfill(candidates, queryLower)
		}
		return candidates
	}
	if !hasAnchoredCandidate(candidates) {
		return candidates[:limit]
	}
	selected := make([]scoredCandidate, 0, limit)
	for _, candidate := range candidates {
		if isWeakBodyOnlyBackfill(candidate, queryLower) {
			continue
		}
		selected = append(selected, candidate)
		if len(selected) == limit {
			return selected
		}
	}
	if len(selected) == 0 {
		return candidates[:limit]
	}
	return selected
}

func enforceSupportingArtifactBudgets(candidates []scoredCandidate, queryLower string, terms map[string]float64) []scoredCandidate {
	testBudget := 0
	switch {
	case hasSpecificTestNameIntent(queryLower):
		testBudget = 2
	case hasTestBehaviorIntent(queryLower):
		testBudget = 3
	case hasExplicitSourceIntent(queryLower) || hasIdentifierTerm(terms):
		testBudget = 2
	}
	commentBudget := 0
	switch {
	case hasCodeCommentIntent(queryLower):
		commentBudget = 3
	case hasExplicitSourceIntent(queryLower) || hasIdentifierTerm(terms):
		commentBudget = 2
	}
	testCount := 0
	commentCount := 0
	out := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.role {
		case "test_case":
			if testCount >= testBudget {
				continue
			}
			testCount++
		case "code_comment":
			if commentCount >= commentBudget {
				continue
			}
			commentCount++
		}
		out = append(out, candidate)
	}
	return out
}

func suppressRawTestSourceCandidates(candidates []scoredCandidate, queryLower string) []scoredCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	testUnitKeys := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.role != "test_case" {
			continue
		}
		if key := lineScopedSourceFileKey(candidate.candidate.Path); key != "" {
			testUnitKeys[key] = true
		}
	}
	if len(testUnitKeys) == 0 && hasTestBehaviorIntent(queryLower) {
		return candidates
	}
	out := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !isRawTestSourceCandidate(candidate.candidate) {
			out = append(out, candidate)
			continue
		}
		if !hasTestBehaviorIntent(queryLower) && !hasExplicitSourceIntent(queryLower) {
			continue
		}
		if len(testUnitKeys) > 0 && !variantHasExactQueryAnchor(candidate.candidate, queryLower) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func hasAnchoredCandidate(candidates []scoredCandidate) bool {
	for _, candidate := range candidates {
		if isAnchoredCandidate(candidate) {
			return true
		}
	}
	return false
}

func isAnchoredCandidate(candidate scoredCandidate) bool {
	if candidate.profile.pathTitleCoreMatches > 0 || candidate.profile.identifierMatches > 0 {
		return true
	}
	if candidate.sourceFile && candidate.profile.coreMatches > 0 {
		return true
	}
	switch candidate.role {
	case "adr", "prd", "rfc", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		return candidate.profile.coreMatches >= 2
	default:
		return false
	}
}

func filterWeakBodyOnlyBackfill(candidates []scoredCandidate, queryLower string) []scoredCandidate {
	out := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if isWeakBodyOnlyBackfill(candidate, queryLower) {
			continue
		}
		out = append(out, candidate)
	}
	if len(out) == 0 {
		return candidates
	}
	return out
}

func isWeakBodyOnlyBackfill(candidate scoredCandidate, queryLower string) bool {
	if candidate.profile.coreTerms < 3 {
		return false
	}
	if candidate.profile.pathTitleCoreMatches > 0 || candidate.profile.identifierMatches > 0 {
		return false
	}
	if candidate.sourceFile {
		return false
	}
	if mode := nonIntentCandidateMode(candidate.candidate); mode != "" && !hasNonIntentModeIntent(queryLower, mode) {
		return true
	}
	if strings.HasPrefix(candidate.role, "openspec_") || candidate.role == "adr" || candidate.role == "prd" || candidate.role == "rfc" {
		return false
	}
	pathLower := strings.ToLower(filepath.ToSlash(candidate.candidate.Path))
	return isBroadMarkdownRole(candidate.role) ||
		candidate.role == "" ||
		strings.HasPrefix(pathLower, "docs/") ||
		strings.Contains(pathLower, "/docs/")
}

func retrievalLimit(queryLower string, terms map[string]float64) int {
	limit := 8
	switch {
	case hasExplicitSourceIntent(queryLower):
		limit = 5
	case hasTestBehaviorIntent(queryLower):
		limit = 5
	case hasCodeCommentIntent(queryLower):
		limit = 5
	case hasNonIntentModeIntent(queryLower, "protocol") || hasNonIntentModeIntent(queryLower, "template"):
		limit = 5
	case containsAny(queryLower, "rfc", "request for comments", "proposal", "alternatives"):
		limit = 5
	case containsAny(queryLower, "architecture", "design document", "technical design", "system design"):
		limit = 6
	case strings.Contains(queryLower, "implementation context") || strings.Contains(queryLower, "agent context") || strings.Contains(queryLower, "implement"):
		limit = 6
	case strings.Contains(queryLower, "resume") || strings.Contains(queryLower, "continue"):
		limit = 7
	case strings.Contains(queryLower, "why") || strings.Contains(queryLower, "decision") || strings.Contains(queryLower, "adr"):
		limit = 5
	case strings.Contains(queryLower, "prd") || strings.Contains(queryLower, "product") || strings.Contains(queryLower, "background"):
		limit = 5
	case strings.Contains(queryLower, "stale") || strings.Contains(queryLower, "superseded"):
		limit = 5
	}
	if hasIdentifierTerm(terms) && limit > 6 {
		limit = 6
	}
	return limit
}

func scoreCandidate(c Candidate, terms map[string]float64, queryLower string) float64 {
	pathLower := strings.ToLower(c.Path)
	titleLower := strings.ToLower(c.Title)
	bodyLower := strings.ToLower(c.Body)
	score := 0.0
	sourceFile := IsSourceContextCandidate(c)
	role := candidateRole(c)
	profile := candidateMatchProfile(c, queryLower)
	explicitSourceIntent := hasExplicitSourceIntent(queryLower)
	planIntent := hasPlanIntent(queryLower)
	productBackgroundIntent := hasProductBackgroundIntent(queryLower)
	lifecycleIntent := hasLifecycleIntent(queryLower)
	identifierHeavy := profile.identifierTerms >= 2
	for term, weight := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(pathLower, term) {
			score += 6.0 * weight
		}
		if strings.Contains(titleLower, term) {
			score += 4.0 * weight
		}
		hits := strings.Count(bodyLower, term)
		if cap := bodyHitCap(role, sourceFile, term); hits > cap {
			hits = cap
		}
		score += float64(hits) * weight
	}

	if IsPlanningIntentPath(c.Path) {
		score += 1.0
	}
	if isLikelyUnrequestedLocalizedPath(pathLower, queryLower) {
		score -= 5.0
	}
	if hasQueryWord(queryLower, "architecture") && strings.Contains(filepath.ToSlash(pathLower), "/architecture/") {
		score += 2.0
	}
	if hasQueryWord(queryLower, "design") && strings.HasSuffix(filepath.ToSlash(pathLower), "/design.md") {
		score += 6.0
	}
	if strings.Contains(filepath.Base(pathLower), "metadata") && !hasQueryWord(queryLower, "metadata") {
		score -= 4.0
	}
	if profile.coreTerms > 0 {
		switch {
		case profile.coreMatches == 0:
			score -= 6.0
		case profile.coreTerms >= 3 && profile.coreMatches == 1:
			score -= 3.0
		}
		if isBroadMarkdownRole(role) && profile.pathTitleCoreMatches == 0 && profile.coreTerms >= 3 && profile.coreMatches < 2 {
			score -= 2.0
		}
		if sameFamilyNeedsCoreEvidence(role) && profile.coreTerms >= 3 && profile.coreMatches < 2 {
			score -= 3.0
		}
	}
	switch role {
	case "adr":
		if containsAny(queryLower, "adr", "decision", "boundary", "why", "architecture", "rationale", "superseded", "stale") {
			score += 3.0
		} else if planIntent && profile.coreMatches >= 2 {
			score += 3.0
		} else if productBackgroundIntent {
			score += 1.5
		}
	case "prd":
		if containsAny(queryLower, "prd", "product", "background", "requirements", "user") {
			score += 4.0
		} else if containsAny(queryLower, "implement", "implementation", "agent context", "source file") {
			score -= 3.0
		}
	case "rfc":
		if containsAny(queryLower, "rfc", "request for comments", "proposal", "alternative", "alternatives", "motivation", "drawback", "drawbacks", "design") {
			score += 4.0
		} else if explicitSourceIntent {
			score -= 2.0
		}
	case "design":
		if containsAny(queryLower, "architecture", "design", "technical", "system", "overview", "proposal") {
			score += 4.0
			if profile.pathTitleCoreMatches > 0 {
				score += 2.0
			}
		} else if explicitSourceIntent {
			score -= 2.0
		}
	case "openspec_bundle":
		if hasOpenSpecStructureIntent(queryLower) {
			score += 5.0
			if profile.identifierMatches > 0 {
				score += 4.0
			}
		} else {
			score = -100.0
		}
	case "openspec_design":
		if containsAny(queryLower, "design", "rationale", "why", "context", "implement", "implementation", "agent context") {
			score += 3.0
		}
		if containsAny(queryLower, "implement", "implementation", "agent context") {
			score += 6.0
		}
		if hasRFCIntent(queryLower) && profile.coreMatches >= 2 {
			score += 3.0
		}
		if profile.identifierMatches > 0 {
			score += 4.0
		}
	case "openspec_tasks":
		if containsAny(queryLower, "task", "todo", "implement", "implementation", "resume", "continue", "agent context") {
			score += 3.0
		}
		if containsAny(queryLower, "implement", "implementation", "agent context") {
			score += 6.0
		}
		if !hasOpenSpecChildRoleIntent(queryLower) {
			score -= 6.0
		}
		if profile.identifierMatches > 0 {
			score += 4.0
		}
	case "openspec_spec":
		if containsAny(queryLower, "spec", "delta", "requirement", "acceptance") {
			score += 2.0
		} else if containsAny(queryLower, "implement", "implementation", "agent context") {
			score -= 1.5
		}
		if !hasOpenSpecChildRoleIntent(queryLower) {
			score -= 6.0
		}
	case "openspec_proposal":
		if containsAny(queryLower, "proposal", "context", "implement", "implementation", "resume", "continue", "agent context", "rfc") {
			score += 2.0
		}
	case "plan":
		if containsAny(queryLower, "plan", "resume", "continue", "migration", "notes") && profile.coreMatches > 0 {
			score += 2.0
		}
	case "agent_note":
		if containsAny(queryLower, "resume", "continue", "followup", "follow-up", "agent", "notes") && profile.coreMatches > 0 {
			score += 2.0
		}
	case "agent_instruction", "protocol", "skill":
		if hasNonIntentModeIntent(queryLower, "protocol") {
			score += 3.0
			if profile.pathTitleCoreMatches > 0 {
				score += 4.0
			}
			if hasRepositoryInstructionIntent(queryLower) {
				if pathDepth(pathLower) == 0 {
					score += 8.0
				} else if !queryMentionsInstructionPathSubject(queryLower, pathLower) {
					score = -100.0
				}
			}
		} else {
			score = -100.0
		}
	case "template":
		if hasNonIntentModeIntent(queryLower, "template") {
			score += 5.0
			if profile.pathTitleCoreMatches > 0 {
				score += 3.0
			}
		} else {
			score -= 10.0
		}
	case "model":
		if hasNonIntentModeIntent(queryLower, "model") {
			score += 4.0
			if profile.pathTitleCoreMatches > 0 || profile.identifierMatches > 0 {
				score += 3.0
			}
		} else {
			score -= 10.0
		}
	case "test_case":
		testAnchor := testNameAnchorScore(c, queryLower)
		if hasTestBehaviorIntent(queryLower) || explicitSourceIntent || profile.identifierMatches > 0 || testAnchor.score > 0 {
			score += 2.5
			if profile.pathTitleCoreMatches > 0 {
				score += 2.0
			}
			if profile.identifierMatches > 0 {
				score += float64(profile.identifierMatches) * 1.5
			}
			if testAnchor.score > 0 {
				score += testAnchor.score
			}
		} else {
			score = -100.0
		}
	case "code_comment":
		if hasCodeCommentIntent(queryLower) || explicitSourceIntent || profile.identifierMatches > 0 {
			score += 2.0
			if profile.pathTitleCoreMatches > 0 {
				score += 2.0
			}
			if profile.identifierMatches > 0 {
				score += float64(profile.identifierMatches) * 1.2
			}
		} else {
			score = -100.0
		}
	}
	if sourceFile {
		if explicitSourceIntent || containsAny(queryLower, "implement", "implementation", "handler") || hasIdentifierTerm(terms) {
			score += 2.0
		} else if profile.pathTitleCoreMatches > 0 {
			score += 2.0
		} else {
			score -= 6.0
		}
		if containsAny(queryLower, "boundary") && profile.pathTitleCoreMatches >= 2 {
			score += 3.0
		}
		if hasQueryWord(queryLower, "session") && strings.Contains(pathLower, "/session.") {
			score += 5.0
		}
	}
	if planIntent && !productBackgroundIntent && role == "prd" {
		score -= 4.0
	}
	if planIntent && !explicitSourceIntent {
		if sourceFile {
			score -= 8.0
		}
		if role == "agent_note" && profile.pathTitleCoreMatches < 2 {
			score -= 8.0
		}
	}
	if hasRFCIntent(queryLower) && !explicitSourceIntent {
		if role == "agent_note" {
			score -= 12.0
		}
		if sourceFile && strings.Contains(pathLower, "/migrations/") {
			score -= 8.0
		}
	}
	if productBackgroundIntent {
		switch {
		case sourceFile && !explicitSourceIntent:
			score = -100.0
		case role == "prd" && profile.pathTitleCoreMatches >= 2:
			score += 12.0
			if hasUnrequestedProductSurface(pathLower, titleLower, queryLower) {
				score -= 10.0
			}
		case role == "prd":
			score -= 20.0
			if hasUnrequestedProductSurface(pathLower, titleLower, queryLower) {
				score -= 10.0
			}
		case role == "adr" && isDurableBackgroundDecision(c, pathLower, titleLower, bodyLower, profile):
			score += 14.0
			if hasUnrequestedProductSurface(pathLower, titleLower, queryLower) {
				score -= 24.0
			}
		case role == "adr":
			score = -100.0
		case role == "plan" || role == "agent_note":
			score = -100.0
		case strings.HasPrefix(role, "openspec_"):
			score = -100.0
		case role == "rfc":
			score = -100.0
		default:
			score -= 8.0
		}
	}
	if explicitSourceIntent {
		if sourceFile {
			score += 4.0
		} else {
			switch role {
			case "adr", "openspec_design", "openspec_tasks", "rfc":
				score -= 1.0
			case "prd", "plan", "agent_note", "openspec_proposal", "openspec_spec":
				score -= 3.0
			default:
				score -= 2.0
			}
		}
	}
	if identifierHeavy {
		if profile.identifierMatches == 0 {
			score -= 4.0
		}
		if sourceFile {
			if explicitSourceIntent && profile.identifierMatches < profile.identifierTerms {
				score -= 30.0
			}
			score += float64(profile.identifierMatches) * 2.0
			if profile.identifierMatches == profile.identifierTerms {
				score += 3.0
			}
		} else {
			if isBroadMarkdownRole(role) && profile.identifierMatches < profile.identifierTerms {
				score -= 2.0
			}
			if explicitSourceIntent {
				score -= 12.0
			}
		}
	}
	if strings.Contains(pathLower, "scratch/") && !hasQueryWord(queryLower, "scratch") {
		score -= 30.0
	} else if strings.Contains(pathLower, "old-") || strings.Contains(pathLower, "legacy") {
		score -= 4.0
	}
	if hasUnrequestedContextSurface(pathLower, titleLower, queryLower) {
		score -= 5.0
	}
	if lifecycleIntent && candidateIsStale(c, pathLower, bodyLower) && profile.coreTerms >= 3 && profile.coreMatches < 2 {
		score -= 8.0
	}
	if lifecycleIntent && !candidateIsStale(c, pathLower, bodyLower) {
		score -= 30.0
	}
	if candidateIsStale(c, pathLower, bodyLower) {
		if containsAny(queryLower, "stale", "superseded", "old", "local", "caching", "history", "why") {
			score += 6.0
		} else {
			score -= 5.0
		}
	}
	if mode := nonIntentCandidateMode(c); mode != "" && !hasNonIntentModeIntent(queryLower, mode) {
		score -= 10.0
	}
	return score
}

func indexedSectionScore(c Candidate, terms map[string]float64, queryLower string) float64 {
	if c.Metadata == nil || c.Metadata["indexed_section_retrieval_mode"] != "section_aware" {
		return 0
	}
	if value := strings.TrimSpace(c.Metadata["indexed_section_match_score"]); value != "" {
		if score, err := strconv.ParseFloat(value, 64); err == nil {
			if score > 8.0 {
				return 8.0
			}
			return score
		}
	}
	headings := metadataJSONList(c, "indexed_section_match_headings_json")
	ranges := metadataJSONList(c, "indexed_section_match_ranges_json")
	bodies := metadataJSONList(c, "indexed_section_match_bodies_json")
	count := len(headings)
	if count == 0 {
		if n, err := strconv.Atoi(c.Metadata["indexed_section_match_count"]); err == nil {
			count = n
		}
	}
	if count == 0 {
		return 0
	}
	score := 2.0 + float64(count)
	for i, heading := range headings {
		headingLower := strings.ToLower(heading)
		bodyLower := ""
		if i < len(bodies) {
			bodyLower = strings.ToLower(bodies[i])
		}
		for term, weight := range terms {
			if term == "" {
				continue
			}
			if strings.Contains(headingLower, term) {
				score += 4.0 * weight
			}
			hits := strings.Count(bodyLower, term)
			if hits > 3 {
				hits = 3
			}
			score += float64(hits) * weight
		}
		switch {
		case containsAny(queryLower, "decision", "adr", "why", "rationale") && containsAny(headingLower, "decision", "rationale", "consequences"):
			score += 1.5
		case containsAny(queryLower, "design", "architecture") && containsAny(headingLower, "design", "architecture", "overview"):
			score += 1.5
		case containsAny(queryLower, "scope", "requirement", "requirements", "acceptance") && containsAny(headingLower, "scope", "requirement", "requirements", "acceptance"):
			score += 1.2
		case containsAny(queryLower, "task", "tasks", "todo", "implement", "implementation") && containsAny(headingLower, "task", "tasks", "todo", "implementation"):
			score += 1.2
		}
	}
	if len(ranges) > 0 {
		score += 0.5
	}
	return score
}

type matchProfile struct {
	coreTerms            int
	coreMatches          int
	pathTitleCoreMatches int
	identifierTerms      int
	identifierMatches    int
}

func candidateMatchProfile(c Candidate, queryLower string) matchProfile {
	pathLower := strings.ToLower(c.Path)
	titleLower := strings.ToLower(c.Title)
	bodyLower := strings.ToLower(c.Body)
	var profile matchProfile
	for _, term := range coreQueryTerms(queryLower) {
		profile.coreTerms++
		pathOrTitle := strings.Contains(pathLower, term) || strings.Contains(titleLower, term)
		if pathOrTitle || strings.Contains(bodyLower, term) {
			profile.coreMatches++
		}
		if pathOrTitle {
			profile.pathTitleCoreMatches++
		}
	}
	for _, term := range identifierQueryTerms(queryLower) {
		profile.identifierTerms++
		if strings.Contains(pathLower, term) || strings.Contains(titleLower, term) || strings.Contains(bodyLower, term) {
			profile.identifierMatches++
			continue
		}
		if isTestCaseCandidate(c) && testNameAnchorContainsCompact(c, term) {
			profile.identifierMatches++
		}
	}
	return profile
}

func coreQueryTerms(queryLower string) []string {
	generic := map[string]bool{
		"adr": true, "architecture": true, "background": true, "code": true,
		"context": true, "decision": true, "design": true, "document": true,
		"documents": true, "documentation": true, "file": true,
		"fix": true, "generate": true, "generating": true, "create": true,
		"creating": true, "implement": true, "implementation": true,
		"instruction": true, "instructions": true, "note": true, "notes": true,
		"plan": true, "prd": true, "product": true, "proposal": true,
		"requirements": true, "rfc": true, "source": true, "spec": true,
		"task": true, "tasks": true, "same": true, "share": true, "shared": true,
		"use": true, "using": true, "user": true, "users": true,
	}
	var out []string
	for _, term := range meaningfulTerms(queryLower) {
		if generic[term] {
			continue
		}
		out = append(out, term)
	}
	return out
}

func identifierQueryTerms(queryLower string) []string {
	var out []string
	for _, term := range meaningfulTerms(queryLower) {
		if strings.ContainsAny(term, "_.-") || looksLikeCompactTestIdentifier(term) {
			out = append(out, term)
		}
	}
	return out
}

type testAnchorScore struct {
	score  float64
	reason string
}

func testNameAnchorScore(c Candidate, queryLower string) testAnchorScore {
	if !isTestCaseCandidate(c) {
		return testAnchorScore{}
	}
	anchors := testNameAnchors(c)
	if len(anchors) == 0 {
		return testAnchorScore{}
	}
	queryTerms := meaningfulTestAnchorTerms(queryLower)
	if len(queryTerms) == 0 {
		return testAnchorScore{}
	}
	for _, anchor := range anchors {
		if anchor.compact == "" {
			continue
		}
		for _, term := range queryTerms {
			if term == anchor.compact || term == anchor.withoutTestPrefix {
				return testAnchorScore{score: 10.0, reason: "exact test-name anchor"}
			}
		}
	}
	if !hasTestBehaviorIntent(queryLower) {
		return testAnchorScore{}
	}
	querySet := map[string]bool{}
	for _, term := range queryTerms {
		if len(term) >= 3 {
			querySet[term] = true
		}
	}
	bestMatches := 0
	bestAnchorTerms := 0
	for _, anchor := range anchors {
		if len(anchor.parts) == 0 {
			continue
		}
		matches := 0
		anchorTerms := 0
		for _, part := range anchor.parts {
			if genericTestAnchorTerm(part) {
				continue
			}
			anchorTerms++
			if querySet[part] {
				matches++
			}
		}
		if matches > bestMatches || (matches == bestMatches && anchorTerms > bestAnchorTerms) {
			bestMatches = matches
			bestAnchorTerms = anchorTerms
		}
	}
	switch {
	case bestMatches >= 4:
		return testAnchorScore{score: 7.0, reason: "test-name token anchor"}
	case bestMatches >= 3:
		return testAnchorScore{score: 5.5, reason: "test-name token anchor"}
	case bestMatches >= 2 && bestAnchorTerms <= 3:
		return testAnchorScore{score: 3.5, reason: "test-name token anchor"}
	default:
		return testAnchorScore{}
	}
}

type normalizedTestAnchor struct {
	parts             []string
	compact           string
	withoutTestPrefix string
}

func testNameAnchors(c Candidate) []normalizedTestAnchor {
	seen := map[string]bool{}
	var anchors []normalizedTestAnchor
	add := func(value string) {
		anchor := normalizeTestAnchor(value)
		if anchor.compact == "" || seen[anchor.compact] {
			return
		}
		seen[anchor.compact] = true
		anchors = append(anchors, anchor)
	}
	add(c.Title)
	if c.Metadata != nil {
		add(c.Metadata["test_name"])
		add(c.Metadata["parent_title"])
	}
	if c.Body != "" {
		for _, line := range strings.Split(c.Body, "\n") {
			line = strings.TrimSpace(line)
			if value, ok := strings.CutPrefix(line, "Test:"); ok {
				add(value)
			}
		}
	}
	return anchors
}

func normalizeTestAnchor(value string) normalizedTestAnchor {
	parts := splitIdentifierParts(value)
	parts = filterEmptyStrings(parts)
	compact := strings.Join(parts, "")
	withoutPrefix := compact
	if strings.HasPrefix(withoutPrefix, "test") && len(withoutPrefix) > 4 {
		withoutPrefix = strings.TrimPrefix(withoutPrefix, "test")
	}
	return normalizedTestAnchor{parts: parts, compact: compact, withoutTestPrefix: withoutPrefix}
}

func meaningfulTestAnchorTerms(queryLower string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" || seen[term] || genericTestAnchorTerm(term) {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, term := range meaningfulTerms(queryLower) {
		add(term)
		if strings.ContainsAny(term, "_.-") {
			for _, part := range splitIdentifier(term) {
				add(part)
			}
		}
		if looksLikeCompactTestIdentifier(term) {
			add(strings.TrimPrefix(term, "test"))
		}
	}
	return out
}

func testNameAnchorContainsCompact(c Candidate, term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	for _, anchor := range testNameAnchors(c) {
		if anchor.compact == term || anchor.withoutTestPrefix == term {
			return true
		}
	}
	return false
}

func looksLikeCompactTestIdentifier(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if len(term) < 9 || !strings.HasPrefix(term, "test") {
		return false
	}
	hasLetter := false
	for _, r := range term[4:] {
		if unicode.IsDigit(r) {
			continue
		}
		if unicode.IsLetter(r) {
			hasLetter = true
			continue
		}
		return false
	}
	return hasLetter
}

func splitIdentifierParts(s string) []string {
	var out []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, strings.ToLower(string(current)))
		current = current[:0]
	}
	var prev rune
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			prev = 0
			continue
		}
		if len(current) > 0 {
			nextStartsWord := unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev))
			if nextStartsWord {
				flush()
			}
		}
		current = append(current, r)
		prev = r
	}
	flush()
	return out
}

func filterEmptyStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func genericTestAnchorTerm(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "a", "an", "and", "are", "behavior", "behaviour", "case", "cases",
		"cover", "covers", "covered", "coverage", "does", "for", "how", "in",
		"protect", "protects", "protected", "regression", "test", "tests",
		"testing", "the", "to", "what", "when", "where", "which", "with":
		return true
	default:
		return false
	}
}

func bodyHitCap(role string, sourceFile bool, term string) int {
	if sourceFile {
		return 10
	}
	if strings.ContainsAny(term, "_.-") {
		return 5
	}
	switch role {
	case "test_case":
		return 4
	case "code_comment":
		return 3
	case "plan", "agent_note", "prd":
		return 3
	case "adr", "rfc", "openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		return 5
	default:
		return 4
	}
}

func isBroadMarkdownRole(role string) bool {
	switch role {
	case "plan", "agent_note", "prd":
		return true
	default:
		return false
	}
}

func sameFamilyNeedsCoreEvidence(role string) bool {
	switch role {
	case "plan", "agent_note", "prd", "rfc":
		return true
	default:
		return false
	}
}

func hasExplicitSourceIntent(queryLower string) bool {
	if containsAny(queryLower, "source file", "source code", "code path", "handler file") {
		return true
	}
	return hasQueryWord(queryLower, "source") || hasQueryWord(queryLower, "file") || hasQueryWord(queryLower, "handler")
}

func hasTestBehaviorIntent(queryLower string) bool {
	if containsAny(queryLower,
		"test", "tests", "testing", "test case", "test cases",
		"behavior", "behaviour", "expected behavior", "edge case", "edge cases",
		"regression", "regressions", "assert", "assertion", "assertions",
		"covered by", "coverage", "protected by",
	) {
		return true
	}
	if !containsAny(queryLower, "cover", "covers", "covered", "protect", "protects", "protected") {
		return false
	}
	return containsAny(queryLower,
		"validation", "retry", "retries", "idempotent", "idempotency",
		"auth", "permission", "permissions", "billing", "analytics",
		"bug", "error", "exception", "failure",
	)
}

func hasCodeCommentIntent(queryLower string) bool {
	if containsAny(queryLower,
		"code comment", "code comments", "comment says", "comments say",
		"implementation rationale", "implementation reason", "why does the code",
		"why is this implemented", "why is it implemented", "source rationale",
	) {
		return true
	}
	for _, word := range []string{"invariant", "invariants", "assumption", "assumptions", "workaround", "workarounds", "todo", "fixme", "hack"} {
		if hasQueryWord(queryLower, word) {
			return true
		}
	}
	if containsAny(queryLower, "comment", "comments", "rationale", "why", "implementation", "source") {
		for _, word := range []string{"constraint", "constraints", "compatibility", "legacy", "temporary"} {
			if hasQueryWord(queryLower, word) {
				return true
			}
		}
	}
	return false
}

func hasProductBackgroundIntent(queryLower string) bool {
	return hasQueryWord(queryLower, "prd") ||
		hasQueryWord(queryLower, "product") ||
		hasQueryWord(queryLower, "background") ||
		hasQueryWord(queryLower, "requirements") ||
		containsAny(queryLower, "user outcome", "user story", "customer access", "product requirement")
}

func hasUnrequestedProductSurface(pathLower, titleLower, queryLower string) bool {
	pathTitle := pathLower + " " + titleLower
	for _, surface := range []string{
		"admin", "auth", "cookie", "override", "overrides", "portal", "analytics", "dashboard",
		"support", "invoice", "invoices", "search", "pricing", "packaging",
		"observability", "runbook", "session", "token",
	} {
		if containsPathTitleToken(pathTitle, surface) && !strings.Contains(queryLower, surface) {
			return true
		}
	}
	return false
}

func hasUnrequestedContextSurface(pathLower, titleLower, queryLower string) bool {
	pathTitle := pathLower + " " + titleLower
	for _, surface := range []string{
		"admin", "override", "overrides", "portal", "dashboard", "observability", "support",
	} {
		if containsPathTitleToken(pathTitle, surface) && !strings.Contains(queryLower, surface) {
			return true
		}
	}
	return false
}

func containsPathTitleToken(s, token string) bool {
	for _, part := range splitIdentifierLikeText(s) {
		if part == token {
			return true
		}
	}
	return false
}

func splitIdentifierLikeText(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
}

func isDurableBackgroundDecision(c Candidate, pathLower, titleLower, bodyLower string, profile matchProfile) bool {
	if fileRole(c.Path) != "adr" || profile.coreMatches < 2 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(c.Status))
	if status != "" && status != "accepted" && !strings.Contains(bodyLower, "status: accepted") {
		return false
	}
	pathTitle := pathLower + " " + titleLower
	if containsAny(pathTitle, "source", "boundary") {
		return true
	}
	return containsAny(bodyLower,
		"authoritative",
		"idempotency boundary",
		"consistency boundary",
	)
}

func hasPlanIntent(queryLower string) bool {
	return hasQueryWord(queryLower, "plan") ||
		hasQueryWord(queryLower, "plans") ||
		hasQueryWord(queryLower, "resume") ||
		hasQueryWord(queryLower, "continue")
}

func hasRFCIntent(queryLower string) bool {
	return hasQueryWord(queryLower, "rfc") ||
		containsAny(queryLower, "request for comments", "proposal", "alternatives")
}

func hasOpenSpecStructureIntent(queryLower string) bool {
	return hasQueryWord(queryLower, "openspec") ||
		hasQueryWord(queryLower, "bundle") ||
		hasQueryWord(queryLower, "bundles") ||
		hasQueryWord(queryLower, "change") ||
		hasQueryWord(queryLower, "changes") ||
		hasQueryWord(queryLower, "collection") ||
		hasQueryWord(queryLower, "collections")
}

func shouldIncludeOpenSpecParent(queryLower string) bool {
	return hasOpenSpecStructureIntent(queryLower)
}

func hasOpenSpecChildRoleIntent(queryLower string) bool {
	return containsAny(queryLower,
		"task",
		"tasks",
		"todo",
		"implement",
		"implementation",
		"agent context",
		"resume",
		"continue",
		"spec",
		"delta",
		"requirement",
		"requirements",
		"acceptance",
		"boundary",
	)
}

func nonIntentCandidateMode(c Candidate) string {
	for _, key := range []string{"classifier_mode", "mode"} {
		if c.Metadata != nil {
			switch strings.ToLower(strings.TrimSpace(c.Metadata[key])) {
			case "protocol", "model", "template", "trace":
				return strings.ToLower(strings.TrimSpace(c.Metadata[key]))
			}
		}
	}
	if isAgentInstructionPath(c.Path) {
		return "protocol"
	}
	switch strings.ToLower(strings.TrimSpace(c.Subtype)) {
	case "agent_instruction", "skill", "maintainer_policy", "ownership_policy",
		"governance_policy", "contribution_policy", "security_policy", "procedure",
		"runbook", "standard":
		return "protocol"
	case "api_contract", "schema_model", "configuration", "workflow_definition":
		return "model"
	case "document_template", "prompt_template", "issue_template", "pull_request_template":
		return "template"
	default:
		return ""
	}
}

func hasRepositoryInstructionIntent(queryLower string) bool {
	return containsAny(queryLower,
		"repository instructions",
		"repository guidance",
		"repo instructions",
		"repo guidance",
		"project instructions",
		"project guidance",
		"project-wide",
		"project wide",
		"global instructions",
		"development guidance",
	) || (containsAny(queryLower, "claude", "codex", "agent instructions") &&
		containsAny(queryLower, "repository", "repo", "project"))
}

func hasNonIntentModeIntent(queryLower, mode string) bool {
	switch mode {
	case "protocol":
		return containsAny(queryLower,
			"instruction", "instructions", "rule", "rules", "policy", "policies",
			"procedure", "procedures", "runbook", "runbooks", "playbook", "skill",
			"skills", "standard", "standards", "convention", "conventions",
			"agent", "agents", "claude", "codex", "maintainer", "maintainers", "codeowners",
			"security policy", "contributing",
		)
	case "model":
		return containsAny(queryLower,
			"schema", "model", "contract", "openapi", "swagger", "graphql",
			"configuration", "config", "manifest", "workflow", "terraform",
			"docker", "compose", "yaml", "json",
		)
	case "template":
		return containsAny(queryLower,
			"template", "templates", "scaffold", "scaffolding", "boilerplate",
			"issue template", "pull request template", "prompt template",
		)
	case "trace":
		return containsAny(queryLower, "trace", "commit", "pull request", "issue", "transcript", "log")
	default:
		return false
	}
}

func hasLifecycleIntent(queryLower string) bool {
	if containsAny(queryLower, "stale", "superseded", "abandoned", "deprecated", "history", "historical", "why not") {
		return true
	}
	return containsAny(queryLower, "local entitlement", "local entitlements") &&
		(containsAny(queryLower, "cache", "caching") || hasQueryWord(queryLower, "local"))
}

func candidateIsStale(c Candidate, pathLower, bodyLower string) bool {
	status := strings.ToLower(strings.TrimSpace(c.Status))
	return status == "stale" ||
		status == "superseded" ||
		strings.Contains(pathLower, "superseded") ||
		strings.Contains(bodyLower, "status: superseded") ||
		strings.Contains(bodyLower, "status: stale")
}

func fileRole(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	switch {
	case isOpenSpecPath(path) && (path == "openspec" || strings.HasSuffix(path, "/openspec")):
		return "openspec_collection"
	case isOpenSpecChangePath(path) && !strings.HasSuffix(path, ".md"):
		return "openspec_bundle"
	case isOpenSpecPath(path) && strings.Contains(path, "/specs/") && strings.HasSuffix(path, "/spec.md"):
		return "openspec_spec"
	case isOpenSpecPath(path) && strings.HasSuffix(path, "/design.md"):
		return "openspec_design"
	case isOpenSpecPath(path) && strings.HasSuffix(path, "/tasks.md"):
		return "openspec_tasks"
	case isOpenSpecPath(path) && strings.HasSuffix(path, "/proposal.md"):
		return "openspec_proposal"
	case strings.HasPrefix(path, "docs/adr/") || strings.HasPrefix(path, "docs/adrs/"):
		return "adr"
	case strings.HasPrefix(path, "docs/prd/") || strings.Contains(path, "/prd/"):
		return "prd"
	case strings.HasPrefix(path, "docs/prds/") || strings.Contains(path, "/prds/"):
		return "prd"
	case strings.HasPrefix(path, "docs/product-specs/") || strings.Contains(path, "/docs/product-specs/") || strings.Contains(path, "/product-specs/"):
		return "prd"
	case isRequirementDocPath(path):
		return "prd"
	case strings.HasPrefix(path, "docs/rfcs/") || strings.HasPrefix(path, "docs/rfc/") || strings.HasPrefix(path, "rfcs/") || strings.HasPrefix(path, "rfc/") || strings.Contains(path, "/docs/rfcs/") || strings.Contains(path, "/docs/rfc/") || strings.Contains(path, "/rfcs/") || strings.Contains(path, "/rfc/"):
		return "rfc"
	case strings.Contains(path, "/plans/") || strings.HasPrefix(path, "plans/"):
		return "plan"
	case isAgentInstructionPath(path):
		return "agent_instruction"
	case strings.HasPrefix(path, ".cursor/") || strings.HasPrefix(path, ".claude/") || strings.HasPrefix(path, ".codex/"):
		return "agent_note"
	default:
		return ""
	}
}

func candidateRole(c Candidate) string {
	pathRole := fileRole(c.Path)
	if strings.HasPrefix(pathRole, "openspec_") {
		return pathRole
	}
	if role := kindSubtypeRole(c); role == "test_case" || role == "code_comment" {
		return role
	}
	if role := classifierRole(c); role != "" {
		return role
	}
	if role := kindSubtypeRole(c); role != "" {
		return role
	}
	return pathRole
}

func classifierRole(c Candidate) string {
	if c.Metadata == nil {
		return ""
	}
	model := strings.ToLower(strings.TrimSpace(c.Metadata["classifier_model"]))
	subtype := strings.ToLower(strings.TrimSpace(c.Metadata["classifier_subtype"]))
	family := strings.ToLower(strings.TrimSpace(c.Metadata["classifier_family"]))
	switch model {
	case "adr", "prd", "rfc", "plan", "agent_note":
		return model
	case "openspec":
		return ""
	case "protocol":
		switch subtype {
		case "agent_instruction":
			return "agent_instruction"
		case "skill":
			return "skill"
		default:
			return "protocol"
		}
	case "template":
		return "template"
	case "model":
		return "model"
	}
	if strings.HasPrefix(family, "protocol.agent_instruction") {
		return "agent_instruction"
	}
	if strings.HasPrefix(family, "protocol.skill") {
		return "skill"
	}
	if strings.HasPrefix(family, "template.") {
		return "template"
	}
	return ""
}

func kindSubtypeRole(c Candidate) string {
	kind := strings.ToLower(strings.TrimSpace(c.Kind))
	subtype := strings.ToLower(strings.TrimSpace(c.Subtype))
	switch {
	case kind == "source_context" && subtype == "test_case":
		return "test_case"
	case kind == "source_context" && subtype == "code_comment":
		return "code_comment"
	case kind == "decision" && subtype == "adr":
		return "adr"
	case kind == "requirements" && subtype == "prd":
		return "prd"
	case kind == "requirements":
		return "prd"
	case kind == "plan":
		return "plan"
	case kind == "design":
		return "design"
	case subtype == "agent_instruction":
		return "agent_instruction"
	case subtype == "skill":
		return "skill"
	case isProtocolSubtype(subtype):
		return "protocol"
	case isTemplateSubtype(subtype):
		return "template"
	case isModelSubtype(subtype):
		return "model"
	default:
		return ""
	}
}

func isTestCaseCandidate(c Candidate) bool {
	if strings.EqualFold(c.Subtype, "test_case") {
		return true
	}
	if c.Metadata == nil {
		return false
	}
	return strings.EqualFold(c.Metadata["source_type"], "test_case")
}

func isCodeCommentCandidate(c Candidate) bool {
	if strings.EqualFold(c.Subtype, "code_comment") {
		return true
	}
	if c.Metadata == nil {
		return false
	}
	return strings.EqualFold(c.Metadata["source_type"], "code_comment")
}

func isProtocolSubtype(subtype string) bool {
	switch subtype {
	case "maintainer_policy", "ownership_policy", "governance_policy",
		"contribution_policy", "security_policy", "procedure", "runbook", "standard":
		return true
	default:
		return false
	}
}

func isTemplateSubtype(subtype string) bool {
	switch subtype {
	case "document_template", "prompt_template", "issue_template", "pull_request_template":
		return true
	default:
		return false
	}
}

func isModelSubtype(subtype string) bool {
	switch subtype {
	case "api_contract", "schema_model", "configuration", "workflow_definition":
		return true
	default:
		return false
	}
}

func isAgentInstructionPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	switch base {
	case "claude.md", "agents.md", "agent.md", "gemini.md", "memento.md", ".cursorrules":
		return true
	}
	return strings.HasSuffix(path, ".agent.md") ||
		strings.HasSuffix(path, ".instructions.md") ||
		strings.Contains(path, "/agents/") ||
		strings.Contains(path, "/instructions/")
}

func isRequirementDocPath(path string) bool {
	path = strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/")
	base := filepath.Base(path)
	return strings.Contains(path, "/requirements/") ||
		strings.Contains(path, "/docs/requirements/") ||
		strings.HasPrefix(base, "req_") ||
		strings.HasPrefix(base, "req-") ||
		strings.HasSuffix(base, "_req.md") ||
		strings.HasSuffix(base, "-req.md")
}

func pathDepth(path string) int {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return 0
	}
	return strings.Count(path, "/")
}

func queryMentionsInstructionPathSubject(queryLower, pathLower string) bool {
	pathLower = strings.Trim(filepath.ToSlash(pathLower), "/")
	if pathLower == "" {
		return false
	}
	parts := strings.Split(pathLower, "/")
	if len(parts) <= 1 {
		return true
	}
	for _, part := range parts[:len(parts)-1] {
		for _, token := range splitIdentifierLikeText(part) {
			if len(token) >= 3 && strings.Contains(queryLower, token) {
				return true
			}
		}
	}
	return false
}

func isLikelyUnrequestedLocalizedPath(pathLower, queryLower string) bool {
	if containsAny(queryLower, "chinese", "japanese", "korean", "french", "german", "spanish", "portuguese", "russian") ||
		hasQueryWord(queryLower, "zh") ||
		hasQueryWord(queryLower, "ja") ||
		hasQueryWord(queryLower, "ko") ||
		hasQueryWord(queryLower, "fr") ||
		hasQueryWord(queryLower, "de") ||
		hasQueryWord(queryLower, "es") ||
		hasQueryWord(queryLower, "pt") ||
		hasQueryWord(queryLower, "ru") {
		return false
	}
	for _, marker := range []string{"/zh/", "/zh-cn/", "/zh-tw/", "/ja/", "/ko/", "/fr/", "/de/", "/es/", "/pt/", "/ru/"} {
		if strings.Contains(pathLower, marker) {
			return true
		}
	}
	return false
}

func isOpenSpecPath(path string) bool {
	path = strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/")
	if path == "openspec" || strings.HasPrefix(path, "openspec/") || strings.HasSuffix(path, "/openspec") {
		return true
	}
	return strings.Contains(path, "/openspec/")
}

func isOpenSpecChangePath(path string) bool {
	path = strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/")
	return isOpenSpecPath(path) && strings.Contains(path, "/changes/")
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func hasQueryWord(queryLower, word string) bool {
	for _, term := range tokenizePreservingIdentifiers(queryLower) {
		if term == word {
			return true
		}
	}
	return false
}

func hasIdentifierTerm(terms map[string]float64) bool {
	for term := range terms {
		if strings.ContainsAny(term, "_.-") || looksLikeCompactTestIdentifier(term) {
			return true
		}
	}
	return false
}

func hasSpecificTestNameIntent(queryLower string) bool {
	if !hasTestBehaviorIntent(queryLower) {
		return false
	}
	for _, term := range meaningfulTerms(queryLower) {
		if len(term) >= 9 && strings.HasPrefix(term, "test") {
			return true
		}
	}
	return false
}

func isRawTestSourceCandidate(c Candidate) bool {
	if isTestCaseCandidate(c) || isCodeCommentCandidate(c) || isMarkdownCandidatePath(c.Path) {
		return false
	}
	path := strings.Trim(strings.ToLower(filepath.ToSlash(c.Path)), "/")
	if path == "" {
		return false
	}
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php", ".java", ".kt", ".kts", ".rs":
	default:
		return false
	}
	name := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")):
		return true
	case ext == ".java" && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "it")):
		return true
	case (ext == ".kt" || ext == ".kts") && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "spec")):
		return true
	case ext == ".rs" && strings.HasSuffix(name, "_test"):
		return true
	}
	parts := strings.Split(path, "/")
	if len(parts) > 1 {
		parts = parts[:len(parts)-1]
	}
	for _, segment := range parts {
		switch segment {
		case "tests", "__tests__", "spec", "cypress", "e2e":
			return true
		}
	}
	return false
}

func lineScopedSourceFileKey(path string) string {
	path = strings.Trim(strings.ToLower(filepath.ToSlash(path)), "/")
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "#l"); idx >= 0 {
		path = path[:idx]
	}
	ext := filepath.Ext(path)
	if ext != "" {
		path = strings.TrimSuffix(path, ext)
	}
	return path
}

func expandedTerms(query string) map[string]float64 {
	terms := map[string]float64{}
	queryLower := strings.ToLower(query)
	for _, term := range meaningfulTerms(query) {
		terms[term] = 1.0
	}
	for _, roleTerm := range []string{"adr", "design", "plan", "prd", "proposal", "rfc", "spec"} {
		if _, ok := terms[roleTerm]; ok {
			terms[roleTerm] = 0.35
		}
	}
	for _, genericTerm := range []string{
		"acceptance", "criteria", "create", "creating", "document", "documents",
		"documentation", "generate", "generating", "instruction", "instructions",
		"requirement", "requirements", "user", "users",
		"behavior", "behaviour", "case", "cases", "cover", "covers", "covered",
		"coverage", "protect", "protects", "protected", "regression", "test",
		"tests", "testing",
	} {
		if _, ok := terms[genericTerm]; ok {
			terms[genericTerm] = 0.25
		}
	}
	add := func(term string, weight float64) {
		if current, ok := terms[term]; !ok || current < weight {
			terms[term] = weight
		}
	}
	if containsAny(queryLower, "product requirements document") ||
		(containsAny(queryLower, "product requirement", "product requirements") && containsAny(queryLower, "document", "doc", "spec", "specification")) {
		add("prd", 2.5)
		add("product", 0.8)
		add("requirements", 0.7)
	}
	if containsAny(queryLower, "architecture decision record") {
		add("adr", 2.5)
		add("decision", 0.8)
	}
	if containsAny(queryLower, "implementation plan", "migration plan", "rollout plan") {
		add("plan", 1.4)
	}
	if containsAny(queryLower, "design document", "technical design document", "architecture design document") {
		add("design", 1.2)
	}
	for term := range terms {
		if !strings.ContainsAny(term, "_.-") {
			continue
		}
		for _, part := range splitIdentifier(term) {
			if _, ok := terms[part]; ok {
				terms[part] = 0.35
			}
		}
	}
	has := func(term string) bool {
		_, ok := terms[term]
		return ok
	}
	if has("entitlement") && has("sync") {
		add("entitlement_sync", 2.0)
		add("harden-entitlement-sync", 2.0)
		add("billing-webhook-hardening", 1.5)
		add("entitlements", 1.2)
	}
	if has("webhook") && (has("replay") || has("protection")) {
		add("webhook_replay_protection", 2.0)
		add("stripe_event_id", 1.5)
		add("idempotency", 1.5)
		add("billing-webhook-hardening", 1.2)
	}
	if has("stripe_event_id") || (has("stripe") && has("event")) {
		add("stripe_event_id", 2.0)
		add("idempotency", 2.0)
		add("webhook", 1.2)
	}
	if has("local") && (has("entitlement") || has("entitlements")) {
		add("local entitlements", 2.0)
		add("local entitlement", 2.0)
		add("caching", 1.5)
		add("cache", 1.2)
		add("superseded", 1.8)
	}
	if has("harden") || has("hardening") {
		add("harden-entitlement-sync", 2.0)
		add("billing-webhook-hardening", 1.8)
	}
	return terms
}

func meaningfulTerms(query string) []string {
	raw := tokenizePreservingIdentifiers(query)
	stop := map[string]bool{
		"a": true, "an": true, "and": true, "all": true, "for": true, "give": true,
		"to": true, "the": true, "of": true, "in": true, "on": true, "with": true,
		"agent": true, "context": true, "resume": true, "continue": true, "find": true,
	}
	seen := map[string]bool{}
	var terms []string
	for _, t := range raw {
		if len(t) < 3 || stop[t] || seen[t] {
			continue
		}
		seen[t] = true
		terms = append(terms, t)
		for _, part := range splitIdentifier(t) {
			if len(part) >= 3 && !stop[part] && !seen[part] {
				seen[part] = true
				terms = append(terms, part)
			}
		}
	}
	return terms
}

func tokenizePreservingIdentifiers(s string) []string {
	var terms []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		terms = append(terms, strings.ToLower(b.String()))
		b.Reset()
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return terms
}

func splitIdentifier(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	if len(fields) <= 1 {
		return nil
	}
	return fields
}

func reasonsForCandidate(c Candidate, terms map[string]float64, queryLower string) []string {
	pathLower := strings.ToLower(c.Path)
	titleLower := strings.ToLower(c.Title)
	bodyLower := strings.ToLower(c.Body)
	var reasons []string
	if c.Metadata != nil && c.Metadata["retrieval_expansion_reason"] != "" {
		reasons = append(reasons, "relationship expansion: "+c.Metadata["retrieval_expansion_reason"])
	}
	if tier := CandidatePackTier(c); tier != PackTierPrimary {
		reason := strings.TrimSpace(c.Metadata["pack_tier_reason"])
		if reason != "" {
			reasons = append(reasons, "pack tier: "+tier+" ("+reason+")")
		} else {
			reasons = append(reasons, "pack tier: "+tier)
		}
	}
	if c.Metadata != nil && c.Metadata["concept_backfill_score"] != "" {
		var parts []string
		if glossary := metadataJSONList(c, "concept_glossary_matched_json"); len(glossary) > 0 {
			parts = append(parts, "glossary "+strings.Join(limitStrings(glossary, 3), ", "))
		}
		if compacts := metadataJSONList(c, "concept_backfill_matched_compacts_json"); len(compacts) > 0 {
			parts = append(parts, "compacts "+strings.Join(limitStrings(compacts, 3), ", "))
		}
		if phrases := metadataJSONList(c, "concept_backfill_matched_phrases_json"); len(phrases) > 0 {
			parts = append(parts, "phrases "+strings.Join(limitStrings(phrases, 3), ", "))
		}
		if pathTerms := metadataJSONList(c, "concept_backfill_matched_path_terms_json"); len(pathTerms) > 0 {
			parts = append(parts, "path terms "+strings.Join(limitStrings(pathTerms, 3), ", "))
		}
		if len(parts) == 0 {
			parts = append(parts, "score "+c.Metadata["concept_backfill_score"])
		}
		reasons = append(reasons, "concept backfill: "+strings.Join(parts, "; "))
	}
	if c.Metadata != nil && c.Metadata["section_pack_mode"] == "sections" {
		reasons = append(reasons, "section-packed context: "+strings.ReplaceAll(c.Metadata["section_pack_headings"], "\n", "; "))
	}
	if c.Metadata != nil && c.Metadata["indexed_section_retrieval_mode"] == "section_aware" {
		headings := metadataJSONList(c, "indexed_section_match_headings_json")
		ranges := metadataJSONList(c, "indexed_section_match_ranges_json")
		if len(headings) > 0 {
			parts := make([]string, 0, len(headings))
			for i, heading := range headings {
				part := heading
				if i < len(ranges) && ranges[i] != "" {
					part += " lines " + ranges[i]
				}
				parts = append(parts, part)
				if len(parts) >= 3 {
					break
				}
			}
			reasons = append(reasons, "indexed section match: "+strings.Join(parts, "; "))
		}
	}
	if c.Metadata != nil && c.Metadata["retrieval_evidence_mode"] == EvidenceModeBalanced {
		if evidenceReasons := strings.TrimSpace(c.Metadata["retrieval_evidence_reasons"]); evidenceReasons != "" {
			reasons = append(reasons, "balanced evidence: "+strings.ReplaceAll(evidenceReasons, "\n", "; "))
		}
	}
	if anchor := testNameAnchorScore(c, queryLower); anchor.score > 0 {
		reasons = append(reasons, anchor.reason)
	}
	for term := range terms {
		if term == "" {
			continue
		}
		switch {
		case strings.Contains(pathLower, term):
			reasons = append(reasons, "query term match in path: "+term)
		case strings.Contains(titleLower, term):
			reasons = append(reasons, "query term match in title: "+term)
		case strings.Contains(bodyLower, term):
			if strings.ContainsAny(term, "_.-") {
				reasons = append(reasons, "identifier/body match: "+term)
			} else {
				reasons = append(reasons, "query term match in body: "+term)
			}
		}
		if len(reasons) >= 3 {
			break
		}
	}
	role := candidateRole(c)
	switch role {
	case "adr":
		if containsAny(queryLower, "adr", "decision", "boundary", "why", "architecture", "rationale", "superseded", "stale") {
			reasons = append(reasons, "authority/query-intent signal: ADR")
		}
	case "prd":
		if containsAny(queryLower, "prd", "product", "background", "requirements", "user") {
			reasons = append(reasons, "authority/query-intent signal: PRD")
		}
	case "rfc":
		if containsAny(queryLower, "rfc", "request for comments", "proposal", "alternative", "alternatives", "motivation", "drawback", "drawbacks", "design") {
			reasons = append(reasons, "authority/query-intent signal: RFC")
		}
	case "design":
		if containsAny(queryLower, "architecture", "design", "technical", "system", "overview") {
			reasons = append(reasons, "design/query-intent signal")
		}
	case "openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		if containsAny(queryLower, "design", "context", "implement", "implementation", "agent context", "resume", "continue", "spec") {
			reasons = append(reasons, "OpenSpec change artifact candidate")
		}
	case "plan", "agent_note":
		if containsAny(queryLower, "plan", "resume", "continue", "notes", "followup", "follow-up", "agent") {
			reasons = append(reasons, "planning/query-intent signal")
		}
	case "agent_instruction", "protocol", "skill":
		if hasNonIntentModeIntent(queryLower, "protocol") {
			reasons = append(reasons, "protocol/query-intent signal")
		}
	case "template":
		if hasNonIntentModeIntent(queryLower, "template") {
			reasons = append(reasons, "template/query-intent signal")
		}
	case "test_case":
		if hasTestBehaviorIntent(queryLower) {
			reasons = append(reasons, "test-case behavior signal")
		}
	case "code_comment":
		if hasCodeCommentIntent(queryLower) {
			reasons = append(reasons, "code-comment rationale signal")
		}
	}
	if prior := authorityPrior(c, role, queryLower); prior.score != 0 {
		for _, reason := range prior.reasons {
			reasons = append(reasons, reason)
			if len(reasons) >= 5 {
				break
			}
		}
	}
	if strings.Contains(bodyLower, "status: superseded") || strings.Contains(bodyLower, "status: stale") || strings.Contains(pathLower, "superseded") {
		reasons = append(reasons, "lifecycle signal: superseded-or-stale")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "selected by retriever score")
	}
	return uniqueStrings(reasons)
}

func expandOpenSpecLinks(selected []Candidate, universe []Candidate, queryLower string, limit int) []Candidate {
	if len(selected) == 0 {
		return selected
	}
	if limit <= 0 {
		limit = len(selected)
	}
	max := limit + 3
	if max < len(selected) {
		max = len(selected)
	}
	byTarget := map[string]Candidate{}
	for _, c := range universe {
		if c.ID == "" {
			continue
		}
		byTarget["artifact:"+c.ID] = c
	}
	seen := map[string]bool{}
	out := make([]Candidate, 0, max)
	for _, c := range selected {
		key := candidateIdentity(c)
		seen[key] = true
		out = append(out, c)
	}
	addTarget := func(target, reason string) {
		if len(out) >= max {
			return
		}
		c, ok := byTarget[target]
		if !ok {
			return
		}
		key := candidateIdentity(c)
		if seen[key] {
			return
		}
		seen[key] = true
		if c.Metadata == nil {
			c.Metadata = map[string]string{}
		} else {
			c.Metadata = copyMetadata(c.Metadata)
		}
		c.Metadata["retrieval_expansion_reason"] = reason
		out = append(out, c)
	}

	for _, c := range selected {
		if c.Metadata == nil {
			continue
		}
		if shouldIncludeOpenSpecParent(queryLower) {
			for _, target := range metadataTargets(c.Metadata, "link_contained_by") {
				addTarget(target, "openspec_parent_bundle")
			}
		}
		if shouldExpandOpenSpecCompanions(queryLower) {
			for _, target := range metadataTargets(c.Metadata, "link_openspec_companion") {
				peer, ok := byTarget[target]
				if !ok || !wantedOpenSpecRole(peer, queryLower) {
					continue
				}
				addTarget(target, "openspec_companion")
			}
		}
		if containsAny(queryLower, "requirement", "requirements", "spec", "capability", "update", "delta") {
			for _, target := range metadataTargets(c.Metadata, "link_updates") {
				addTarget(target, "openspec_updated_capability")
			}
		}
		if fileRole(c.Path) == "openspec_bundle" || c.Metadata["artifact_scope"] == "bundle" {
			for _, target := range metadataTargets(c.Metadata, "link_contains") {
				child, ok := byTarget[target]
				if !ok || !wantedOpenSpecRole(child, queryLower) {
					continue
				}
				addTarget(target, "openspec_bundle_child")
			}
		}
	}
	return out
}

func metadataTargets(metadata map[string]string, key string) []string {
	value := strings.TrimSpace(metadata[key])
	if value == "" {
		return nil
	}
	var out []string
	for _, target := range strings.Split(value, "\n") {
		target = strings.TrimSpace(target)
		if target != "" {
			out = append(out, target)
		}
	}
	return out
}

func wantedOpenSpecRole(c Candidate, queryLower string) bool {
	role := ""
	if c.Metadata != nil {
		role = c.Metadata["openspec_role"]
	}
	if role == "" {
		role = fileRole(c.Path)
	}
	switch {
	case containsAny(queryLower, "task", "tasks", "todo", "resume", "continue"):
		return role == "tasks" || role == "openspec_tasks" ||
			role == "design" || role == "openspec_design" ||
			role == "proposal" || role == "openspec_proposal"
	case containsAny(queryLower, "design", "rationale", "why"):
		return role == "design" || role == "openspec_design" || role == "proposal" || role == "openspec_proposal"
	case containsAny(queryLower, "requirement", "requirements", "spec", "capability", "delta"):
		return role == "spec_delta" || role == "capability_spec" || role == "openspec_spec" || role == "proposal" || role == "openspec_proposal"
	default:
		return role == "proposal" || role == "design" || role == "tasks" ||
			role == "openspec_proposal" || role == "openspec_design" || role == "openspec_tasks"
	}
}

func shouldExpandOpenSpecCompanions(queryLower string) bool {
	return containsAny(queryLower,
		"agent context",
		"continue",
		"context",
		"implement",
		"implementation",
		"resume",
		"task",
		"tasks",
	)
}

func candidateIdentity(c Candidate) string {
	if c.ID != "" {
		return c.ID
	}
	return c.Path
}

func copyMetadata(metadata map[string]string) map[string]string {
	out := make(map[string]string, len(metadata)+1)
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func uniqueStrings(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}
