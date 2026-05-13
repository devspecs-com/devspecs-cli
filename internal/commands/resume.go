package commands

import (
	"encoding/json"
	"fmt"
	"io"
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
	cmd.Flags().IntVar(&limit, "limit", 5, "Max items per group (0 = unlimited)")
	cmd.Flags().BoolVar(&all, "all", false, "Include settled artifacts older than 14 days (recency filter only)")
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

	repoRoot := resolveRepoScope(db, repoName, false)

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
		updated, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		terminal := settledStatuses[r.Status]
		// Idle clock for non-terminal work: authored_at when present, else updated_at.
		// Do not use last_observed_at — it refreshes on every scan and would flood "In Progress".
		idle := resumeStaleIdleTime(r)
		old := !idle.IsZero() && idle.Before(thirtyDays)

		if terminal {
			if all || (!updated.IsZero() && updated.After(fourteenDays)) {
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
	authored, _ := time.Parse(time.RFC3339, r.AuthoredAt)
	updated, _ := time.Parse(time.RFC3339, r.UpdatedAt)
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
	fmt.Fprintf(w, "    Authored: %s (%s)\n", relativeTime(authored, now), r.AuthoredAt)
	fmt.Fprintf(w, "    Last updated: %s (%s)\n", relativeTime(updated, now), r.UpdatedAt)
	fmt.Fprintf(w, "    Continue: ds context %s\n", sid)
	*idx++
}

func writeSettledItem(w io.Writer, idx *int, r store.ResumeRow, now time.Time) {
	updated, _ := time.Parse(time.RFC3339, r.UpdatedAt)
	sid := shortOrTruncated(r)

	fmt.Fprintf(w, "\n%2d. %s  %s\n", *idx, sid, r.Title)
	fmt.Fprintf(w, "    Status: %s  |  Kind: %s\n", r.Status, r.Kind)
	if r.SourcePath != "" {
		fmt.Fprintf(w, "    Source: %s\n", r.SourcePath)
	}
	if line := formatResumeTagsLine(r.TagsJoined); line != "" {
		fmt.Fprint(w, line)
	}
	fmt.Fprintf(w, "    Settled: %s (%s)\n", relativeTime(updated, now), r.UpdatedAt)
	fmt.Fprintf(w, "    Next: verify manually, or ds context %s for downstream work\n", sid)
	*idx++
}

func writeStaleItem(w io.Writer, idx *int, r store.ResumeRow, now time.Time) {
	authored, _ := time.Parse(time.RFC3339, r.AuthoredAt)
	updated, _ := time.Parse(time.RFC3339, r.UpdatedAt)
	observed, _ := time.Parse(time.RFC3339, r.LastObservedAt)
	idle := resumeStaleIdleTime(r)
	sid := shortOrTruncated(r)

	fmt.Fprintf(w, "\n%2d. %s  %s\n", *idx, sid, r.Title)
	fmt.Fprintf(w, "    Status: %s  |  Kind: %s\n", r.Status, r.Kind)
	if r.SourcePath != "" {
		fmt.Fprintf(w, "    Source: %s\n", r.SourcePath)
	}
	if line := formatResumeTagsLine(r.TagsJoined); line != "" {
		fmt.Fprint(w, line)
	}
	fmt.Fprintf(w, "    Authored: %s (%s)\n", relativeTime(authored, now), r.AuthoredAt)
	fmt.Fprintf(w, "    Last updated: %s (%s)\n", relativeTime(updated, now), r.UpdatedAt)
	if !idle.IsZero() {
		fmt.Fprintf(w, "    Idle (stale) since: %s — consider archiving or updating\n", relativeTime(idle, now))
	}
	if !observed.IsZero() {
		fmt.Fprintf(w, "    Last observed: %s (%s)\n", relativeTime(observed, now), r.LastObservedAt)
	}
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
			"authored_at":      r.AuthoredAt,
			"updated_at":       r.UpdatedAt,
			"last_observed_at": r.LastObservedAt,
			"source_path":      r.SourcePath,
			"tags":             splitResumeTagsCSV(r.TagsJoined),
			"total_todos":      r.TotalTodos,
			"open_todos":       r.OpenTodos,
		}
	}
	return result
}

// resumeStaleIdleTime returns the clock used to decide whether a non-terminal artifact
// is stale (>30d idle). Prefers authored_at; falls back to updated_at when authored is missing.
func resumeStaleIdleTime(r store.ResumeRow) time.Time {
	if a, err := time.Parse(time.RFC3339, strings.TrimSpace(r.AuthoredAt)); err == nil && !a.IsZero() {
		return a
	}
	u, _ := time.Parse(time.RFC3339, strings.TrimSpace(r.UpdatedAt))
	return u
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
