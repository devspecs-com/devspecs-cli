package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	findGraphDiagnosticsMode       = "typed_edge_pack_scout_v1"
	findGraphMaxSeeds              = 8
	findGraphMaxCandidates         = 6
	findGraphMaxOutgoingPerEdge    = 3
	findGraphMaxSuppressed         = 12
	findGraphMinAdmitConfidence    = 0.75
	findGraphSuppressionSupportMsg = "support-only edge cannot admit candidate"
)

var findGraphAdmittingEdges = map[string]bool{
	"tests_source":    true,
	"mentions_symbol": true,
}

var findGraphSupportOnlyEdges = map[string]bool{
	"explicit_reference":         true,
	"same_file_or_line_variant":  true,
	"openspec_companion":         true,
	"mentions_same_concept":      true,
	"same_layout_group":          true,
	"same_workstream_anchor":     true,
	"same_workstream_reference":  true,
	"co_changed_with":            true,
	"same_pr_reference":          true,
	"same_ticket_reference":      true,
	"same_commit_reference":      true,
	"same_branch_reference":      true,
	"same_workstream_cluster":    true,
	"support_cross_role":         true,
	"strong_workstream_cluster":  true,
	"workstream_cluster_member":  true,
	"workstream_cluster_support": true,
}

type FindGraphOutput struct {
	Query            string               `json:"query"`
	Retriever        string               `json:"retriever"`
	Mode             string               `json:"mode"`
	RankedResults    []FindResult         `json:"ranked_results"`
	GraphDiagnostics FindGraphDiagnostics `json:"graph_diagnostics"`
}

type FindGraphDiagnostics struct {
	Mode            string                 `json:"mode"`
	SeedCount       int                    `json:"seed_count"`
	CandidateCount  int                    `json:"candidate_count"`
	SuppressedCount int                    `json:"suppressed_count,omitempty"`
	Counts          map[string]int         `json:"counts,omitempty"`
	Candidates      []FindGraphCandidate   `json:"candidates,omitempty"`
	Suppressed      []FindGraphSuppression `json:"suppressed,omitempty"`
	Notes           []string               `json:"notes,omitempty"`
}

type FindGraphCandidate struct {
	ID                string   `json:"id,omitempty"`
	ShortID           string   `json:"short_id,omitempty"`
	Path              string   `json:"path,omitempty"`
	SourcePath        string   `json:"source_path,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Subtype           string   `json:"subtype,omitempty"`
	Title             string   `json:"title,omitempty"`
	Role              string   `json:"role,omitempty"`
	RoleReason        string   `json:"role_reason,omitempty"`
	SeedPath          string   `json:"seed_path,omitempty"`
	AdmissionEdgeType string   `json:"admission_edge_type"`
	Confidence        float64  `json:"confidence"`
	Weight            float64  `json:"weight,omitempty"`
	SourceSignal      string   `json:"source_signal,omitempty"`
	CompanionDerived  bool     `json:"companion_derived,omitempty"`
	Receipt           string   `json:"receipt"`
	SupportReceipts   []string `json:"support_receipts,omitempty"`
}

type FindGraphSuppression struct {
	Path       string  `json:"path,omitempty"`
	SeedPath   string  `json:"seed_path,omitempty"`
	EdgeType   string  `json:"edge_type,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reason     string  `json:"reason"`
}

func findGraphOutput(query, retrieverName string, candidates []retrieval.Candidate, reasons map[string][]string, diagnostics FindGraphDiagnostics) FindGraphOutput {
	return FindGraphOutput{
		Query:            query,
		Retriever:        retrieverName,
		Mode:             diagnostics.Mode,
		RankedResults:    findResults(candidates, reasons, retrieverName),
		GraphDiagnostics: diagnostics,
	}
}

