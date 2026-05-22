package classify

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ClassifyCandidate evaluates candidate against the declarative document-model
// profile and returns the resolver decision. The evaluator is intentionally
// generic: model behavior comes from PipelineConfig evidence rules, not
// per-document-type Go classifiers.
func ClassifyCandidate(candidate Candidate, cfg PipelineConfig) Resolution {
	candidate = prepareCandidate(candidate)

	var classifications []Classification
	for _, id := range sortedModelIDs(cfg.Models) {
		if cl, ok := evaluateModel(id, cfg.Models[id], candidate, cfg.Resolver); ok {
			classifications = append(classifications, cl)
		}
	}
	if cfg.LocalModels.Enabled {
		for _, local := range cfg.LocalModels.Definitions {
			if cl, ok := evaluateLocalModel(local, cfg, candidate); ok {
				classifications = append(classifications, cl)
			}
		}
	}

	return resolveClassifications(classifications, cfg.Resolver)
}

func prepareCandidate(candidate Candidate) Candidate {
	if featuresEmpty(candidate.Features) {
		candidate.Features = ExtractFeatures(candidate.Path, candidate.Body)
	}
	for i := range candidate.ChildCandidates {
		if candidate.ChildCandidates[i].Scope == "" {
			candidate.ChildCandidates[i].Scope = ScopeDocument
		}
		if featuresEmpty(candidate.ChildCandidates[i].Features) {
			candidate.ChildCandidates[i] = EnrichCandidate(candidate.ChildCandidates[i])
		}
	}
	return candidate
}

func featuresEmpty(features Features) bool {
	return len(features.PathTokens) == 0 &&
		len(features.FilenameTokens) == 0 &&
		len(features.Frontmatter) == 0 &&
		features.Title == "" &&
		len(features.Headings) == 0 &&
		len(features.Sections) == 0 &&
		len(features.Markers) == 0
}

func evaluateModel(id string, model ModelConfig, candidate Candidate, resolver ResolverConfig) (Classification, bool) {
	if !model.Enabled || !hasScope(model, candidate.Scope) {
		return Classification{}, false
	}

	score := 0.0
	var positive []Reason
	var negative []Reason

	if len(model.PathHints) > 0 && matchesAnyGlob(model.PathHints, candidate.Path) {
		score += resolver.ConfiguredPathPrior
		positive = append(positive, Reason{
			Code:     ReasonPathHint,
			Polarity: ReasonPositive,
			Message:  "Candidate path matches a declarative model path hint.",
			Evidence: firstMatchingGlob(model.PathHints, candidate.Path),
		})
	}

	posScore, posReasons := evaluateRules(candidate, model, model.Evidence, ReasonPositive)
	negScore, negReasons := evaluateRules(candidate, model, model.NegativeEvidence, ReasonNegative)
	score += posScore
	score -= negScore
	positive = append(positive, posReasons...)
	negative = append(negative, negReasons...)

	subformat, subScore, subPositive, subNegative := bestSubmodel(candidate, model, model.Subformats, ReasonSubformatEvidence)
	family, familyScore, familyPositive, familyNegative := bestSubmodel(candidate, model, model.Families, ReasonFamilyEvidence)
	score += subScore + familyScore
	positive = append(positive, subPositive...)
	positive = append(positive, familyPositive...)
	negative = append(negative, subNegative...)
	negative = append(negative, familyNegative...)

	confidence := clamp01(score)
	classification := Classification{
		Classifier:      id,
		Scope:           candidate.Scope,
		Subformat:       qualifyChildModel(id, subformat),
		Family:          qualifyChildModel(id, family),
		Accepted:        confidence >= resolver.RejectBelow || model.Fallback,
		Confidence:      confidence,
		Mode:            model.Mode,
		Kind:            model.Kind,
		Subtype:         model.Subtype,
		Status:          deriveStatus(candidate.Features),
		Lifecycle:       deriveLifecycle(candidate.Features),
		Authority:       model.Authority,
		FormatProfile:   model.FormatProfile,
		PositiveReasons: positive,
		NegativeReasons: negative,
	}
	if model.EmitChildCandidates {
		classification.ChildCandidates = candidate.ChildCandidates
	}
	if candidate.Scope == ScopeContainer {
		classification.LayoutGroup = candidate.Path
	}
	return classification, true
}

