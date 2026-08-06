package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// NewPruneCmd creates the ds prune maintenance command.
func NewPruneCmd() *cobra.Command {
	var dryRun, vacuum, asJSON bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove index data for repository roots that no longer exist",
		Long: `Remove stale repository roots from the global DevSpecs index.

A logical repository is removed only when none of its recorded roots still
exist. Deleted SQLite pages are reusable immediately; pass --vacuum to compact
the database file and return unused space to the filesystem.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrune(cmd, dryRun, vacuum, asJSON)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report stale repositories without changing the index")
	cmd.Flags().BoolVar(&vacuum, "vacuum", false, "Compact the database after pruning to reclaim filesystem space")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runPrune(cmd *cobra.Command, dryRun, vacuum, asJSON bool) error {
	started := time.Now()
	success := false
	props := map[string]any{"dry_run": dryRun, "vacuum": vacuum, "json": asJSON}
	defer func() {
		telemetry.RecordCommand("prune", success, time.Since(started), props)
	}()
	if dryRun && vacuum {
		return fmt.Errorf("--dry-run and --vacuum cannot be used together")
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			report := store.PruneReport{DryRun: dryRun, StaleRoots: []string{}}
			success = true
			return outputPruneReport(cmd, report, asJSON)
		}
		return fmt.Errorf("inspect index: %w", err)
	}
	db, err := openDBAtPath(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	report, err := db.Prune(store.PruneOptions{DryRun: dryRun, Vacuum: vacuum})
	if err != nil {
		return store.FriendlySQLiteBusyError(err)
	}
	props["repositories_pruned_bucket"] = telemetry.CountBucket(report.RepositoriesPruned)
	props["artifacts_pruned_bucket"] = telemetry.CountBucket(report.ArtifactsPruned)
	success = true
	return outputPruneReport(cmd, report, asJSON)
}

func outputPruneReport(cmd *cobra.Command, report store.PruneReport, asJSON bool) error {
	if report.StaleRoots == nil {
		report.StaleRoots = []string{}
	}
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	out := cmd.OutOrStdout()
	verb := "Removed"
	if report.DryRun {
		verb = "Would remove"
	}
	fmt.Fprintln(out, "DevSpecs index prune")
	fmt.Fprintf(out, "Stale roots: %d\n", len(report.StaleRoots))
	fmt.Fprintf(out, "%s repositories: %d\n", verb, report.RepositoriesPruned)
	fmt.Fprintf(out, "%s artifacts: %d\n", verb, report.ArtifactsPruned)
	fmt.Fprintf(out, "Index size: %s", formatByteSize(report.BytesBefore))
	if report.Vacuumed {
		fmt.Fprintf(out, " -> %s after vacuum", formatByteSize(report.BytesAfter))
	}
	fmt.Fprintln(out)
	if !report.DryRun && report.RepositoriesPruned > 0 && !report.Vacuumed {
		fmt.Fprintln(out, "Freed pages are reusable by SQLite. Run `ds prune --vacuum` to return unused space to the filesystem.")
	}
	return nil
}

func formatByteSize(size int64) string {
	const unit = int64(1024)
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	divisor := unit
	label := "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		if size < divisor*unit {
			break
		}
		divisor *= unit
		label = next
	}
	return fmt.Sprintf("%.1f %s", float64(size)/float64(divisor), label)
}
