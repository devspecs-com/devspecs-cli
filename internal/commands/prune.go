package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const pruneProgressDelay = 750 * time.Millisecond

// NewPruneCmd creates the ds prune maintenance command.
func NewPruneCmd() *cobra.Command {
	var dryRun, vacuum, asJSON bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove stale and redundant data from the local index",
		Long: `Remove stale repository roots and redundant capture history from the
global DevSpecs index.

A logical repository is removed only when none of its recorded roots still
exist. Consecutive capture revisions with identical content are collapsed while
preserving the current revision and distinct content transitions. Deleted
SQLite pages are reusable immediately; pass --vacuum to compact the database
file and return unused space to the filesystem.`,
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
	progress := newPruneProgressReporter(cmd.ErrOrStderr(), !asJSON, pruneProgressDelay)
	defer progress.stop()

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
	var lease *store.IndexWriterLease
	if !dryRun {
		waitCtx, cancel := context.WithTimeout(cmd.Context(), autoIndexDeadline)
		defer cancel()
		var noticeCmd *cobra.Command
		if !asJSON {
			noticeCmd = cmd
		}
		lease, err = store.AcquireIndexWriter(waitCtx, dbPath, indexWaitNotice(noticeCmd, "Prune"))
		if err != nil {
			return indexOperationError("prune writer wait", autoIndexDeadlineLabel, err)
		}
		defer func() { _ = lease.Release() }()
	}
	progress.setPhase("inspect")
	db, err := openDBAtPath(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	report, err := db.Prune(store.PruneOptions{
		DryRun: dryRun,
		Vacuum: vacuum,
		Progress: func(event store.PruneProgress) {
			progress.setPhase(event.Phase)
		},
	})
	if err != nil {
		return store.FriendlySQLiteBusyError(err)
	}
	props["repositories_pruned_bucket"] = telemetry.CountBucket(report.RepositoriesPruned)
	props["artifacts_pruned_bucket"] = telemetry.CountBucket(report.ArtifactsPruned)
	props["revisions_compacted_bucket"] = telemetry.CountBucket(report.RevisionsCompacted)
	success = true
	return outputPruneReport(cmd, report, asJSON)
}

type pruneProgressReporter struct {
	mu      sync.Mutex
	out     io.Writer
	enabled bool
	delay   time.Duration
	started time.Time
	phase   string
	emitted bool
	stopped bool
	timer   *time.Timer
}

func newPruneProgressReporter(out io.Writer, enabled bool, delay time.Duration) *pruneProgressReporter {
	return &pruneProgressReporter{out: out, enabled: enabled, delay: delay, started: time.Now()}
}

func (p *pruneProgressReporter) setPhase(phase string) {
	if p == nil || !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || phase == "" || phase == p.phase {
		return
	}
	p.phase = phase
	if phase == "complete" {
		if p.emitted {
			fmt.Fprintf(p.out, "Prune progress: complete (%s)\n", formatProgressDuration(time.Since(p.started)))
		}
		p.stopTimerLocked()
		return
	}
	if phase == "compact" {
		p.stopTimerLocked()
		p.emitPhaseLocked()
		return
	}
	if p.emitted {
		p.emitPhaseLocked()
		return
	}
	if p.timer == nil {
		p.timer = time.AfterFunc(p.delay, p.emitCurrentPhase)
	}
}

func (p *pruneProgressReporter) emitCurrentPhase() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timer = nil
	if p.stopped || p.phase == "" || p.phase == "complete" {
		return
	}
	p.emitPhaseLocked()
}

func (p *pruneProgressReporter) emitPhaseLocked() {
	labels := map[string]string{
		"inspect": "inspecting index",
		"delete":  "removing stale index data",
		"compact": "compacting index",
	}
	label := labels[p.phase]
	if label == "" {
		return
	}
	fmt.Fprintf(p.out, "Prune progress: %s\n", label)
	p.emitted = true
}

func (p *pruneProgressReporter) stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	p.stopTimerLocked()
}

func (p *pruneProgressReporter) stopTimerLocked() {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

func formatProgressDuration(duration time.Duration) string {
	if duration < time.Second {
		return duration.Round(100 * time.Millisecond).String()
	}
	return duration.Round(time.Second).String()
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
	compactVerb := "Compacted"
	if report.DryRun {
		compactVerb = "Would compact"
	}
	fmt.Fprintf(out, "%s duplicate capture revisions: %d\n", compactVerb, report.RevisionsCompacted)
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
