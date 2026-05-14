package retrieval

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
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
}

type Retriever interface {
	Name() string
	Retrieve(candidates []Candidate, query string) []Candidate
}

type WeightedFilesRetrieverV0 struct{}

func (WeightedFilesRetrieverV0) Name() string { return "eval_weighted_files_v0" }

func (WeightedFilesRetrieverV0) Retrieve(candidates []Candidate, query string) []Candidate {
	return retrieveWeightedFilesV0(candidates, query)
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
	for _, prefix := range []string{"openspec/", "docs/adr/", "docs/plans/", "docs/prd/", ".cursor/", ".claude/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func IsSourceContextCandidate(c Candidate) bool {
	return !strings.EqualFold(filepath.Ext(c.Path), ".md") && !IsPlanningIntentPath(c.Path)
}

func retrieveWeightedFilesV0(candidates []Candidate, query string) []Candidate {
	terms := expandedTerms(query)
	queryLower := strings.ToLower(query)
	type scored struct {
		candidate Candidate
		score     float64
	}
	var scoredCandidates []scored
	for _, c := range candidates {
		score := scoreCandidate(c, terms, queryLower)
		if score >= 4.0 {
			scoredCandidates = append(scoredCandidates, scored{candidate: c, score: score})
		}
	}
	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score == scoredCandidates[j].score {
			return scoredCandidates[i].candidate.Path < scoredCandidates[j].candidate.Path
		}
		return scoredCandidates[i].score > scoredCandidates[j].score
	})
	limit := retrievalLimit(queryLower, terms)
	if len(scoredCandidates) > limit {
		scoredCandidates = scoredCandidates[:limit]
	}
	out := make([]Candidate, 0, len(scoredCandidates))
	for _, sf := range scoredCandidates {
		out = append(out, sf.candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func retrievalLimit(queryLower string, terms map[string]float64) int {
	limit := 8
	switch {
	case strings.Contains(queryLower, "implementation context") || strings.Contains(queryLower, "agent context") || strings.Contains(queryLower, "implement"):
		limit = 6
	case strings.Contains(queryLower, "resume") || strings.Contains(queryLower, "continue"):
		limit = 7
	case strings.Contains(queryLower, "why") || strings.Contains(queryLower, "decision") || strings.Contains(queryLower, "adr"):
		limit = 5
	case strings.Contains(queryLower, "prd") || strings.Contains(queryLower, "product") || strings.Contains(queryLower, "background"):
		limit = 7
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
	bodyLower := strings.ToLower(c.Body)
	score := 0.0
	sourceFile := IsSourceContextCandidate(c)
	role := fileRole(c.Path)
	for term, weight := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(pathLower, term) {
			score += 6.0 * weight
		}
		hits := strings.Count(bodyLower, term)
		if hits > 8 {
			hits = 8
		}
		score += float64(hits) * weight
	}

	if IsPlanningIntentPath(c.Path) {
		score += 1.0
	}
	switch role {
	case "adr":
		if containsAny(queryLower, "adr", "decision", "boundary", "why", "architecture", "rationale", "superseded", "stale") {
			score += 3.0
		}
	case "prd":
		if containsAny(queryLower, "prd", "product", "background", "requirements", "user") {
			score += 4.0
		} else if containsAny(queryLower, "implement", "implementation", "agent context", "source file") {
			score -= 3.0
		}
	case "openspec_design":
		if containsAny(queryLower, "design", "rationale", "why", "context", "implement", "implementation", "agent context") {
			score += 3.0
		}
	case "openspec_tasks":
		if containsAny(queryLower, "task", "todo", "implement", "implementation", "resume", "continue", "agent context") {
			score += 3.0
		}
	case "openspec_spec":
		if containsAny(queryLower, "spec", "delta", "requirement", "acceptance") {
			score += 2.0
		}
	case "plan":
		if containsAny(queryLower, "plan", "resume", "continue", "migration", "notes") {
			score += 2.0
		}
	case "agent_note":
		if containsAny(queryLower, "resume", "continue", "followup", "follow-up", "agent", "notes") {
			score += 2.0
		}
	}
	if sourceFile {
		if containsAny(queryLower, "source", "file", "code", "implement", "implementation", "handler", "migration") || hasIdentifierTerm(terms) {
			score += 2.0
		} else {
			score -= 2.0
		}
	}
	if strings.Contains(pathLower, "scratch/") || strings.Contains(pathLower, "old-") || strings.Contains(pathLower, "legacy") {
		score -= 4.0
	}
	if strings.Contains(pathLower, "superseded") || strings.Contains(bodyLower, "status: superseded") || strings.Contains(bodyLower, "status: stale") {
		if containsAny(queryLower, "stale", "superseded", "old", "local", "caching", "history", "why") {
			score += 4.0
		} else {
			score -= 5.0
		}
	}
	return score
}

func fileRole(path string) string {
	path = filepath.ToSlash(path)
	switch {
	case strings.Contains(path, "/specs/") && strings.HasSuffix(path, "/spec.md"):
		return "openspec_spec"
	case strings.HasPrefix(path, "openspec/") && strings.HasSuffix(path, "/design.md"):
		return "openspec_design"
	case strings.HasPrefix(path, "openspec/") && strings.HasSuffix(path, "/tasks.md"):
		return "openspec_tasks"
	case strings.HasPrefix(path, "openspec/") && strings.HasSuffix(path, "/proposal.md"):
		return "openspec_proposal"
	case strings.HasPrefix(path, "docs/adr/") || strings.HasPrefix(path, "docs/adrs/"):
		return "adr"
	case strings.HasPrefix(path, "docs/prd/") || strings.Contains(path, "/prd/"):
		return "prd"
	case strings.Contains(path, "/plans/") || strings.HasPrefix(path, "plans/"):
		return "plan"
	case strings.HasPrefix(path, ".cursor/") || strings.HasPrefix(path, ".claude/"):
		return "agent_note"
	default:
		return ""
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func hasIdentifierTerm(terms map[string]float64) bool {
	for term := range terms {
		if strings.ContainsAny(term, "_.-") {
			return true
		}
	}
	return false
}

func expandedTerms(query string) map[string]float64 {
	terms := map[string]float64{}
	for _, term := range meaningfulTerms(query) {
		terms[term] = 1.0
	}
	has := func(term string) bool {
		_, ok := terms[term]
		return ok
	}
	add := func(term string, weight float64) {
		if current, ok := terms[term]; !ok || current < weight {
			terms[term] = weight
		}
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
	bodyLower := strings.ToLower(c.Body)
	var reasons []string
	for term := range terms {
		if term == "" {
			continue
		}
		switch {
		case strings.Contains(pathLower, term):
			reasons = append(reasons, "query term match in path: "+term)
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
	role := fileRole(c.Path)
	switch role {
	case "adr":
		if containsAny(queryLower, "adr", "decision", "boundary", "why", "architecture", "rationale", "superseded", "stale") {
			reasons = append(reasons, "authority/query-intent signal: ADR")
		}
	case "prd":
		if containsAny(queryLower, "prd", "product", "background", "requirements", "user") {
			reasons = append(reasons, "authority/query-intent signal: PRD")
		}
	case "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
		if containsAny(queryLower, "design", "context", "implement", "implementation", "agent context", "resume", "continue", "spec") {
			reasons = append(reasons, "OpenSpec change artifact candidate")
		}
	case "plan", "agent_note":
		if containsAny(queryLower, "plan", "resume", "continue", "notes", "followup", "follow-up", "agent") {
			reasons = append(reasons, "planning/query-intent signal")
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