func evaluateLocalModel(local LocalModelDefinition, cfg PipelineConfig, candidate Candidate) (Classification, bool) {
	base, ok := cfg.Models[local.BaseModel]
	if !ok {
		return Classification{}, false
	}
	model := base
	if local.Authority != "" {
		model.Authority = local.Authority
	}
	model.PathHints = append(append([]string{}, base.PathHints...), local.PathHints...)
	if len(local.RequiredHeadings) > 0 {
		model.Evidence = append(model.Evidence, EvidenceRule{
			ID:      local.ID + "_required_headings",
			Weight:  0.20,
			Reason:  ReasonLocalOverride,
			Message: "Local model required headings are present.",
			Match: EvidenceMatch{
				Scope:       ScopeDocument,
				HeadingsAll: local.RequiredHeadings,
			},
		})
	}
	if len(local.PositiveTerms) > 0 {
		model.Evidence = append(model.Evidence, EvidenceRule{
			ID:      local.ID + "_positive_terms",
			Weight:  0.12,
			Reason:  ReasonLocalOverride,
			Message: "Local model positive terms are present.",
			Match: EvidenceMatch{
				Scope:           ScopeDocument,
				BodyContainsAny: local.PositiveTerms,
			},
		})
	}
	if len(local.Evidence) > 0 {
		model.Evidence = append(model.Evidence, local.Evidence...)
	}
	if len(local.NegativeTerms) > 0 {
		model.NegativeEvidence = append(model.NegativeEvidence, EvidenceRule{
			ID:      local.ID + "_negative_terms",
			Weight:  0.12,
			Reason:  ReasonLocalOverride,
			Message: "Local model negative terms are present.",
			Match: EvidenceMatch{
				Scope:           ScopeDocument,
				BodyContainsAny: local.NegativeTerms,
			},
		})
	}
	if len(local.NegativeEvidence) > 0 {
		model.NegativeEvidence = append(model.NegativeEvidence, local.NegativeEvidence...)
	}
	return evaluateModel(local.ID, model, candidate, cfg.Resolver)
}

func evaluateRules(candidate Candidate, model ModelConfig, rules []EvidenceRule, polarity ReasonPolarity) (float64, []Reason) {
	score := 0.0
	reasons := make([]Reason, 0, len(rules))
	for _, rule := range rules {
		if rule.Weight <= 0 {
			continue
		}
		matched, evidence := matchRule(candidate, model, rule)
		if !matched {
			continue
		}
		score += rule.Weight
		reason := rule.Reason
		if reason == "" {
			reason = ReasonLocalOverride
		}
		message := rule.Message
		if message == "" {
			message = rule.ID
		}
		reasons = append(reasons, Reason{
			Code:     reason,
			Polarity: polarity,
			Message:  message,
			Evidence: evidence,
		})
	}
	return score, reasons
}

func bestSubmodel(candidate Candidate, model ModelConfig, submodels map[string]SubmodelConfig, defaultReason ReasonCode) (string, float64, []Reason, []Reason) {
	if len(submodels) == 0 {
		return "", 0, nil, nil
	}
	bestID := ""
	bestScore := 0.0
	var bestPositive []Reason
	var bestNegative []Reason
	for _, id := range sortedSubmodelIDs(submodels) {
		sub := submodels[id]
		if !sub.Enabled {
			continue
		}
		posScore, positive := evaluateRules(candidate, model, normalizeRuleReasons(sub.Evidence, defaultReason), ReasonPositive)
		negScore, negative := evaluateRules(candidate, model, sub.NegativeEvidence, ReasonNegative)
		score := posScore - negScore
		if score <= 0 {
			continue
		}
		if score > bestScore {
			bestID = id
			bestScore = score
			bestPositive = positive
			bestNegative = negative
		}
	}
	return bestID, bestScore, bestPositive, bestNegative
}

