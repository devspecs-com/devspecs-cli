package retrieval

import (
	"path/filepath"
	"strings"
)

const (
	PackRoleBackgroundDecisions = "background_decisions"
	PackRoleImplementation      = "implementation_surface"
	PackRoleBehaviorTests       = "behavior_tests"
	PackRoleConfigSchema        = "config_schema"
	PackRoleOpenWork            = "open_work"
	PackRoleSupportingContext   = "supporting_context"
	PackRoleExcludedNoise       = "excluded_noise"
)

var packRoleOrder = []string{
	PackRoleBackgroundDecisions,
	PackRoleImplementation,
	PackRoleBehaviorTests,
	PackRoleConfigSchema,
	PackRoleOpenWork,
	PackRoleSupportingContext,
}

var packRoleTitles = map[string]string{
	PackRoleBackgroundDecisions: "Background / decisions",
	PackRoleImplementation:      "Implementation surface",
	PackRoleBehaviorTests:       "Behavior tests",
	PackRoleConfigSchema:        "Config / schema",
	PackRoleOpenWork:            "Open work",
	PackRoleSupportingContext:   "Supporting context",
	PackRoleExcludedNoise:       "Excluded as likely noise",
}

var packRoleBudgets = map[string]int{
	PackRoleBackgroundDecisions: 3,
	PackRoleImplementation:      6,
	PackRoleBehaviorTests:       5,
	PackRoleConfigSchema:        3,
	PackRoleOpenWork:            3,
	PackRoleSupportingContext:   3,
	PackRoleExcludedNoise:       5,
}

type RoleGroupedPack struct {
	Mode          string            `json:"mode"`
	Groups        []PackGroup       `json:"groups"`
	ExcludedNoise []PackItem        `json:"excluded_noise,omitempty"`
	Counts        map[string]int    `json:"counts,omitempty"`
	Notes         []string          `json:"notes,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type PackGroup struct {
	Role          string     `json:"role"`
	Title         string     `json:"title"`
	Budget        int        `json:"budget,omitempty"`
	OverflowCount int        `json:"overflow_count,omitempty"`
	Items         []PackItem `json:"items"`
}

type PackItem struct {
	OriginalRank   int      `json:"original_rank"`
	ID             string   `json:"id,omitempty"`
	ShortID        string   `json:"short_id,omitempty"`
	Path           string   `json:"path,omitempty"`
	SourcePath     string   `json:"source_path,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Subtype        string   `json:"subtype,omitempty"`
	Title          string   `json:"title,omitempty"`
	Status         string   `json:"status,omitempty"`
	PackTier       string   `json:"pack_tier,omitempty"`
	Role           string   `json:"role"`
	RoleConfidence float64  `json:"role_confidence,omitempty"`
	RoleReason     string   `json:"role_reason,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	AuthorityCues  []string `json:"authority_cues,omitempty"`
	TokenEstimate  int      `json:"token_estimate,omitempty"`
}

type PackRoleDecision struct {
	Role       string  `json:"role"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason,omitempty"`
}

func BuildRoleGroupedPack(candidates []Candidate, reasons map[string][]string, query string) RoleGroupedPack {
	groupsByRole := make(map[string]PackGroup, len(packRoleOrder))
	counts := make(map[string]int, len(packRoleOrder)+1)
	for _, role := range packRoleOrder {
		groupsByRole[role] = PackGroup{
			Role:   role,
			Title:  PackRoleTitle(role),
			Budget: packRoleBudgets[role],
		}
	}

	seen := map[string]bool{}
	excluded := make([]PackItem, 0)
	for i, c := range candidates {
		key := packCandidateKey(c)
		if key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}

		decision := ClassifyPackRole(c, query)
		item := packItemFromCandidate(i+1, c, decision, reasons[c.Path])
		if decision.Role == PackRoleExcludedNoise {
			counts[PackRoleExcludedNoise]++
			excluded = append(excluded, item)
			continue
		}
		group, ok := groupsByRole[decision.Role]
		if !ok {
			decision = PackRoleDecision{
				Role:       PackRoleSupportingContext,
				Confidence: 0.55,
				Reason:     "fallback supporting context role",
			}
			item.Role = decision.Role
			item.RoleConfidence = decision.Confidence
			item.RoleReason = decision.Reason
			group = groupsByRole[decision.Role]
		}
		group.Items = append(group.Items, item)
		groupsByRole[decision.Role] = group
		counts[decision.Role]++
	}

	groups := make([]PackGroup, 0, len(packRoleOrder))
	for _, role := range packRoleOrder {
		group := groupsByRole[role]
		if len(group.Items) == 0 {
			continue
		}
		if group.Budget > 0 && len(group.Items) > group.Budget {
			group.OverflowCount = len(group.Items) - group.Budget
		}
		groups = append(groups, group)
	}

	if budget := packRoleBudgets[PackRoleExcludedNoise]; budget > 0 && len(excluded) > budget {
		excluded = excluded[:budget]
	}
	if len(counts) == 0 {
		counts = nil
	}

	return RoleGroupedPack{
		Mode:          "role_grouped_pack_v0",
		Groups:        groups,
		ExcludedNoise: excluded,
		Counts:        counts,
	}
}