func buildFindGraphDiagnostics(db *store.DB, fp store.FilterParams, query string, ranked []retrieval.Candidate) FindGraphDiagnostics {
	diag := FindGraphDiagnostics{
		Mode:   findGraphDiagnosticsMode,
		Counts: map[string]int{},
	}
	if len(ranked) == 0 {
		diag.Notes = append(diag.Notes, "no direct retrieval seeds available")
		return diag
	}
	allCandidates, err := loadRetrievalCandidates(db, fp)
	if err != nil {
		diag.Notes = append(diag.Notes, "graph diagnostics unavailable: "+err.Error())
		return diag
	}
	byID := map[string]retrieval.Candidate{}
	for _, c := range allCandidates {
		if c.ID != "" {
			byID[c.ID] = c
		}
	}
	directIDs := map[string]bool{}
	directPaths := map[string]bool{}
	for _, c := range ranked {
		if c.ID != "" {
			directIDs[c.ID] = true
		}
		if c.Path != "" {
			directPaths[normalizeFindGraphPath(c.Path)] = true
		}
		if c.Source != "" {
			directPaths[normalizeFindGraphPath(c.Source)] = true
		}
	}

	seeds := ranked
	if len(seeds) > findGraphMaxSeeds {
		seeds = seeds[:findGraphMaxSeeds]
	}
	diag.SeedCount = len(seeds)
	seenCandidates := map[string]bool{}
	allowsSourceTestGraph := findGraphQueryAllowsSourceTest(query)
	for seedIndex, seed := range seeds {
		if seed.ID == "" {
			continue
		}
		seedRole := retrieval.ClassifyPackRole(seed, query)
		if seedRole.Role == retrieval.PackRoleExcludedNoise {
			diag.Counts["suppressed_seed_pack_exclusion"]++
			continue
		}
		if !findGraphSourceTestRole(seedRole.Role) {
			diag.Counts["suppressed_seed_role"]++
			continue
		}
		if !findGraphSeedHasSpecificQueryAnchor(seed, query, seedIndex) {
			diag.Counts["suppressed_seed_anchor"]++
			continue
		}
		edges := findGraphSeedEdges(db, seed)
		perType := map[string]int{}
		for _, edge := range edges {
			targetID := findGraphOtherArtifactID(edge, seed.ID)
			if targetID == "" || directIDs[targetID] {
				continue
			}
			target, ok := byID[targetID]
			if !ok {
				diag.Counts["suppressed_missing_candidate"]++
				continue
			}
			if directPaths[normalizeFindGraphPath(target.Path)] || directPaths[normalizeFindGraphPath(target.Source)] {
				continue
			}
			if findGraphSupportOnlyEdge(edge) {
				diag.Counts["suppressed_support_only"]++
				diag.addSuppression(target, seed, edge, findGraphSuppressionSupportMsg)
				continue
			}
			if !findGraphAdmittingEdge(edge) {
				diag.Counts["suppressed_unknown_edge"]++
				diag.addSuppression(target, seed, edge, "edge type is not enabled for graph admission")
				continue
			}
			if edge.Confidence < findGraphMinAdmitConfidence {
				diag.Counts["suppressed_low_confidence"]++
				diag.addSuppression(target, seed, edge, "edge confidence below graph admission threshold")
				continue
			}
			if !allowsSourceTestGraph {
				diag.Counts["suppressed_query_intent"]++
				diag.addSuppression(target, seed, edge, "query does not ask for source/test/debug/implementation context")
				continue
			}
			if perType[edge.EdgeType] >= findGraphMaxOutgoingPerEdge {
				diag.Counts["suppressed_edge_budget"]++
				continue
			}
			if seenCandidates[targetID] {
				continue
			}
			role := retrieval.ClassifyPackRole(target, query)
			if role.Role == retrieval.PackRoleExcludedNoise {
				diag.Counts["suppressed_pack_exclusion"]++
				diag.addSuppression(target, seed, edge, role.Reason)
				continue
			}
			if !findGraphEdgeRolesCompatible(edge.EdgeType, seedRole.Role, role.Role) {
				diag.Counts["suppressed_role_mismatch"]++
				diag.addSuppression(target, seed, edge, "edge does not connect source and test roles")
				continue
			}
			perType[edge.EdgeType]++
			seenCandidates[targetID] = true
			diag.Candidates = append(diag.Candidates, findGraphCandidate(target, seed, edge, role))
			diag.Counts["admitted_"+edge.EdgeType]++
			if len(diag.Candidates) >= findGraphMaxCandidates {
				diag.Counts["candidate_budget_reached"]++
				break
			}
		}
		if len(diag.Candidates) >= findGraphMaxCandidates {
			break
		}
	}
	sort.SliceStable(diag.Candidates, func(i, j int) bool {
		if diag.Candidates[i].Confidence != diag.Candidates[j].Confidence {
			return diag.Candidates[i].Confidence > diag.Candidates[j].Confidence
		}
		if diag.Candidates[i].AdmissionEdgeType != diag.Candidates[j].AdmissionEdgeType {
			return diag.Candidates[i].AdmissionEdgeType < diag.Candidates[j].AdmissionEdgeType
		}
		return diag.Candidates[i].Path < diag.Candidates[j].Path
	})
	diag.CandidateCount = len(diag.Candidates)
	diag.SuppressedCount = len(diag.Suppressed)
	if len(diag.Counts) == 0 {
		diag.Counts = nil
	}
	if len(diag.Candidates) == 0 {
		diag.Notes = append(diag.Notes, "no typed graph attachments admitted")
	}
	return diag
}

