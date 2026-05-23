package retrieval

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type localGlossary struct {
	candidateCount int
	concepts       map[string]localGlossaryConcept
}

type localGlossaryConcept struct {
	key          string
	label        string
	documentDF   int
	evidence     map[string]int
	examplePath  string
	examplePaths []string
}

type localGlossaryMention struct {
	key      string
	label    string
	evidence string
	path     string
}

func buildLocalGlossary(candidates []Candidate) localGlossary {
	glossary := localGlossary{
		candidateCount: len(candidates),
		concepts:       map[string]localGlossaryConcept{},
	}
	for _, candidate := range candidates {
		mentions := localGlossaryMentions(candidate)
		seenInCandidate := map[string]bool{}
		for _, mention := range mentions {
			if mention.key == "" {
				continue
			}
			concept := glossary.concepts[mention.key]
			if concept.key == "" {
				concept = localGlossaryConcept{
					key:      mention.key,
					label:    mention.label,
					evidence: map[string]int{},
				}
			}
			if !seenInCandidate[mention.key] {
				concept.documentDF++
				seenInCandidate[mention.key] = true
			}
			concept.evidence[mention.evidence]++
			if concept.examplePath == "" {
				concept.examplePath = filepath.ToSlash(mention.path)
			}
			if len(concept.examplePaths) < 4 && mention.path != "" && !stringSliceContains(concept.examplePaths, filepath.ToSlash(mention.path)) {
				concept.examplePaths = append(concept.examplePaths, filepath.ToSlash(mention.path))
			}
			glossary.concepts[mention.key] = concept
		}
	}
	return glossary
}

func localGlossaryMentions(c Candidate) []localGlossaryMention {
	path := filepath.ToSlash(c.Path)
	var mentions []localGlossaryMention
	addText := func(text, evidence string) {
		mentions = append(mentions, localGlossaryPhraseMentions(text, evidence, path)...)
		mentions = append(mentions, localGlossaryCompactMentions(text, evidence, path)...)
	}
	pathBase := path
	if before, _, ok := strings.Cut(pathBase, "#"); ok {
		pathBase = before
	}
	pathBase = strings.TrimSuffix(pathBase, filepath.Ext(pathBase))
	for _, segment := range strings.Split(pathBase, "/") {
		addText(segment, "path")
	}
	addText(c.Title, "title")
	if c.Metadata != nil {
		for _, key := range []string{"test_name", "parent_title"} {
			if value := strings.TrimSpace(c.Metadata[key]); value != "" {
				addText(value, "test_name")
			}
		}
		for _, key := range []string{"openspec_change_id", "openspec_capability"} {
			if value := strings.TrimSpace(c.Metadata[key]); value != "" {
				addText(value, "metadata")
			}
		}
		for _, heading := range metadataJSONList(c, "indexed_section_match_headings_json") {
			addText(heading, "heading")
		}
	}
	for _, section := range c.Sections {
		if section.HeadingPath != "" {
			for _, part := range strings.Split(section.HeadingPath, ">") {
				addText(part, "heading")
			}
		}
		if section.Title != "" {
			addText(section.Title, "heading")
		}
	}
	return mentions
}

func localGlossaryPhraseMentions(text, evidence, path string) []localGlossaryMention {
	words := conceptWords(text)
	if len(words) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var mentions []localGlossaryMention
	add := func(words []string) {
		if len(words) == 0 || len(words) > 5 {
			return
		}
		phrase := strings.Join(words, " ")
		if phrase == "" || seen[phrase] {
			return
		}
		seen[phrase] = true
		mentions = append(mentions, localGlossaryMention{
			key:      localGlossaryPhraseKey(phrase),
			label:    phrase,
			evidence: evidence,
			path:     path,
		})
	}
	if len(words) == 1 {
		if len(words[0]) >= 6 {
			add(words)
		}
		return mentions
	}
	if len(words) <= 5 {
		add(words)
	}
	for n := 4; n >= 2; n-- {
		if len(words) < n {
			continue
		}
		for i := 0; i+n <= len(words); i++ {
			add(words[i : i+n])
		}
	}
	return mentions
}

func localGlossaryCompactMentions(text, evidence, path string) []localGlossaryMention {
	compacts := conceptCompactsFromText(text)
	mentions := make([]localGlossaryMention, 0, len(compacts))
	for _, compact := range compacts {
		if compact == "" {
			continue
		}
		mentions = append(mentions, localGlossaryMention{
			key:      localGlossaryCompactKey(compact),
			label:    compact,
			evidence: evidence,
			path:     path,
		})
	}
	return mentions
}