func ClassifyPackRole(c Candidate, query string) PackRoleDecision {
	queryLower := strings.ToLower(query)
	role := candidateRole(c)
	if excluded, reason := classifyExcludedNoise(c, role, queryLower); excluded {
		return PackRoleDecision{Role: PackRoleExcludedNoise, Confidence: 0.82, Reason: reason}
	}
	if isTestCaseCandidate(c) || role == "test_case" {
		return PackRoleDecision{Role: PackRoleBehaviorTests, Confidence: 0.92, Reason: "test artifact captures expected behavior"}
	}
	if isConfigSchemaCandidate(c, role) {
		return PackRoleDecision{Role: PackRoleConfigSchema, Confidence: 0.82, Reason: "configuration, schema, workflow, or contract artifact"}
	}
	if IsSourceContextCandidate(c) || role == "code_comment" {
		return PackRoleDecision{Role: PackRoleImplementation, Confidence: 0.78, Reason: "source or implementation-adjacent artifact"}
	}
	if isOpenWorkCandidate(c, role) {
		return PackRoleDecision{Role: PackRoleOpenWork, Confidence: 0.78, Reason: "plan, tasks, todo, or active work artifact"}
	}
	if isBackgroundDecisionCandidate(c, role) {
		return PackRoleDecision{Role: PackRoleBackgroundDecisions, Confidence: 0.82, Reason: "decision, design, requirements, or spec artifact"}
	}
	if role == "agent_instruction" || role == "agent_note" || role == "protocol" || role == "skill" || role == "template" {
		return PackRoleDecision{Role: PackRoleSupportingContext, Confidence: 0.62, Reason: "requested process or instruction artifact"}
	}
	if IsPlanningIntentPath(c.Path) || isMarkdownCandidatePath(c.Path) {
		return PackRoleDecision{Role: PackRoleSupportingContext, Confidence: 0.58, Reason: "supporting documentation artifact"}
	}
	return PackRoleDecision{Role: PackRoleSupportingContext, Confidence: 0.5, Reason: "supporting matched artifact"}
}

func PackRoleTitle(role string) string {
	if title := packRoleTitles[role]; title != "" {
		return title
	}
	return role
}

func classifyExcludedNoise(c Candidate, role, queryLower string) (bool, string) {
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	switch {
	case isStaleArchiveCandidate(c) && !queryRequestsStaleOrHistory(queryLower) && packQueryTitlePathOverlap(c, queryLower) < 2:
		return true, "stale, archived, deprecated, or superseded artifact; query does not ask for historical context"
	case (role == "template" || isTemplatePathCandidate(pathLower)) && !queryRequestsTemplate(queryLower):
		return true, "template-like artifact; query does not ask for templates"
	case (role == "agent_instruction" || isAgentInstructionPath(c.Path)) && !queryRequestsAgentInstructions(queryLower):
		return true, "agent instruction artifact; query does not ask for repo agent rules"
	case role == "skill" && !queryRequestsSkill(c, queryLower):
		return true, "skill/protocol artifact; query does not ask for skills"
	case role == "protocol" && !queryRequestsProtocol(queryLower):
		return true, "process or protocol artifact; query does not ask for procedures or policies"
	case isGeneratedOrVendorCandidate(c) && !queryRequestsGeneratedOrVendor(queryLower):
		return true, "generated, vendor, or dependency artifact; query does not ask for generated/vendor context"
	default:
		return false, ""
	}
}

func isBackgroundDecisionCandidate(c Candidate, role string) bool {
	switch role {
	case "adr", "prd", "rfc", "design", "openspec_design", "openspec_spec", "openspec_proposal":
		return true
	}
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	return strings.Contains(pathLower, "/adr/") ||
		strings.Contains(pathLower, "/adrs/") ||
		strings.Contains(pathLower, "/architecture/") ||
		strings.Contains(pathLower, "/design/") ||
		strings.Contains(pathLower, "/requirements/") ||
		strings.Contains(pathLower, "/rfcs/") ||
		strings.Contains(pathLower, "/rfc/")
}