func findGraphQueryAllowsSourceTest(query string) bool {
	for _, token := range findGraphQueryFields(query) {
		switch token {
		case "assert", "assertion", "assertions", "behavior", "behaviour", "bug", "code", "coverage", "debug", "e2e", "fail", "failing", "failure", "fix", "fixture", "fixtures", "implementation", "integration", "regression", "source", "sources", "spec", "specs", "test", "tests", "trace", "unit", "validate", "verify":
			return true
		}
		if strings.HasPrefix(token, "implement") {
			return true
		}
	}
	return false
}

func findGraphSeedHasSpecificQueryAnchor(seed retrieval.Candidate, query string, seedIndex int) bool {
	terms := findGraphSpecificQueryTerms(query)
	if len(terms) == 0 {
		return false
	}
	strongHaystack := strings.ToLower(strings.Join([]string{seed.Path, seed.Source, seed.Title}, " "))
	compactStrongHaystack := findGraphCompactAlnum(strongHaystack)
	bodyHaystack := strings.ToLower(seed.Body)
	compactBodyHaystack := findGraphCompactAlnum(bodyHaystack)
	for _, term := range terms {
		if strings.Contains(strongHaystack, term) || strings.Contains(compactStrongHaystack, term) {
			return true
		}
		if seedIndex < 2 && (strings.Contains(bodyHaystack, term) || strings.Contains(compactBodyHaystack, term)) {
			return true
		}
	}
	return false
}

func findGraphSpecificQueryTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, field := range findGraphQueryFields(query) {
		field = strings.TrimSpace(field)
		if len(field) < 4 || findGraphGenericQueryTerm(field) || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func findGraphQueryFields(query string) []string {
	return strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func findGraphGenericQueryTerm(term string) bool {
	switch term {
	case "adapter", "adapters", "assert", "behavior", "behaviour", "code", "coverage", "debug", "failure", "failing", "fixture", "fixtures", "implementation", "implement", "integration", "source", "spec", "test", "tests", "unit", "validate", "verify":
		return true
	default:
		return false
	}
}

func findGraphCompactAlnum(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func findGraphSourceTestRole(role string) bool {
	return role == retrieval.PackRoleImplementation || role == retrieval.PackRoleBehaviorTests
}

func findGraphSupportOnlyEdge(edge store.ArtifactEdgeRow) bool {
	if edge.EdgeType == "mentions_symbol" && edge.SourceSignal == "symbol_reference" {
		return true
	}
	return findGraphSupportOnlyEdges[edge.EdgeType]
}

func findGraphAdmittingEdge(edge store.ArtifactEdgeRow) bool {
	if edge.EdgeType == "mentions_symbol" {
		return edge.SourceSignal == "test_symbol_match"
	}
	return findGraphAdmittingEdges[edge.EdgeType]
}

func findGraphEdgeRolesCompatible(edgeType, seedRole, targetRole string) bool {
	if edgeType != "tests_source" && edgeType != "mentions_symbol" {
		return true
	}
	return (seedRole == retrieval.PackRoleImplementation && targetRole == retrieval.PackRoleBehaviorTests) ||
		(seedRole == retrieval.PackRoleBehaviorTests && targetRole == retrieval.PackRoleImplementation)
}

func findGraphSeedEdges(db *store.DB, seed retrieval.Candidate) []store.ArtifactEdgeRow {
	repoID := metadataValue(seed, "repo_id")
	if repoID == "" {
		return nil
	}
	var edges []store.ArtifactEdgeRow
	srcEdges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{RepoID: repoID, SrcArtifactID: seed.ID})
	if err == nil {
		edges = append(edges, srcEdges...)
	}
	dstEdges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{RepoID: repoID, DstArtifactID: seed.ID})
	if err == nil {
		edges = append(edges, dstEdges...)
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Confidence != edges[j].Confidence {
			return edges[i].Confidence > edges[j].Confidence
		}
		if edges[i].Weight != edges[j].Weight {
			return edges[i].Weight > edges[j].Weight
		}
		return edges[i].EdgeType < edges[j].EdgeType
	})
	return edges
}

