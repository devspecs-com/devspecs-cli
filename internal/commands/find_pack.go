package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

type FindPackOutput struct {
	Query            string                `json:"query"`
	Retriever        string                `json:"retriever"`
	Mode             string                `json:"mode"`
	Summary          retrieval.PackSummary `json:"summary,omitempty"`
	Groups           []retrieval.PackGroup `json:"groups"`
	ExcludedNoise    []retrieval.PackItem  `json:"excluded_noise,omitempty"`
	Counts           map[string]int        `json:"counts,omitempty"`
	RankedResults    []FindResult          `json:"ranked_results"`
	GraphContext     *FindGraphPackContext `json:"graph_context,omitempty"`
	GraphDiagnostics *FindGraphDiagnostics `json:"graph_diagnostics,omitempty"`
}

func findPackOutput(query, retrieverName string, candidates []retrieval.Candidate, reasons map[string][]string, rolePack retrieval.RoleGroupedPack) FindPackOutput {
	return FindPackOutput{
		Query:         query,
		Retriever:     retrieverName,
		Mode:          rolePack.Mode,
		Summary:       rolePack.Summary,
		Groups:        rolePack.Groups,
		ExcludedNoise: rolePack.ExcludedNoise,
		Counts:        rolePack.Counts,
		RankedResults: findResults(candidates, reasons, retrieverName),
	}
}

func writeFindPackText(out io.Writer, query, retrieverName string, rolePack retrieval.RoleGroupedPack) error {
	fmt.Fprintf(out, "Working set: %s\n", query)
	fmt.Fprintf(out, "Retriever: %s\n", retrieverName)
	fmt.Fprintf(out, "Mode: %s\n", rolePack.Mode)
	writePackSummary(out, rolePack.Summary)

	if len(rolePack.Groups) == 0 && len(rolePack.ExcludedNoise) == 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "No matching artifacts found.")
		return nil
	}

	for _, group := range rolePack.Groups {
		if len(group.Items) == 0 {
			continue
		}
		title := group.Title
		if title == "" {
			title = retrieval.PackRoleTitle(group.Role)
		}
		fmt.Fprintf(out, "\n%s (%d)\n", title, len(group.Items))
		if group.OverflowCount > 0 {
			fmt.Fprintf(out, "  Note: %d item(s) over the recommended budget of %d.\n", group.OverflowCount, group.Budget)
		}
		for _, item := range group.Items {
			writePackItem(out, item, false)
		}
	}

	if len(rolePack.ExcludedNoise) > 0 {
		fmt.Fprintf(out, "\n%s (%d)\n", retrieval.PackRoleTitle(retrieval.PackRoleExcludedNoise), len(rolePack.ExcludedNoise))
		for _, item := range rolePack.ExcludedNoise {
			writePackItem(out, item, true)
		}
	}
	return nil
}

func writePackSummary(out io.Writer, summary retrieval.PackSummary) {
	if summary.IncludedCount == 0 && summary.ExcludedNoiseCount == 0 && summary.GroupCount == 0 {
		return
	}
	fmt.Fprintf(out, "Summary: %d included across %d role group(s)", summary.IncludedCount, summary.RoleDiversity)
	if summary.ExcludedNoiseCount > 0 {
		fmt.Fprintf(out, "; %d excluded as likely noise", summary.ExcludedNoiseCount)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Coverage: background=%s implementation=%s tests=%s config=%s open_work=%s\n",
		yesNo(summary.HasBackgroundDecisions),
		yesNo(summary.HasImplementation),
		yesNo(summary.HasBehaviorTests),
		yesNo(summary.HasConfigSchema),
		yesNo(summary.HasOpenWork),
	)
	if len(summary.Notes) > 0 {
		fmt.Fprintf(out, "Notes: %s\n", strings.Join(limitStrings(summary.Notes, 2), "; "))
	}
}

func writePackItem(out io.Writer, item retrieval.PackItem, excluded bool) {
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
	if item.PackTier != "" {
		fmt.Fprintf(out, "  %2d. %s  %s [%s]\n", item.OriginalRank, label, title, item.PackTier)
	} else {
		fmt.Fprintf(out, "  %2d. %s  %s\n", item.OriginalRank, label, title)
	}
	if item.Path != "" {
		fmt.Fprintf(out, "      Source: %s\n", item.Path)
	}
	if item.SourcePath != "" && item.SourcePath != item.Path {
		fmt.Fprintf(out, "      From: %s\n", item.SourcePath)
	}
	if item.Kind != "" || item.Subtype != "" {
		fmt.Fprintf(out, "      Type: %s\n", compactKindSubtype(item.Kind, item.Subtype))
	}
	if item.RoleReason != "" {
		prefix := "Why"
		if excluded {
			prefix = "Because"
		}
		fmt.Fprintf(out, "      %s: %s\n", prefix, item.RoleReason)
	}
	if len(item.AuthorityCues) > 0 {
		fmt.Fprintf(out, "      Cues: %s\n", strings.Join(limitStrings(item.AuthorityCues, 3), "; "))
	}
	if len(item.Reasons) > 0 {
		reasonLabel := "Matched"
		if excluded {
			reasonLabel = "Weak match"
		}
		fmt.Fprintf(out, "      %s: %s\n", reasonLabel, strings.Join(limitStrings(item.Reasons, 3), "; "))
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func compactKindSubtype(kind, subtype string) string {
	kind = strings.TrimSpace(kind)
	subtype = strings.TrimSpace(subtype)
	switch {
	case kind == "" && subtype == "":
		return "-"
	case subtype == "":
		return kind
	case kind == "":
		return subtype
	default:
		return kind + "/" + subtype
	}
}

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]string, limit, limit+1)
	copy(out, values[:limit])
	out = append(out, fmt.Sprintf("+%d more", len(values)-limit))
	return out
}