func isOpenWorkCandidate(c Candidate, role string) bool {
	switch role {
	case "plan", "openspec_tasks":
		return true
	}
	textLower := packDescriptorLower(c, role)
	return containsAny(textLower, "todo", "todos", "task", "tasks", "backlog", "roadmap", "next step", "follow-up", "followup")
}

func isConfigSchemaCandidate(c Candidate, role string) bool {
	if role == "model" {
		return true
	}
	subtype := strings.ToLower(strings.TrimSpace(c.Subtype))
	if subtype == "schema_model" || subtype == "configuration" || subtype == "workflow_definition" || subtype == "api_contract" {
		return true
	}
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	base := strings.ToLower(filepath.Base(pathLower))
	switch filepath.Ext(pathLower) {
	case ".yaml", ".yml", ".toml", ".json", ".jsonnet", ".hcl", ".tf", ".sql", ".graphql", ".proto":
		return true
	}
	return base == "dockerfile" ||
		base == "makefile" ||
		strings.Contains(pathLower, "/config/") ||
		strings.Contains(pathLower, "/configs/") ||
		strings.Contains(pathLower, "/schema/") ||
		strings.Contains(pathLower, "/schemas/") ||
		strings.Contains(pathLower, "/migrations/") ||
		strings.Contains(pathLower, "/.github/workflows/")
}

func isStaleArchiveCandidate(c Candidate) bool {
	statusLower := strings.ToLower(strings.TrimSpace(c.Status))
	if isActivePackStatus(statusLower) {
		return false
	}
	if containsAny(statusLower, "archived", "deprecated", "superseded", "obsolete", "stale") {
		return true
	}
	for _, value := range []string{
		packMetadataValue(c, "classifier_status"),
		packMetadataValue(c, "classifier_lifecycle"),
		packMetadataValue(c, "lifecycle"),
		packMetadataValue(c, "status"),
	} {
		if containsAny(strings.ToLower(value), "archived", "deprecated", "superseded", "obsolete", "stale") {
			return true
		}
	}
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	if hasPathSegment(pathLower, "archive") ||
		hasPathSegment(pathLower, "archives") ||
		hasPathSegment(pathLower, "archived") ||
		hasPathSegment(pathLower, "deprecated") ||
		hasPathSegment(pathLower, "obsolete") {
		return true
	}
	base := strings.ToLower(filepath.Base(pathLower))
	return strings.HasPrefix(base, "deprecated-") ||
		strings.HasPrefix(base, "deprecated_") ||
		strings.HasPrefix(base, "obsolete-") ||
		strings.HasPrefix(base, "obsolete_") ||
		strings.HasSuffix(base, ".deprecated.md")
}

func isActivePackStatus(statusLower string) bool {
	switch statusLower {
	case "active", "accepted", "approved", "current", "implementing", "in_progress", "in-progress", "proposed":
		return true
	default:
		return false
	}
}

func isGeneratedOrVendorCandidate(c Candidate) bool {
	pathLower := strings.ToLower(filepath.ToSlash(c.Path))
	return containsAny(pathLower,
		"/vendor/",
		"/node_modules/",
		"/dist/",
		"/build/",
		"/generated/",
		"/gen/",
		".generated.",
		"_generated.",
		"generated_",
	)
}

func isTemplatePathCandidate(pathLower string) bool {
	if strings.Contains(pathLower, "/templates/") ||
		strings.Contains(pathLower, ".github/issue_template/") ||
		strings.Contains(pathLower, "/.github/issue_template/") ||
		strings.Contains(pathLower, ".github/pull_request_template/") ||
		strings.Contains(pathLower, "/.github/pull_request_template/") {
		return true
	}
	base := strings.ToLower(filepath.Base(pathLower))
	return strings.Contains(base, "template") &&
		(strings.HasSuffix(base, ".md") || strings.HasSuffix(base, ".mdx") || strings.HasSuffix(base, ".txt"))
}

func queryRequestsTemplate(queryLower string) bool {
	return containsAny(queryLower,
		"template",
		"issue template",
		"pull request template",
		"pr template",
		"pull request workflow",
		"pull request requirements",
		"pull requests",
		"pull request",
	)
}

