package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// NewFindCmd creates the ds find command.
func NewFindCmd() *cobra.Command {
	var (
		kind            string
		subtype         string
		tag             string
		branch          string
		user            string
		repoName        string
		allRepos        bool
		asJSON          bool
		noRefresh       bool
		pack            bool
		verbose         bool
		graphDiag       bool
		gitReceipts     = true
		anchorFirst     = true
		anchorMode      string
		boundaryPrimary bool
	)

	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search artifacts by title, path, or body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fp := store.FilterParams{Kind: kind, Subtype: subtype, Tag: tag, Branch: branch, User: user}
			if cmd.Flags().Changed("experimental-anchor-first-mode") && !cmd.Flags().Changed("experimental-anchor-first-ranking") {
				anchorFirst = true
			}
			return runFind(cmd, args[0], fp, repoName, allRepos, asJSON, noRefresh, pack, verbose, graphDiag, gitReceipts, anchorFirst, anchorMode, boundaryPrimary)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&subtype, "subtype", "", "Filter by subtype")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().BoolVar(&allRepos, "all", false, "Search artifacts in all indexed repos (ignore cwd scope)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().BoolVar(&pack, "pack", false, "Group results into a role-based context pack with inclusion and exclusion receipts")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed human output for pack receipts and diagnostics")
	cmd.Flags().BoolVar(&graphDiag, "graph-diagnostics", false, "Attach opt-in typed-edge graph diagnostics without changing ranked results")
	cmd.Flags().BoolVar(&gitReceipts, "git-receipts", true, "Attach bounded local git commit receipts to pack output when available")
	cmd.Flags().BoolVar(&anchorFirst, "experimental-anchor-first-ranking", true, "Use repo-local TF-IDF anchor-first ordering; pass false to disable")
	cmd.Flags().StringVar(&anchorMode, "experimental-anchor-first-mode", retrieval.DefaultAnchorFirstMode, "Anchor-first tuning mode: v1, rerank_only, selected_only, strong_field, or strict")
	cmd.Flags().BoolVar(&boundaryPrimary, "experimental-boundary-primary", false, "Tier pack output into a source-safe primary working set plus related context summary")
	return cmd
}

