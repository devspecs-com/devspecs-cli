package retrieval

import (
	"path/filepath"
	"sort"
	"strconv"
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
	sort.Slice(scoredCandidates, func(i, j int) bool {
		if scoredCandidates[i].score == scoredCandidates[j].score {
			return scoredCandidates[i].candidate.Path < scoredCandidates[j].candidate.Path
		}
		return scoredCandidates[i].score > scoredCandidates[j].score
	})
	limit := retrievalLimit(queryLower, terms)
	scoredCandidates = collapseVariantCandidates(scoredCandidates, queryLower)
	scoredCandidates = selectScoredCandidates(scoredCandidates, queryLower, limit)
	out := make([]Candidate, 0, len(scoredCandidates))
	for _, sf := range scoredCandidates {
		out = append(out, sf.candidate)
	}
	out = expandOpenSpecLinks(out, candidates, queryLower, limit)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
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
			"claude", "agents", "maintainer", "maintainers", "codeowners",
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
	case kind == "decision" && subtype == "adr":
		return "adr"
	case kind == "requirements" && subtype == "prd":
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
		if strings.ContainsAny(term, "_.-") {
			return true
		}
	}
	return false
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