func findGraphOtherArtifactID(edge store.ArtifactEdgeRow, seedID string) string {
	switch seedID {
	case edge.SrcArtifactID:
		return edge.DstArtifactID
	case edge.DstArtifactID:
		return edge.SrcArtifactID
	default:
		return ""
	}
}

func findGraphCandidate(target, seed retrieval.Candidate, edge store.ArtifactEdgeRow, role retrieval.PackRoleDecision) FindGraphCandidate {
	return FindGraphCandidate{
		ID:                target.ID,
		ShortID:           shortCandidateID(target),
		Path:              target.Path,
		SourcePath:        target.Source,
		Kind:              target.Kind,
		Subtype:           target.Subtype,
		Title:             target.Title,
		Role:              role.Role,
		RoleReason:        role.Reason,
		SeedPath:          findGraphDisplayPath(seed),
		AdmissionEdgeType: edge.EdgeType,
		Confidence:        edge.Confidence,
		Weight:            edge.Weight,
		SourceSignal:      edge.SourceSignal,
		CompanionDerived:  findGraphCompanionDerived(target) || findGraphCompanionDerived(seed),
		Receipt:           findGraphReceipt(target, seed, edge),
	}
}

func findGraphReceipt(target, seed retrieval.Candidate, edge store.ArtifactEdgeRow) string {
	srcPath := findGraphDisplayPath(seed)
	dstPath := findGraphDisplayPath(target)
	if seed.ID == edge.DstArtifactID {
		srcPath = findGraphDisplayPath(target)
		dstPath = findGraphDisplayPath(seed)
	}
	explanation := strings.TrimSpace(edge.Explanation)
	if explanation == "" {
		explanation = strings.TrimSpace(edge.SourceSignal)
	}
	if explanation == "" {
		explanation = "typed graph evidence"
	}
	return fmt.Sprintf("%s connects %s -> %s: %s", edge.EdgeType, srcPath, dstPath, explanation)
}

func findGraphCompanionDerived(c retrieval.Candidate) bool {
	reason := strings.ToLower(strings.TrimSpace(metadataValue(c, "admission_reason")))
	return reason == "test_source_companion"
}

func (diag *FindGraphDiagnostics) addSuppression(target, seed retrieval.Candidate, edge store.ArtifactEdgeRow, reason string) {
	if len(diag.Suppressed) >= findGraphMaxSuppressed {
		diag.Counts["suppressed_budget_reached"]++
		return
	}
	diag.Suppressed = append(diag.Suppressed, FindGraphSuppression{
		Path:       findGraphDisplayPath(target),
		SeedPath:   findGraphDisplayPath(seed),
		EdgeType:   edge.EdgeType,
		Confidence: edge.Confidence,
		Reason:     reason,
	})
}

func writeFindGraphDiagnosticsText(out io.Writer, diag FindGraphDiagnostics) {
	fmt.Fprintf(out, "\nGraph attachments (%d)\n", diag.CandidateCount)
	fmt.Fprintf(out, "  Mode: %s\n", diag.Mode)
	if len(diag.Candidates) == 0 {
		fmt.Fprintln(out, "  No typed graph attachments admitted.")
		if diag.SuppressedCount > 0 {
			fmt.Fprintf(out, "  Suppressed: %d support/noise candidate(s)\n", diag.SuppressedCount)
		}
		return
	}
	for i, c := range diag.Candidates {
		title := strings.TrimSpace(c.Title)
		if title == "" {
			title = c.Path
		}
		fmt.Fprintf(out, "  %2d. %s  %s\n", i+1, c.ShortID, title)
		if c.Path != "" {
			fmt.Fprintf(out, "      Source: %s\n", c.Path)
		}
		if c.Role != "" {
			fmt.Fprintf(out, "      Role: %s\n", retrieval.PackRoleTitle(c.Role))
		}
		fmt.Fprintf(out, "      Why: %s\n", c.Receipt)
	}
	if diag.SuppressedCount > 0 {
		fmt.Fprintf(out, "  Suppressed: %d support/noise candidate(s)\n", diag.SuppressedCount)
	}
}

func findGraphDisplayPath(c retrieval.Candidate) string {
	if c.Path != "" {
		return c.Path
	}
	if c.Source != "" {
		return c.Source
	}
	return c.ID
}

func normalizeFindGraphPath(path string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(path, "\\", "/")))
}