func runFind(cmd *cobra.Command, query string, fp store.FilterParams, repoName string, allRepos, asJSON, noRefresh, pack, verbose, graphDiag, gitReceipts, anchorFirst bool, anchorMode string, boundaryPrimary bool) error {
	start := time.Now()
	success := false
	anchorMode = retrieval.NormalizeAnchorFirstMode(anchorMode)
	if anchorMode == "" {
		return fmt.Errorf("unknown --experimental-anchor-first-mode; valid values: %s", strings.Join(retrieval.ValidAnchorFirstModes(), ", "))
	}
	props := map[string]any{
		"query_length_bucket": telemetry.QueryLengthBucket(query),
		"json":                asJSON,
		"pack":                pack,
		"verbose":             verbose,
		"graph_diagnostics":   graphDiag,
		"git_receipts":        gitReceipts,
		"anchor_first":        anchorFirst,
		"anchor_first_mode":   anchorMode,
		"boundary_primary":    boundaryPrimary,
	}
	defer func() {
		telemetry.RecordCommand("find", success, time.Since(start), props)
	}()

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	fp.RepoRoot = resolveRepoScope(db, repoName, allRepos)

	loadResult, err := loadRetrievalCandidatesForQueryWithReport(db, fp, query)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}
	candidates := loadResult.Candidates
	recordFindRuntimeProps(props, loadResult.Report)
	emitFindRuntimeDebug(cmd, loadResult.Report)
	retriever := retrieval.WeightedFilesRetrieverV0{AnchorFirstRanking: anchorFirst, AnchorFirstMode: anchorMode}
	matches := retriever.Retrieve(candidates, query)
	if len(matches) == 0 {
		matches = retrieval.QueryBaseline(candidates, query)
	}
	reasons := reasonsByPath(retrieval.ExplainCandidates(matches, query))
	var graphDiagnostics FindGraphDiagnostics
	if graphDiag {
		graphDiagnostics = buildFindGraphDiagnostics(db, fp, query, matches)
		props["graph_candidate_count_bucket"] = telemetry.CountBucket(graphDiagnostics.CandidateCount)
	}
	success = true
	props["result_count_bucket"] = telemetry.CountBucket(len(matches))

	if pack {
		rolePack := retrieval.BuildRoleGroupedPack(matches, reasons, query)
		if boundaryPrimary {
			rolePack = retrieval.ApplyBoundaryPrimaryPackForQuery(rolePack, query)
		}
		var gitTrust *FindGitTrustContext
		if gitReceipts && fp.RepoRoot != "" {
			gitTrust = buildFindGitTrustContext(cmd.Context(), fp.RepoRoot, query, rolePack)
			if gitTrust != nil {
				props["git_receipt_count_bucket"] = telemetry.CountBucket(len(gitTrust.Receipts))
			}
		}
		if asJSON {
			out := findPackOutput(query, retriever.Name(), matches, reasons, rolePack)
			out.GitTrust = gitTrust
			if graphDiag {
				out.GraphDiagnostics = &graphDiagnostics
				out.GraphContext = findGraphPackContext(graphDiagnostics)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		}
		if err := writeFindPackText(cmd.OutOrStdout(), query, retriever.Name(), rolePack, gitTrust, verbose); err != nil {
			return err
		}
		if graphDiag {
			writeFindGraphPackText(cmd.OutOrStdout(), findGraphPackContext(graphDiagnostics))
		}
		return nil
	}

	if graphDiag {
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(findGraphOutput(query, retriever.Name(), matches, reasons, graphDiagnostics))
		}
		if err := writeFindResultsText(cmd.OutOrStdout(), matches, reasons); err != nil {
			return err
		}
		writeFindGraphDiagnosticsText(cmd.OutOrStdout(), graphDiagnostics)
		return nil
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(findResults(matches, reasons, retriever.Name()))
	}

	return writeFindResultsText(cmd.OutOrStdout(), matches, reasons)
}

