package commands

import (
	"fmt"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

func runHubPrune(cmd *cobra.Command, before string, dryRun, vacuum, asJSON bool) error {
	if before == "" {
		return fmt.Errorf("--hub requires --before <RFC3339>")
	}
	if vacuum {
		return fmt.Errorf("--vacuum is not yet supported with --hub; deleted pages remain reusable by SQLite")
	}
	cutoff, err := time.Parse(time.RFC3339, before)
	if err != nil {
		return fmt.Errorf("--before must be RFC3339: %w", err)
	}
	cutoff = cutoff.UTC()
	if !cutoff.Before(time.Now().UTC()) {
		return fmt.Errorf("--before must be in the past")
	}
	open := hubstore.Open
	if dryRun {
		open = hubstore.OpenReadOnly
	}
	db, err := open(cmd.Context(), hubstore.Options{})
	if err != nil {
		return err
	}
	defer db.Close()
	var report hubstore.PruneReport
	if dryRun {
		report, err = db.PlanHubPrune(cmd.Context(), cutoff)
	} else {
		report, err = db.PruneHub(cmd.Context(), cutoff)
	}
	if err != nil {
		return err
	}
	if asJSON {
		return writeHubJSON(cmd, report)
	}
	verb := "Removed"
	if dryRun {
		verb = "Would remove"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "DevSpecs hub prune\nCutoff: %s\n%s entries: %d\nPayload: %s\n", cutoff.Format(time.RFC3339), verb, report.Entries, formatByteSize(report.PayloadBytes)); err != nil {
		return err
	}
	if report.More {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "More eligible entries remain; repeat with the same cutoff."); err != nil {
			return err
		}
	}
	if report.BackupPath != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Backup: %s\n", report.BackupPath); err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Deleted pages are reusable by SQLite; the hub file is not vacuumed.")
		return err
	}
	return nil
}