func queryRequestsAgentInstructions(queryLower string) bool {
	if containsAny(queryLower,
		"agent instruction",
		"agent instructions",
		"agents.md",
		"claude.md",
		".cursorrules",
		"repo instructions",
		"repository instructions",
		"project instructions",
		"coding instructions",
		"developer instructions",
		"repo rules",
		"repository rules",
		"project rules",
		"coding rules",
		"developer rules",
		"repo guidelines",
		"repository guidelines",
		"project guidelines",
		"developer guidelines",
		"development guidelines",
		"coding guidelines",
		"repo constraints",
		"repository constraints",
		"project constraints",
		"coding standards",
		"contributor guidelines",
		"rules for agents",
		"before editing",
		"before changing",
		"load the project-specific",
		"load project-specific",
	) {
		return true
	}
	return (containsAny(queryLower, "claude", "cursor", "codex", "assistant", "agent") && containsAny(queryLower, "instruction", "instructions", "rules", "guidelines", "constraints", "standards")) ||
		(containsAny(queryLower, "repo", "repository", "project", "developer", "development", "coding", "contributor", "contributors") && containsAny(queryLower, "instruction", "instructions", "rules", "guidelines", "constraints", "standards"))
}

func queryRequestsSkill(c Candidate, queryLower string) bool {
	if containsAny(queryLower, "skill", "skills", "workflow", "workflows", "playbook", "playbooks") {
		return true
	}
	return packQueryTitlePathOverlap(c, queryLower) >= 2
}

func queryRequestsProtocol(queryLower string) bool {
	return containsAny(queryLower,
		"protocol",
		"policy",
		"policies",
		"procedure",
		"procedures",
		"runbook",
		"governance",
		"contributing",
		"contribution",
		"guideline",
		"guidelines",
		"constraint",
		"constraints",
		"rules",
		"standard",
		"standards",
		"security policy",
		"pull request workflow",
		"pr workflow",
		"review workflow",
		"release workflow",
	)
}

func queryRequestsStaleOrHistory(queryLower string) bool {
	return containsAny(queryLower, "stale", "archive", "archived", "deprecated", "superseded", "history", "old behavior", "drift")
}

func queryRequestsGeneratedOrVendor(queryLower string) bool {
	return containsAny(queryLower, "generated", "vendor", "dependency", "dependencies", "dist", "build output")
}

func packQueryTitlePathOverlap(c Candidate, queryLower string) int {
	terms := meaningfulTerms(queryLower)
	if len(terms) == 0 {
		return 0
	}
	haystack := strings.ToLower(c.Path + "\n" + c.Title)
	count := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			count++
		}
	}
	return count
}

func packItemFromCandidate(rank int, c Candidate, decision PackRoleDecision, reasons []string) PackItem {
	path := c.Path
	if path == "" {
		path = c.Source
	}
	sourcePath := c.Source
	if sourcePath == path {
		sourcePath = ""
	}
	return PackItem{
		OriginalRank:   rank,
		ID:             c.ID,
		ShortID:        packMetadataValue(c, "short_id"),
		Path:           path,
		SourcePath:     sourcePath,
		Kind:           c.Kind,
		Subtype:        c.Subtype,
		Title:          c.Title,
		Status:         c.Status,
		PackTier:       CandidatePackTier(c),
		Role:           decision.Role,
		RoleConfidence: decision.Confidence,
		RoleReason:     decision.Reason,
		Reasons:        reasons,
		AuthorityCues:  packAuthorityCues(c),
		TokenEstimate:  estimatePackTokens(c),
	}
}

func packCandidateKey(c Candidate) string {
	if c.Path != "" {
		return c.Path
	}
	return c.Source
}

func packAuthorityCues(c Candidate) []string {
	cues := AuthorityCues(c)
	if len(cues) == 0 || !isActivePackStatus(strings.ToLower(strings.TrimSpace(c.Status))) {
		return cues
	}
	out := make([]string, 0, len(cues))
	for _, cue := range cues {
		cueLower := strings.ToLower(cue)
		if containsAny(cueLower, "superseded", "stale", "deprecated", "archived") {
			continue
		}
		out = append(out, cue)
	}
	return out
}

func packDescriptorLower(c Candidate, role string) string {
	parts := []string{c.Path, c.Source, c.Kind, c.Subtype, c.Title, c.Status, role}
	if c.Metadata != nil {
		for _, key := range []string{"classifier_model", "classifier_subtype", "classifier_family", "classifier_status", "classifier_lifecycle", "source_type", "artifact_scope", "lifecycle", "status"} {
			if value := c.Metadata[key]; value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func packMetadataValue(c Candidate, key string) string {
	if c.Metadata == nil {
		return ""
	}
	return c.Metadata[key]
}

func estimatePackTokens(c Candidate) int {
	if c.Body == "" {
		return 0
	}
	return (len(c.Body) + 3) / 4
}
