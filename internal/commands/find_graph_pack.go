package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const findGraphPackContextMode = "graph_pack_context_v0"

type FindGraphPackContext struct {
	Mode            string               `json:"mode"`
	EvidenceMode    string               `json:"evidence_mode"`
	Title           string               `json:"title"`
	CandidateCount  int                  `json:"candidate_count"`
	SuppressedCount int                  `json:"suppressed_count,omitempty"`
	Counts          map[string]int       `json:"counts,omitempty"`
	Groups          []FindGraphPackGroup `json:"groups,omitempty"`
	Notes           []string             `json:"notes,omitempty"`
}

type FindGraphPackGroup struct {
	Role  string               `json:"role"`
	Title string               `json:"title"`
	Items []FindGraphCandidate `json:"items"`
}

func findGraphPackContext(diag FindGraphDiagnostics) *FindGraphPackContext {
	ctx := &FindGraphPackContext{
		Mode:            findGraphPackContextMode,
		EvidenceMode:    diag.Mode,
		Title:           "Related via test/source evidence",
		CandidateCount:  diag.CandidateCount,
		SuppressedCount: diag.SuppressedCount,
		Counts:          diag.Counts,
		Notes:           diag.Notes,
	}
	grouped := map[string][]FindGraphCandidate{}
	for _, candidate := range diag.Candidates {
		role := strings.TrimSpace(candidate.Role)
		if role == "" {
			role = retrieval.PackRoleSupportingContext
		}
		grouped[role] = append(grouped[role], candidate)
	}
	for _, role := range findGraphPackRoleOrder(grouped) {
		ctx.Groups = append(ctx.Groups, FindGraphPackGroup{
			Role:  role,
			Title: retrieval.PackRoleTitle(role),
			Items: grouped[role],
		})
	}
	return ctx
}

func findGraphPackRoleOrder(grouped map[string][]FindGraphCandidate) []string {
	preferred := []string{
		retrieval.PackRoleImplementation,
		retrieval.PackRoleBehaviorTests,
		retrieval.PackRoleConfigSchema,
		retrieval.PackRoleSupportingContext,
	}
	seen := map[string]bool{}
	var out []string
	for _, role := range preferred {
		if len(grouped[role]) > 0 {
			out = append(out, role)
			seen[role] = true
		}
	}
	for role := range grouped {
		if !seen[role] {
			out = append(out, role)
		}
	}
	return out
}

func writeFindGraphPackText(out io.Writer, ctx *FindGraphPackContext) {
	if ctx == nil {
		return
	}
	fmt.Fprintf(out, "\n%s (%d)\n", ctx.Title, ctx.CandidateCount)
	fmt.Fprintf(out, "  Mode: %s\n", ctx.Mode)
	if ctx.EvidenceMode != "" {
		fmt.Fprintf(out, "  Evidence: %s\n", ctx.EvidenceMode)
	}
	if len(ctx.Groups) == 0 {
		fmt.Fprintln(out, "  No related source/test graph context admitted.")
		if ctx.SuppressedCount > 0 {
			fmt.Fprintf(out, "  Suppressed: %d support/noise candidate(s)\n", ctx.SuppressedCount)
		}
		for _, note := range limitStrings(ctx.Notes, 2) {
			fmt.Fprintf(out, "  Note: %s\n", note)
		}
		return
	}
	for _, group := range ctx.Groups {
		fmt.Fprintf(out, "  %s (%d)\n", group.Title, len(group.Items))
		for i, item := range group.Items {
			writeGraphPackItem(out, i+1, item)
		}
	}
	if ctx.SuppressedCount > 0 {
		fmt.Fprintf(out, "  Suppressed: %d support/noise candidate(s)\n", ctx.SuppressedCount)
	}
}

func writeGraphPackItem(out io.Writer, rank int, item FindGraphCandidate) {
	label := item.ShortID
	if label == "" {
		label = item.ID
	}
	if label == "" {
		label = item.Path
	}
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = item.Path
	}
	fmt.Fprintf(out, "    %2d. %s  %s\n", rank, label, title)
	if item.Path != "" {
		fmt.Fprintf(out, "        Source: %s\n", item.Path)
	}
	if item.SeedPath != "" {
		fmt.Fprintf(out, "        Connected from: %s\n", item.SeedPath)
	}
	evidence := item.AdmissionEdgeType
	if item.SourceSignal != "" {
		evidence += "/" + item.SourceSignal
	}
	if item.CompanionDerived {
		evidence += " companion-derived"
	}
	fmt.Fprintf(out, "        Evidence: %s confidence %.2f\n", evidence, item.Confidence)
	if item.Receipt != "" {
		fmt.Fprintf(out, "        Why: %s\n", item.Receipt)
	}
}
