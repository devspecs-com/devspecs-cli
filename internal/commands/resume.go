package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewResumeCmd creates the ds resume command.
func NewResumeCmd() *cobra.Command {
	var (
		asJSON    bool
		noRefresh bool
		limit     int
		all       bool
		tag       string
		branch    string
		user      string
		repoName  string
	)

	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Show artifacts grouped by lifecycle phase — continue where you left off",
		RunE: func(cmd *cobra.Command, args []string) error {
			fp := store.FilterParams{Tag: tag, Branch: branch, User: user}
			return runResume(cmd, fp, repoName, asJSON, noRefresh, limit, all)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max items per group (0 = unlimited)")
	cmd.Flags().BoolVar(&all, "all", false, "Remove recency/count caps on settled group")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	return cmd
}

var settledStatuses = map[string]bool{
	"completed": true, "implemented": true, "approved": true,
	"accepted": true, "rejected": true, "cancelled": true, "superseded": true,
}

func runResume(cmd *cobra.Command, fp store.FilterParams, repoName string, asJSON, noRefresh bool, limit int, all bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	wd, _ := os.Getwd()
	repoRoot := resolveRepoRootFromWd(wd)

	if repoName != "" {
		repoRoot = resolveRepoRootByName(db, repoName)
	}

	rows, err := db.ResumeArtifacts(repoRoot, fp)
	if err != nil {
		return fmt.Errorf("resume query: %w", err)
	}

	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No DevSpecs indexed yet. Run: ds scan")
		return nil
	}

	now := time.Now()
	fourteenDays := now.AddDate(0, 0, -14)
	thirtyDays := now.AddDate(0, 0, -30)

	var inProgress, settled, stale []store.ResumeRow

	for _, r := range rows {
		observed, _ := time.Parse(time.RFC3339, r.LastObservedAt)
		terminal := settledStatuses[r.Status]
		old := !observed.IsZero() && observed.Before(thirtyDays)

		if terminal {
			if all || observed.After(fourteenDays) {
				settled = append(settled, r)
			}
		} else if old {
			stale = append(stale, r)
		} else {
			inProgress = append(inProgress, r)
		}
	}

	if limit > 0 {
		inProgress = capSlice(inProgress, limit)
		settled = capSlice(settled, limit)
		stale = capSlice(stale, limit)
	}
	if !all && len(settled) > 10 {
		settled = settled[:10]
	}

	if asJSON {
		obj := map[string]any{
			"in_progress":      resumeRowsToJSON(inProgress),
			"recently_settled": resumeRowsToJSON(settled),
			"stale":            resumeRowsToJSON(stale),
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	out := cmd.OutOrStdout()
	repoBasename := repoRoot
	if idx := strings.LastIndex(repoRoot, "/"); idx >= 0 {
		repoBasename = repoRoot[idx+1:]
	}
	if idx := strings.LastIndex(repoBasename, "\\"); idx >= 0 {
		repoBasename = repoBasename[idx+1:]
	}
	fmt.Fprintf(out, "DevSpecs Resume (%s)\n", repoBasename)

	counter := 1
	if len(inProgress) > 0 {
		fmt.Fprintf(out, "\nIn Progress (%d)\n", len(inProgress))
		for _, r := range inProgress {
			writeInProgressItem(out, &counter, r, now)
		}
	}

	if len(settled) > 0 {
		fmt.Fprintf(out, "\nRecently Settled (%d)\n", len(settled))
		for _, r := range settled {
			writeSettledItem(out, &counter, r, now)
		}
	}

	if len(stale) > 0 {
		fmt.Fprintf(out, "\nStale (%d)\n", len(stale))
		for _, r := range stale {
			writeStaleItem(out, &counter, r, now)
		}
	}

	return nil
}

func writeInProgressItem(w io.Writer, idx *int, r store.ResumeRow, now time.Time) {
	observed, _ := time.Parse(time.RFC3339, r.LastObservedAt)
	sid := shortOrTruncated(r)

	fmt.Fprintf(w, "\n%2d. %s  %s\n", *idx, sid, r.Title)
	fmt.Fprintf(w, "    Status: %s  |  Kind: %s\n", r.Status, r.Kind)
	if r.SourcePath != "" {
		fmt.Fprintf(w, "    Source: %s\n", r.SourcePath)
	}
	if line := formatResumeTagsLine(r.TagsJoined); line != "" {
		fmt.Fprint(w, line)
	}
	if r.TotalTodos > 0 {
		fmt.Fprintf(w, "    Todos:  %d open / %d total\n", r.OpenTodos, r.TotalTodos)
	}
	fmt.Fprintf(w, "    Last observed: %s (%s)\n", relativeTime(observed, now), r.LastObservedAt)
	fmt.Fprintf(w, "    Continue: ds context %s\n", sid)
	*idx++
}

func writeSettledItem(w io.Writer, idx *int, r store.ResumeRow, now time.Time) {
	observed, _ := time.Parse(time.RFC3339, r.LastObservedAt)
	sid := shortOrTruncated(r)

	fmt.Fprintf(w, "\n%2d. %s  %s\n", *idx, sid, r.Title)
	fmt.Fprintf(w, "    Status: %s  |  Kind: %s\n", r.Status, r.Kind)
	if r.SourcePath != "" {
		fmt.Fprintf(w, "    Source: %s\n", r.SourcePath)
	}
	if line := formatResumeTagsLine(r.TagsJoined); line != "" {
		fmt.Fprint(w, line)
	}
	fmt.Fprintf(w, "    Settled: %s (%s)\n", relativeTime(observed, now), r.LastObservedAt)
	fmt.Fprintf(w, "    Next: verify manually, or ds context %s for downstream work\n", sid)
	*idx++
}

func writeStaleItem(w io.Writer, idx *int, r store.ResumeRow, now time.Time) {
	observed, _ := time.Parse(time.RFC3339, r.LastObservedAt)
	sid := shortOrTruncated(r)

	fmt.Fprintf(w, "\n%2d. %s  %s\n", *idx, sid, r.Title)
	fmt.Fprintf(w, "    Status: %s  |  Kind: %s\n", r.Status, r.Kind)
	if r.SourcePath != "" {
		fmt.Fprintf(w, "    Source: %s\n", r.SourcePath)
	}
	if line := formatResumeTagsLine(r.TagsJoined); line != "" {
		fmt.Fprint(w, line)
	}
	fmt.Fprintf(w, "    Last observed: %s — consider archiving or updating\n", relativeTime(observed, now))
	*idx++
}

func formatResumeTagsLine(tagsJoined string) string {
	if strings.TrimSpace(tagsJoined) == "" {
		return ""
	}
	return fmt.Sprintf("    Tags: %s\n", tagsJoined)
}

func shortOrTruncated(r store.ResumeRow) string {
	if r.ShortID != "" {
		return r.ShortID
	}
	if len(r.ID) > 8 {
		return r.ID[:8]
	}
	return r.ID
}

func relativeTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 48*time.Hour:
		return "yesterday"
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}
}

func resumeRowsToJSON(rows []store.ResumeRow) []map[string]any {
	result := make([]map[string]any, len(rows))
	for i, r := range rows {
		result[i] = map[string]any{
			"id":               r.ID,
			"short_id":         r.ShortID,
			"kind":             r.Kind,
			"title":            r.Title,
			"status":           r.Status,
			"last_observed_at": r.LastObservedAt,
			"source_path":      r.SourcePath,
			"tags":             splitResumeTagsCSV(r.TagsJoined),
			"total_todos":      r.TotalTodos,
			"open_todos":       r.OpenTodos,
		}
	}
	return result
}

func capSlice(rows []store.ResumeRow, limit int) []store.ResumeRow {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func splitResumeTagsCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ", ") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
