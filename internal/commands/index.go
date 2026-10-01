package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// NewIndexCmd creates explicit local-index recovery commands.
func NewIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Back up, rebuild, or restore the local index",
		Long: `Manage the rebuildable local SQLite index without changing repository files.

Backup works across DevSpecs schema versions. Rebuild uses the same full cold
scan as ds scan, validates a staged replacement, and preserves the displaced
index. Restore publishes an exact backup snapshot without migrating it.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newIndexBackupCmd())
	cmd.AddCommand(newIndexRebuildCmd())
	cmd.AddCommand(newIndexRestoreCmd())
	return cmd
}

type indexBackupReport struct {
	Operation              string            `json:"operation"`
	Backup                 store.IndexBackup `json:"backup"`
	RepositoryFilesWritten bool              `json:"repository_files_written"`
}

type indexReplacementReport struct {
	Operation   string                 `json:"operation"`
	RebuiltRoot string                 `json:"rebuilt_root,omitempty"`
	Replacement store.IndexReplacement `json:"replacement"`
}

func newIndexBackupCmd() *cobra.Command {
	var outputPath string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a verified snapshot of the local index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIndexBackup(cmd, outputPath, asJSON)
		},
	}
	cmd.Flags().StringVar(&outputPath, "output", "", "Write the backup to this file instead of the managed backup directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newIndexRebuildCmd() *cobra.Command {
	var path string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Build and validate a fresh index before replacing the active one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIndexRebuild(cmd, path, asJSON)
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "Repository path to rebuild from")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func newIndexRestoreCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "restore <backup-file>",
		Short: "Restore an exact index snapshot and preserve the displaced index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIndexRestore(cmd, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runIndexBackup(cmd *cobra.Command, outputPath string, asJSON bool) error {
	started := time.Now()
	success := false
	defer func() {
		telemetry.RecordCommand("index_backup", success, time.Since(started), nil)
	}()
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve index path: %w", err)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), explicitScanDeadline)
	defer cancel()
	var noticeCmd *cobra.Command
	if !asJSON {
		noticeCmd = cmd
	}
	writeIndexDeadline(noticeCmd, "Index backup", explicitScanDeadlineLabel)
	lease, err := store.AcquireIndexWriter(ctx, dbPath, indexWaitNotice(noticeCmd, "Index backup"))
	if err != nil {
		return indexOperationError("index backup writer wait", explicitScanDeadlineLabel, err)
	}
	defer func() { _ = lease.Release() }()
	record, err := store.CreateIndexBackup(ctx, dbPath, outputPath, store.IndexBackupReasonManual, store.IndexBackupRetentionManual)
	if err != nil {
		return indexOperationError("index backup", explicitScanDeadlineLabel, err)
	}
	report := indexBackupReport{Operation: "backup", Backup: record, RepositoryFilesWritten: false}
	success = true
	return outputIndexBackupReport(cmd, report, asJSON)
}

func runIndexRebuild(cmd *cobra.Command, path string, asJSON bool) error {
	started := time.Now()
	success := false
	defer func() {
		telemetry.RecordCommand("index_rebuild", success, time.Since(started), nil)
	}()
	repoRoot, err := resolveRepoRoot(path)
	if err != nil {
		return err
	}
	originalOutput := cmd.OutOrStdout()
	cmd.SetOut(io.Discard)
	var replacement *store.IndexReplacement
	err = runScanWithRecovery(cmd, path, false, asJSON, false, false, true, false, false, false, false, false, false, false, false, false, false, false, false, &replacement)
	cmd.SetOut(originalOutput)
	if err != nil {
		return err
	}
	if replacement == nil {
		return fmt.Errorf("index rebuild completed without a replacement report")
	}
	report := indexReplacementReport{Operation: "rebuild", RebuiltRoot: repoRoot, Replacement: *replacement}
	success = true
	return outputIndexReplacementReport(cmd, report, asJSON)
}

func runIndexRestore(cmd *cobra.Command, backupPath string, asJSON bool) error {
	started := time.Now()
	success := false
	defer func() {
		telemetry.RecordCommand("index_restore", success, time.Since(started), nil)
	}()
	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve index path: %w", err)
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), explicitScanDeadline)
	defer cancel()
	var noticeCmd *cobra.Command
	if !asJSON {
		noticeCmd = cmd
	}
	writeIndexDeadline(noticeCmd, "Index restore", explicitScanDeadlineLabel)
	lease, err := store.AcquireIndexWriter(ctx, dbPath, indexWaitNotice(noticeCmd, "Index restore"))
	if err != nil {
		return indexOperationError("index restore writer wait", explicitScanDeadlineLabel, err)
	}
	defer func() { _ = lease.Release() }()
	replacement, err := store.RestoreIndex(ctx, dbPath, backupPath)
	if err != nil {
		return indexOperationError("index restore", explicitScanDeadlineLabel, err)
	}
	report := indexReplacementReport{Operation: "restore", Replacement: replacement}
	success = true
	return outputIndexReplacementReport(cmd, report, asJSON)
}

func outputIndexBackupReport(cmd *cobra.Command, report indexBackupReport, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "DevSpecs index backup")
	fmt.Fprintf(out, "Backup: %s\n", report.Backup.Path)
	fmt.Fprintf(out, "Manifest: %s\n", report.Backup.ManifestPath)
	fmt.Fprintf(out, "Schema: v%d\n", report.Backup.SourceSchema)
	fmt.Fprintf(out, "Size: %s\n", formatByteSize(report.Backup.Bytes))
	fmt.Fprintln(out, "Integrity: ok")
	fmt.Fprintln(out, "Repository files written: false")
	return nil
}

func outputIndexReplacementReport(cmd *cobra.Command, report indexReplacementReport, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "DevSpecs index %s\n", report.Operation)
	if report.RebuiltRoot != "" {
		fmt.Fprintf(out, "Rebuilt repository: %s\n", report.RebuiltRoot)
	}
	fmt.Fprintf(out, "Active index: %s\n", report.Replacement.ActivePath)
	fmt.Fprintf(out, "Schema: v%d (%s)\n", report.Replacement.ActiveSchema, report.Replacement.Compatibility)
	if report.Replacement.DisplacedIndexBackup != nil {
		fmt.Fprintf(out, "Previous index backup: %s\n", report.Replacement.DisplacedIndexBackup.Path)
	}
	if report.Replacement.PreservedIndexPath != "" {
		fmt.Fprintf(out, "Previous unreadable index preserved: %s\n", report.Replacement.PreservedIndexPath)
	}
	fmt.Fprintln(out, "Repository files written: false")
	switch report.Replacement.Compatibility {
	case store.IndexCompatibilityOlder:
		fmt.Fprintln(out, "Next: launch the older CLI now; a normal current-CLI command will migrate this index forward again.")
	case store.IndexCompatibilityNewer:
		fmt.Fprintln(out, "Next: update DevSpecs before normal indexed commands, or run `ds index rebuild` for an intentional downgrade.")
	}
	return nil
}