func normalizeRuleReasons(rules []EvidenceRule, defaultReason ReasonCode) []EvidenceRule {
	out := append([]EvidenceRule{}, rules...)
	for i := range out {
		if out[i].Reason == "" {
			out[i].Reason = defaultReason
		}
	}
	return out
}

func matchRule(candidate Candidate, model ModelConfig, rule EvidenceRule) (bool, string) {
	match := rule.Match
	hasPredicate := match.Always
	evidence := rule.ID

	if match.Scope != "" {
		hasPredicate = true
		if candidate.Scope != match.Scope {
			return false, ""
		}
		evidence = string(match.Scope)
	}
	if match.PathHints {
		hasPredicate = true
		glob := firstMatchingGlob(model.PathHints, candidate.Path)
		if glob == "" {
			return false, ""
		}
		evidence = glob
	}
	if len(match.PathGlobs) > 0 {
		hasPredicate = true
		glob := firstMatchingGlob(match.PathGlobs, candidate.Path)
		if glob == "" {
			return false, ""
		}
		evidence = glob
	}
	if len(match.PathContainsAny) > 0 {
		hasPredicate = true
		if hit := containsAnyTerm(normalizedPath(candidate.Path), match.PathContainsAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.PathSuffixesAny) > 0 {
		hasPredicate = true
		if hit := suffixAny(normalizedPath(candidate.Path), match.PathSuffixesAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.FilenameAny) > 0 {
		hasPredicate = true
		name := strings.ToLower(filepath.Base(normalizedPath(candidate.Path)))
		if hit := containsAnyTerm(name, match.FilenameAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.TitleAny) > 0 {
		hasPredicate = true
		if hit := containsAnyTerm(candidate.Features.Title, match.TitleAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.TitleAll) > 0 {
		hasPredicate = true
		if hit := containsAllTerms(candidate.Features.Title, match.TitleAll); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.FrontmatterExists) > 0 {
		hasPredicate = true
		if hit := frontmatterExists(candidate.Features.Frontmatter, match.FrontmatterExists); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.FrontmatterEquals) > 0 {
		hasPredicate = true
		if hit := frontmatterEquals(candidate.Features.Frontmatter, match.FrontmatterEquals); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.HeadingsAny) > 0 {
		hasPredicate = true
		if hit := headingsAny(candidate.Features.Headings, match.HeadingsAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.HeadingsAll) > 0 {
		hasPredicate = true
		if hit := headingsAll(candidate.Features.Headings, match.HeadingsAll); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.SectionRolesAny) > 0 {
		hasPredicate = true
		if hit := sectionRolesAny(candidate.Features.Sections, match.SectionRolesAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.SectionRolesAll) > 0 {
		hasPredicate = true
		if hit := sectionRolesAll(candidate.Features.Sections, match.SectionRolesAll); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if match.ChecklistMin > 0 {
		hasPredicate = true
		if candidate.Features.ChecklistItems < match.ChecklistMin {
			return false, ""
		}
		evidence = "checklist"
	}
	if match.DateTokensMin > 0 {
		hasPredicate = true
		if len(candidate.Features.DateTokens) < match.DateTokensMin {
			return false, ""
		}
		evidence = "date_token"
	}
	if len(match.MarkersAny) > 0 {
		hasPredicate = true
		if hit := listAny(candidate.Features.Markers, match.MarkersAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.IdentifiersAny) > 0 {
		hasPredicate = true
		if hit := listAny(candidate.Features.Identifiers, match.IdentifiersAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.LocalTermsAny) > 0 {
		hasPredicate = true
		if hit := listAny(candidate.Features.LocalTerms, match.LocalTermsAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.BodyContainsAny) > 0 {
		hasPredicate = true
		if hit := containsAnyTerm(candidate.Body, match.BodyContainsAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.BodyContainsAll) > 0 {
		hasPredicate = true
		if hit := containsAllTerms(candidate.Body, match.BodyContainsAll); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.BodyRegexAny) > 0 {
		hasPredicate = true
		if hit := regexAny(candidate.Body, match.BodyRegexAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.ChildRolesAny) > 0 {
		hasPredicate = true
		if hit := childRolesAny(candidate.ChildCandidates, match.ChildRolesAny); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}
	if len(match.ChildRolesAll) > 0 {
		hasPredicate = true
		if hit := childRolesAll(candidate.ChildCandidates, match.ChildRolesAll); hit == "" {
			return false, ""
		} else {
			evidence = hit
		}
	}

	return hasPredicate, evidence
}

func resolveClassifications(classifications []Classification, resolver ResolverConfig) Resolution {
	if len(classifications) == 0 {
		return Resolution{}
	}
	sort.SliceStable(classifications, func(i, j int) bool {
		if classifications[i].Confidence == classifications[j].Confidence {
			return classifications[i].Classifier < classifications[j].Classifier
		}
		return classifications[i].Confidence > classifications[j].Confidence
	})

	winner := classifications[0]
	var second *Classification
	if len(classifications) > 1 {
		second = &classifications[1]
	}

	ambiguous := winner.Confidence < resolver.WeakAccept
	if second != nil && winner.Confidence >= resolver.WeakAccept && winner.Confidence < resolver.StrongAccept {
		ambiguous = winner.Confidence-second.Confidence < resolver.AmbiguityGap
	}

	fallbackGeneric := false
	if ambiguous {
		if fallback, ok := findClassification(classifications, resolver.Fallback); ok {
			winner = fallback
			winner.Accepted = true
			fallbackGeneric = fallback.Classifier == ModelGenericMarkdown
		}
	}
	winner.Accepted = winner.Accepted || winner.Confidence >= resolver.RejectBelow

	alternatives := make([]Classification, 0, len(classifications)-1)
	for _, cl := range classifications {
		if cl.Classifier == winner.Classifier {
			continue
		}
		alternatives = append(alternatives, cl)
	}
	return Resolution{
		Winner:          winner,
		Alternatives:    alternatives,
		Ambiguous:       ambiguous,
		FallbackGeneric: fallbackGeneric,
	}
}

func deriveStatus(features Features) string {
	if status := frontmatterValue(features.Frontmatter, "status"); status != "" {
		return normalizePhrase(status)
	}
	for _, phrase := range features.StatusPhrases {
		phrase = strings.TrimPrefix(normalizePhrase(phrase), "status:")
		if phrase != "" {
			return phrase
		}
	}
	return ""
}

func deriveLifecycle(features Features) string {
	for _, marker := range []string{MarkerSuperseded, MarkerDeprecated, MarkerStale} {
		if listAny(features.Markers, []string{marker}) != "" {
			return marker
		}
	}
	for _, phrase := range features.LifecyclePhrases {
		phrase = normalizePhrase(phrase)
		switch phrase {
		case "superseded", "deprecated", "stale", "archived", "obsolete", "rejected", "legacy", "old":
			return phrase
		}
	}
	return deriveStatus(features)
}

func sortedModelIDs(models map[string]ModelConfig) []string {
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedSubmodelIDs(submodels map[string]SubmodelConfig) []string {
	ids := make([]string, 0, len(submodels))
	for id := range submodels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func qualifyChildModel(parent, child string) string {
	if child == "" || strings.Contains(child, ".") {
		return child
	}
	return parent + "." + child
}

func findClassification(classifications []Classification, id string) (Classification, bool) {
	for _, cl := range classifications {
		if cl.Classifier == id {
			return cl, true
		}
	}
	return Classification{}, false
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizedPath(path string) string {
	return strings.ToLower(filepath.ToSlash(path))
}

func matchesAnyGlob(patterns []string, path string) bool {
	return firstMatchingGlob(patterns, path) != ""
}

func firstMatchingGlob(patterns []string, path string) string {
	path = normalizedPath(path)
	for _, pattern := range patterns {
		pattern = strings.ToLower(filepath.ToSlash(pattern))
		if globMatches(pattern, path) {
			return pattern
		}
	}
	return ""
}

func globMatches(pattern, path string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	re := "^" + globPatternToRegex(pattern) + "$"
	ok, err := regexp.MatchString(re, path)
	return err == nil && ok
}

func globPatternToRegex(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	return b.String()
}

func containsAnyTerm(haystack string, terms []string) string {
	haystack = strings.ToLower(haystack)
	for _, term := range terms {
		needle := strings.ToLower(strings.TrimSpace(term))
		if needle != "" && strings.Contains(haystack, needle) {
			return term
		}
	}
	return ""
}

func containsAllTerms(haystack string, terms []string) string {
	haystack = strings.ToLower(haystack)
	var last string
	for _, term := range terms {
		needle := strings.ToLower(strings.TrimSpace(term))
		if needle == "" {
			continue
		}
		if !strings.Contains(haystack, needle) {
			return ""
		}
		last = term
	}
	return last
}

func suffixAny(value string, suffixes []string) string {
	value = strings.ToLower(value)
	for _, suffix := range suffixes {
		suffix = strings.ToLower(filepath.ToSlash(strings.TrimSpace(suffix)))
		if suffix != "" && strings.HasSuffix(value, suffix) {
			return suffix
		}
	}
	return ""
}

func frontmatterExists(frontmatter map[string]string, keys []string) string {
	var last string
	for _, key := range keys {
		if frontmatterValue(frontmatter, key) == "" {
			return ""
		}
		last = key
	}
	return last
}

func frontmatterEquals(frontmatter map[string]string, expected map[string]string) string {
	var last string
	for key, want := range expected {
		got := normalizePhrase(frontmatterValue(frontmatter, key))
		if got == "" || got != normalizePhrase(want) {
			return ""
		}
		last = key + ":" + want
	}
	return last
}

func frontmatterValue(frontmatter map[string]string, key string) string {
	want := strings.ToLower(strings.TrimSpace(key))
	for gotKey, value := range frontmatter {
		if strings.ToLower(strings.TrimSpace(gotKey)) == want {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func headingsAny(headings []Heading, terms []string) string {
	for _, heading := range headings {
		if hit := containsAnyTerm(heading.Text, terms); hit != "" {
			return hit
		}
	}
	return ""
}

func headingsAll(headings []Heading, terms []string) string {
	var last string
	for _, term := range terms {
		found := false
		for _, heading := range headings {
			if containsAnyTerm(heading.Text, []string{term}) != "" {
				found = true
				last = term
				break
			}
		}
		if !found {
			return ""
		}
	}
	return last
}

func sectionRolesAny(sections []Section, roles []string) string {
	for _, section := range sections {
		for _, role := range roles {
			if normalizePhrase(section.Role) == normalizePhrase(role) {
				return role
			}
		}
	}
	return ""
}

func sectionRolesAll(sections []Section, roles []string) string {
	var last string
	for _, role := range roles {
		if sectionRolesAny(sections, []string{role}) == "" {
			return ""
		}
		last = role
	}
	return last
}

func listAny(items []string, terms []string) string {
	for _, item := range items {
		for _, term := range terms {
			if normalizePhrase(item) == normalizePhrase(term) {
				return term
			}
		}
	}
	return ""
}

func regexAny(body string, patterns []string) string {
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(body) {
			return pattern
		}
	}
	return ""
}

func childRolesAny(children []Candidate, roles []string) string {
	for _, child := range children {
		for _, role := range roles {
			if normalizePhrase(child.Role) == normalizePhrase(role) {
				return role
			}
		}
	}
	return ""
}

func childRolesAll(children []Candidate, roles []string) string {
	var last string
	for _, role := range roles {
		if childRolesAny(children, []string{role}) == "" {
			return ""
		}
		last = role
	}
	return last
}
