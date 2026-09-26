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
	if vacuum && dryRun {
		return fmt.Errorf("--dry-run and --vacuum cannot be used together")
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
	if vacuum {
		compaction, compactErr := db.VacuumHub(cmd.Context())
		if compactErr != nil {
			backup := compaction.BackupPath
			if backup == "" {
				backup = report.BackupPath
			}
			if backup == "" {
				return fmt.Errorf("hub prune committed %d entries; vacuum failed before backup: %w", report.Entries, compactErr)
			}
			return fmt.Errorf("hub prune committed %d entries; vacuum failed (verified backup: %s): %w", report.Entries, backup, compactErr)
		}
		report.Vacuum = &compaction
		report.BackupPath = compaction.BackupPath
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
		if report.Vacuum != nil {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Hub file: %s -> %s; reclaimed: %s\n", formatByteSize(report.Vacuum.BytesBefore), formatByteSize(report.Vacuum.BytesAfter), formatByteSize(report.Vacuum.ReclaimedBytes))
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Deleted pages are reusable by SQLite; the hub file is not vacuumed.")
		return err
	}
	return nil
}