func writeFindResultsText(out io.Writer, matches []retrieval.Candidate, reasons map[string][]string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tSUBTYPE\tTITLE\tSOURCE\n")
	for _, c := range matches {
		displayID := shortCandidateID(c)
		if displayID == "" {
			displayID = c.ID
		}
		if len(displayID) > 13 {
			displayID = displayID[:13] + "..."
		}
		sub := c.Subtype
		if sub == "" {
			sub = "-"
		}
		source := c.Path
		if source == "" {
			source = c.Source
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", displayID, c.Kind, sub, c.Title, source)
		if cues := retrieval.AuthorityCues(c); len(cues) > 0 {
			fmt.Fprintf(w, "\t\t\tCues: %s\t\n", strings.Join(cues, "; "))
		}
		if rs := reasons[c.Path]; len(rs) > 0 {
			fmt.Fprintf(w, "\t\t\tReasons: %s\t\n", strings.Join(rs, "; "))
		}
	}
	return w.Flush()
}

func recordFindRuntimeProps(props map[string]any, report indexquery.CandidateLoadReport) {
	if report.RuntimeMode == "" {
		return
	}
	props["find_runtime"] = report.RuntimeMode
	props["find_effective_runtime"] = report.EffectiveMode
	props["candidate_count_bucket"] = telemetry.CountBucket(report.HydratedCount)
	if report.FullArtifactCount > 0 {
		props["full_artifact_count_bucket"] = telemetry.CountBucket(report.FullArtifactCount)
	}
	if report.PreselectedCount > 0 {
		props["preselected_count_bucket"] = telemetry.CountBucket(report.PreselectedCount)
	}
	if report.FallbackReason != "" {
		props["find_runtime_fallback"] = report.FallbackReason
	}
	if report.OptimizedError != "" {
		props["find_runtime_optimized_error"] = true
	}
}

func emitFindRuntimeDebug(cmd *cobra.Command, report indexquery.CandidateLoadReport) {
	if os.Getenv("DEVSPECS_FIND_RUNTIME_DEBUG") == "" || report.RuntimeMode == "" {
		return
	}
	_ = json.NewEncoder(cmd.ErrOrStderr()).Encode(map[string]any{
		"type":                "find_runtime",
		"runtime_mode":        report.RuntimeMode,
		"effective_mode":      report.EffectiveMode,
		"full_artifact_count": report.FullArtifactCount,
		"preselected_count":   report.PreselectedCount,
		"hydrated_count":      report.HydratedCount,
		"fallback_reason":     report.FallbackReason,
		"lane_counts":         report.LaneCounts,
		"preselect_ms":        report.PreselectMS,
		"hydrate_ms":          report.HydrateMS,
		"full_load_ms":        report.FullLoadMS,
		"optimized_error":     report.OptimizedError,
	})
}

type FindResult struct {
	ID             string            `json:"ID"`
	RepoID         string            `json:"RepoID"`
	ShortID        string            `json:"ShortID"`
	Path           string            `json:"path,omitempty"`
	Kind           string            `json:"Kind"`
	Subtype        string            `json:"Subtype"`
	Title          string            `json:"Title"`
	Status         string            `json:"Status"`
	CurrentRevID   string            `json:"CurrentRevID"`
	CreatedAt      string            `json:"CreatedAt"`
	UpdatedAt      string            `json:"UpdatedAt"`
	LastObservedAt string            `json:"LastObservedAt"`
	SourcePath     string            `json:"source_path,omitempty"`
	Retriever      string            `json:"retriever"`
	AuthorityCues  []string          `json:"authority_cues,omitempty"`
	Reasons        []string          `json:"reasons,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

func findResults(candidates []retrieval.Candidate, reasons map[string][]string, retrieverName string) []FindResult {
	results := make([]FindResult, 0, len(candidates))
	for _, c := range candidates {
		results = append(results, FindResult{
			ID:             c.ID,
			RepoID:         metadataValue(c, "repo_id"),
			ShortID:        metadataValue(c, "short_id"),
			Path:           c.Path,
			Kind:           c.Kind,
			Subtype:        c.Subtype,
			Title:          c.Title,
			Status:         c.Status,
			CurrentRevID:   metadataValue(c, "current_revision_id"),
			CreatedAt:      metadataValue(c, "created_at"),
			UpdatedAt:      metadataValue(c, "updated_at"),
			LastObservedAt: metadataValue(c, "last_observed_at"),
			SourcePath:     c.Source,
			Retriever:      retrieverName,
			AuthorityCues:  retrieval.AuthorityCues(c),
			Reasons:        reasons[c.Path],
			Metadata:       compactResultMetadata(c.Metadata),
		})
	}
	return results
}

func compactResultMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	keys := []string{
		"section_pack_mode",
		"section_pack_source",
		"section_pack_count",
		"section_pack_total",
		"section_pack_headings",
		"indexed_section_retrieval_mode",
		"indexed_section_match_count",
		"indexed_section_total",
		"indexed_section_match_ids_json",
		"indexed_section_match_headings_json",
		"indexed_section_match_ranges_json",
		"anchor_first_score",
		"anchor_first_mode",
		"anchor_matches_json",
		"anchor_fields_json",
		"anchor_types_json",
		"anchor_term_frequency_json",
		"anchor_first_backfill",
	}
	out := map[string]string{}
	for _, key := range keys {
		if value := metadata[key]; value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func reasonsByPath(reasons []retrieval.Reason) map[string][]string {
	out := make(map[string][]string, len(reasons))
	for _, reason := range reasons {
		out[reason.Path] = reason.Reasons
	}
	return out
}

func metadataValue(c retrieval.Candidate, key string) string {
	if c.Metadata == nil {
		return ""
	}
	return c.Metadata[key]
}