func applyGlossaryToConceptProfile(profile conceptQueryProfile, glossary localGlossary) conceptQueryProfile {
	allowedWords := map[string]bool{}
	var compacts []string
	for _, compact := range profile.compacts {
		if concept, ok := glossary.supportedCompact(compact); ok {
			compacts = append(compacts, compact)
			for _, word := range splitIdentifierParts(concept.label) {
				allowedWords[word] = true
			}
			for _, word := range splitIdentifierParts(compact) {
				allowedWords[word] = true
			}
		}
	}
	var phrases []string
	for _, phrase := range profile.phrases {
		if concept, ok := glossary.supportedPhrase(phrase); ok {
			phrases = append(phrases, phrase)
			for _, word := range splitIdentifierParts(concept.label) {
				allowedWords[word] = true
			}
			for _, word := range splitIdentifierParts(phrase) {
				allowedWords[word] = true
			}
		}
	}
	var words []string
	for _, word := range profile.words {
		if allowedWords[word] {
			words = append(words, word)
		}
	}
	profile.compacts = uniqueStrings(compacts)
	profile.phrases = uniqueStrings(phrases)
	profile.words = uniqueStrings(words)
	return profile
}

func annotateConceptRankWithGlossary(rank ConceptRank, glossary localGlossary) ConceptRank {
	var concepts []localGlossaryConcept
	for _, compact := range rank.MatchedCompacts {
		if concept, ok := glossary.supportedCompact(compact); ok {
			concepts = append(concepts, concept)
		}
	}
	for _, phrase := range rank.MatchedPhrases {
		if concept, ok := glossary.supportedPhrase(phrase); ok {
			concepts = append(concepts, concept)
		}
	}
	if len(concepts) == 0 {
		return rank
	}
	seen := map[string]bool{}
	for _, concept := range concepts {
		if seen[concept.key] {
			continue
		}
		seen[concept.key] = true
		rank.GlossaryMatches = append(rank.GlossaryMatches, concept.label)
		rank.GlossaryEvidence = append(rank.GlossaryEvidence, concept.evidenceSummary())
	}
	rank.GlossaryMatches = uniqueStrings(rank.GlossaryMatches)
	rank.GlossaryEvidence = uniqueStrings(rank.GlossaryEvidence)
	return rank
}

func filterGlossaryRanks(ranks []ConceptRank) []ConceptRank {
	out := ranks[:0]
	for _, rank := range ranks {
		if len(rank.GlossaryMatches) == 0 {
			continue
		}
		out = append(out, rank)
	}
	return out
}

func (g localGlossary) supportedCompact(compact string) (localGlossaryConcept, bool) {
	for _, alt := range conceptCompactAlternates(compact) {
		if concept, ok := g.concepts[localGlossaryCompactKey(alt)]; ok && g.supportsConcept(concept) {
			return concept, true
		}
		if concept, ok := g.concepts[localGlossaryPhraseKey(strings.Join(splitIdentifierParts(alt), " "))]; ok && g.supportsConcept(concept) {
			return concept, true
		}
	}
	return localGlossaryConcept{}, false
}

func (g localGlossary) supportedPhrase(phrase string) (localGlossaryConcept, bool) {
	key := localGlossaryPhraseKey(phrase)
	concept, ok := g.concepts[key]
	if !ok || !g.supportsConcept(concept) {
		return localGlossaryConcept{}, false
	}
	return concept, true
}

func (g localGlossary) supportsConcept(concept localGlossaryConcept) bool {
	if concept.key == "" || concept.documentDF <= 0 {
		return false
	}
	if noisyConceptDF(concept.documentDF, g.candidateCount) {
		return false
	}
	if concept.evidence["test_name"] > 0 && concept.documentDF <= localGlossaryDFLimit(g.candidateCount, 0.08, 8) {
		return true
	}
	if concept.evidence["path"] > 0 || concept.evidence["title"] > 0 || concept.evidence["heading"] > 0 || concept.evidence["metadata"] > 0 {
		return concept.documentDF <= localGlossaryDFLimit(g.candidateCount, 0.10, 10)
	}
	return localGlossaryEvidenceKindCount(concept.evidence) >= 2 && concept.documentDF <= localGlossaryDFLimit(g.candidateCount, 0.12, 12)
}

func localGlossaryDFLimit(total int, ratio float64, floor int) int {
	limit := int(float64(total) * ratio)
	if limit < floor {
		limit = floor
	}
	if limit < 1 {
		limit = 1
	}
	return limit
}

func localGlossaryEvidenceKindCount(evidence map[string]int) int {
	count := 0
	for _, n := range evidence {
		if n > 0 {
			count++
		}
	}
	return count
}

func (c localGlossaryConcept) evidenceSummary() string {
	keys := make([]string, 0, len(c.evidence))
	for key, count := range c.evidence {
		if count <= 0 {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+1)
	parts = append(parts, "df="+strconv.Itoa(c.documentDF))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Itoa(c.evidence[key]))
	}
	return c.label + " (" + strings.Join(parts, ", ") + ")"
}

func localGlossaryPhraseKey(phrase string) string {
	words := conceptWords(phrase)
	if len(words) == 0 {
		return ""
	}
	return "phrase:" + strings.Join(words, " ")
}

func localGlossaryCompactKey(compact string) string {
	compact = strings.Join(splitIdentifierParts(compact), "")
	if compact == "" {
		return ""
	}
	return "compact:" + compact
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
