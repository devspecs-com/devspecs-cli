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
	out = expandOpenSpecLinks(out, candidates, queryLower, limit)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func retrievalLimit(queryLower string, terms map[string]float64) int {
	limit := 8
	switch {
	case hasExplicitSourceIntent(queryLower):
		limit = 5
	case containsAny(queryLower, "rfc", "request for comments", "proposal", "alternatives"):
		limit = 5
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
	role := fileRole(c.Path)
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
	case "openspec_bundle":
		if containsAny(queryLower, "proposal", "design", "task", "tasks", "spec", "implement", "implementation", "agent context", "resume", "continue") {
			score += 3.0
		}
		if profile.identifierMatches > 0 {
			score += 4.0
		}
	case "openspec_design":
		if containsAny(queryLower, "design", "rationale", "why", "context", "implement", "implementation", "agent context") {
			score += 3.0
		}
		if containsAny(queryLower, "implement", "implementation", "agent context") {
			score += 6.0
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
		if profile.identifierMatches > 0 {
			score += 4.0
		}
	case "openspec_spec":
		if containsAny(queryLower, "spec", "delta", "requirement", "acceptance") {
			score += 2.0
		} else if containsAny(queryLower, "implement", "implementation", "agent context") {
			score -= 1.5
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
	}
	if sourceFile {
		if explicitSourceIntent || containsAny(queryLower, "implement", "implementation", "handler") || hasIdentifierTerm(terms) {
			score += 2.0
		} else if profile.pathTitleCoreMatches >= 2 {
			score += 2.0
		} else {
			score -= 2.0
		}
		if containsAny(queryLower, "boundary") && profile.pathTitleCoreMatches >= 2 {
			score += 3.0
		}
		if hasQueryWord(queryLower, "session") && strings.Contains(pathLower, "/session.") {
			score += 5.0
		}
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
			score -= 6.0
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
	if strings.Contains(pathLower, "scratch/") || strings.Contains(pathLower, "old-") || strings.Contains(pathLower, "legacy") {
		score -= 4.0
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
		}
	}
	return profile
}

func coreQueryTerms(queryLower string) []string {
	generic := map[string]bool{
		"adr": true, "architecture": true, "background": true, "code": true,
		"context": true, "decision": true, "design": true, "file": true,
		"implement": true, "implementation": true, "note": true, "notes": true,
		"plan": true, "prd": true, "product": true, "proposal": true,
		"requirements": true, "rfc": true, "source": true, "spec": true,
		"task": true, "tasks": true,
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
		if strings.ContainsAny(term, "_.-") {
			out = append(out, term)
		}
	}
	return out
}

func bodyHitCap(role string, sourceFile bool, term string) int {
	if sourceFile {
		return 10
	}
	if strings.ContainsAny(term, "_.-") {
		return 5
	}
	switch role {
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

func hasProductBackgroundIntent(queryLower string) bool {
	return containsAny(queryLower, "prd", "product", "background", "requirements", "user outcome", "user story", "customer access")
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
	case strings.HasPrefix(path, "docs/rfcs/") || strings.HasPrefix(path, "docs/rfc/") || strings.HasPrefix(path, "rfcs/") || strings.HasPrefix(path, "rfc/") || strings.Contains(path, "/docs/rfcs/") || strings.Contains(path, "/docs/rfc/") || strings.Contains(path, "/rfcs/") || strings.Contains(path, "/rfc/"):
		return "rfc"
	case strings.Contains(path, "/plans/") || strings.HasPrefix(path, "plans/"):
		return "plan"
	case strings.HasPrefix(path, ".cursor/") || strings.HasPrefix(path, ".claude/"):
		return "agent_note"
	default:
		return ""
	}
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
	for _, roleTerm := range []string{"adr", "design", "plan", "prd", "proposal", "rfc", "spec"} {
		if _, ok := terms[roleTerm]; ok {
			terms[roleTerm] = 0.35
		}
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
	titleLower := strings.ToLower(c.Title)
	bodyLower := strings.ToLower(c.Body)
	var reasons []string
	if c.Metadata != nil && c.Metadata["retrieval_expansion_reason"] != "" {
		reasons = append(reasons, "relationship expansion: "+c.Metadata["retrieval_expansion_reason"])
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
	case "rfc":
		if containsAny(queryLower, "rfc", "request for comments", "proposal", "alternative", "alternatives", "motivation", "drawback", "drawbacks", "design") {
			reasons = append(reasons, "authority/query-intent signal: RFC")
		}
	case "openspec_bundle", "openspec_design", "openspec_tasks", "openspec_spec", "openspec_proposal":
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
		for _, target := range metadataTargets(c.Metadata, "link_contained_by") {
			addTarget(target, "openspec_parent_bundle")
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
		return role == "tasks" || role == "openspec_tasks" || role == "proposal" || role == "openspec_proposal"
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
