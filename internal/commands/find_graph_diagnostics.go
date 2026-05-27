package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	findGraphDiagnosticsMode       = "typed_edge_pack_scout_v0"
	findGraphMaxSeeds              = 8
	findGraphMaxCandidates         = 6
	findGraphMaxOutgoingPerEdge    = 3
	findGraphMaxSuppressed         = 12
	findGraphMinAdmitConfidence    = 0.75
	findGraphSuppressionSupportMsg = "support-only edge cannot admit candidate"
)

var findGraphAdmittingEdges = map[string]bool{
	"tests_source":              true,
	"mentions_symbol":           true,
	"same_file_or_line_variant": true,
	"explicit_reference":        true,
	"openspec_companion":        true,
}

var findGraphSupportOnlyEdges = map[string]bool{
	"mentions_same_concept":  true,
	"same_layout_group":      true,
	"same_workstream_anchor": true,
	"co_changed_with":        true,
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
	for _, seed := range seeds {
		if seed.ID == "" {
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
			if findGraphSupportOnlyEdges[edge.EdgeType] {
				diag.Counts["suppressed_support_only"]++
				diag.addSuppression(target, seed, edge, findGraphSuppressionSupportMsg)
				continue
			}
			if !findGraphAdmittingEdges[edge.EdgeType] {
				diag.Counts["suppressed_unknown_edge"]++
				diag.addSuppression(target, seed, edge, "edge type is not enabled for graph admission")
				continue
			}
			if edge.Confidence < findGraphMinAdmitConfidence {
				diag.Counts["suppressed_low_confidence"]++
				diag.addSuppression(target, seed, edge, "edge confidence below graph admission threshold")
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
		Receipt:           findGraphReceipt(seed, edge),
	}
}

func findGraphReceipt(seed retrieval.Candidate, edge store.ArtifactEdgeRow) string {
	seedPath := findGraphDisplayPath(seed)
	explanation := strings.TrimSpace(edge.Explanation)
	if explanation == "" {
		explanation = strings.TrimSpace(edge.SourceSignal)
	}
	if explanation == "" {
		explanation = "typed graph evidence"
	}
	return fmt.Sprintf("%s from %s: %s", edge.EdgeType, seedPath, explanation)
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
