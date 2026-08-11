package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/freshness"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

const (
	autoIndexDeadline         = 10 * time.Minute
	autoIndexDeadlineLabel    = "10m"
	explicitScanDeadline      = 30 * time.Minute
	explicitScanDeadlineLabel = "30m"
)

// ensureFresh checks if the index is stale and auto-scans if needed.
// Resolves the repo root by walking up from cwd (via .git or .devspecs/).
// Prints a one-line notice to stderr when updates occur.
func ensureFresh(cmd *cobra.Command, db *store.DB) error {
	wd, err := os.Getwd()
	if err != nil {
		debugLog("ensureFresh: Getwd failed: %v", err)
		return nil
	}

	repoRoot := resolveIndexedRepoRoot(db, wd)
	if repoRoot == "" {
		repoRoot = canonicalRepoRoot(resolveRepoRootFromWd(wd))
	}
	debugLog("ensureFresh: wd=%s resolved_root=%s", wd, repoRoot)

	status, err := freshness.CheckContext(cmd.Context(), db, repoRoot)
	if err != nil {
		return err
	}
	if status == nil {
		debugLog("ensureFresh: no repo row found for %s — skipping", repoRoot)
		return nil
	}
	if !status.Stale {
		debugLog("ensureFresh: index is fresh for %s", repoRoot)
		return nil
	}

	debugLog("ensureFresh: stale — reason=%s, triggering auto-scan", status.Reason)
	if !commandSuppressNonResultProgress(cmd) {
		maybeWarnWorkspaceRoot(cmd, repoRoot)
	}
	return runScanQuietAndNotify(cmd, db, repoRoot)
}

func ensureRepoIndexed(cmd *cobra.Command, db *store.DB, repoRoot string) error {
	repoRoot = canonicalRepoRoot(repoRoot)
	if repoRoot == "" {
		return nil
	}
	status, err := freshness.CheckContext(cmd.Context(), db, repoRoot)
	if err != nil {
		return err
	}
	if status != nil && !status.Stale {
		debugLog("ensureRepoIndexed: index is fresh for %s", repoRoot)
		return nil
	}
	if status == nil {
		debugLog("ensureRepoIndexed: no repo row for %s; triggering auto-scan", repoRoot)
	} else {
		debugLog("ensureRepoIndexed: stale reason=%s; triggering auto-scan", status.Reason)
	}
	if !commandSuppressNonResultProgress(cmd) {
		maybeWarnWorkspaceRoot(cmd, repoRoot)
	}
	return runScanQuietAndNotify(cmd, db, repoRoot)
}

func ensureRepoIndexedForTask(cmd *cobra.Command, db *store.DB, repoRoot string) error {
	repoRoot = canonicalRepoRoot(repoRoot)
	if repoRoot == "" {
		return nil
	}
	status, err := freshness.CheckContext(cmd.Context(), db, repoRoot)
	if err != nil {
		return err
	}
	substrateReady, substrateReason := taskIndexSubstrateReady(db, repoRoot)
	if status != nil && !status.Stale && substrateReady {
		debugLog("ensureRepoIndexedForTask: task substrate is fresh for %s", repoRoot)
		return nil
	}
	if status == nil {
		debugLog("ensureRepoIndexedForTask: no repo row for %s; triggering task auto-scan", repoRoot)
	} else if status.Stale {
		debugLog("ensureRepoIndexedForTask: stale reason=%s; triggering task auto-scan", status.Reason)
	} else {
		debugLog("ensureRepoIndexedForTask: substrate reason=%s; triggering task auto-scan", substrateReason)
	}
	if !commandSuppressNonResultProgress(cmd) {
		maybeWarnWorkspaceRoot(cmd, repoRoot)
	}
	return runTaskScanQuietAndNotify(cmd, db, repoRoot)
}

func taskIndexSubstrateReady(db *store.DB, repoRoot string) (bool, string) {
	meta := db.GetRepoByRoot(canonicalRepoRoot(repoRoot))
	if meta == nil {
		return false, "repo not indexed"
	}
	sourceContextCount := countRepoSourcesByType(db, meta.ID, "source_context")
	if sourceContextCount == 0 {
		return true, ""
	}
	counts, err := db.CountSourceManifest(meta.ID)
	if err != nil {
		return false, "source manifest count failed"
	}
	if counts.Files == 0 {
		return false, "source manifest missing"
	}
	return true, ""
}

func countRepoSourcesByType(db *store.DB, repoID, sourceType string) int {
	var count int
	if err := db.QueryRow("SELECT COUNT(DISTINCT artifact_id) FROM sources WHERE repo_id = ? AND source_type = ?", repoID, sourceType).Scan(&count); err != nil {
		return 0
	}
	return count
}

func runScanQuietAndNotify(cmd *cobra.Command, db *store.DB, repoRoot string) error {
	quiet := commandSuppressNonResultProgress(cmd)
	scanCmd := cmd
	if quiet {
		scanCmd = nil
	}
	result, err := runScanQuiet(cmd.Context(), scanCmd, db, repoRoot)
	if err != nil {
		return err
	}
	if !quiet && result != nil && (result.New > 0 || result.Updated > 0) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Index updated (%d new, %d updated)\n", result.New, result.Updated)
	}
	return nil
}

func runTaskScanQuietAndNotify(cmd *cobra.Command, db *store.DB, repoRoot string) error {
	quiet := commandSuppressNonResultProgress(cmd)
	scanCmd := cmd
	if quiet {
		scanCmd = nil
	}
	result, err := runTaskScanQuiet(cmd.Context(), scanCmd, db, repoRoot)
	if err != nil {
		return err
	}
	if !quiet && result != nil && (result.New > 0 || result.Updated > 0) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Task index updated (%d new, %d updated)\n", result.New, result.Updated)
	}
	return nil
}

// resolveRepoRootFromWd finds the project root by checking for .git or .devspecs/
// walking upward from the given directory.
func resolveRepoRootFromWd(wd string) string {
	info := repo.Detect(wd)
	if info.IsGit {
		return info.RootPath
	}
	if root := findDevspecsRoot(wd); root != "" {
		return root
	}
	return wd
}

func findDevspecsRoot(dir string) string {
	current := dir
	for {
		candidate := filepath.Join(current, ".devspecs")
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func runScanQuiet(parent context.Context, cmd *cobra.Command, db *store.DB, repoRoot string) (*scan.Result, error) {
	ctx, cancel := context.WithTimeout(parent, autoIndexDeadline)
	defer cancel()
	writeIndexDeadline(cmd, "Auto-index", autoIndexDeadlineLabel)
	lease, err := db.AcquireIndexWriter(ctx, indexWaitNotice(cmd, "Auto-index"))
	if err != nil {
		return nil, indexOperationError("auto-index", autoIndexDeadlineLabel, err)
	}
	defer func() { _ = lease.Release() }()
	status, err := freshness.CheckContext(ctx, db, canonicalRepoRoot(repoRoot))
	if err != nil {
		return nil, indexOperationError("auto-index", autoIndexDeadlineLabel, err)
	}
	if status != nil && !status.Stale {
		debugLog("runScanQuiet: queued refresh became redundant for %s", repoRoot)
		return nil, nil
	}
	db.SetMaxOpenConns(1)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		debugLog("runScanQuiet: LoadRepoConfig error: %v", err)
	}
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}, &sourcecontext.Adapter{}}
	if cfg != nil && cfg.TestCaseArtifactsEnabled(false) {
		adpts = append(adpts, &testcase.Adapter{})
	}
	if cfg != nil && cfg.CodeCommentArtifactsEnabled(false) {
		adpts = append(adpts, &codecomment.Adapter{})
	}
	scanner := scan.New(db, ids, adpts)
	scanOpts, err := liveScanRunOptionsContext(ctx, db, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("auto-index options: %w", err)
	}
	if cmd != nil {
		scanOpts.Progress = scanProgressStderr(cmd.ErrOrStderr(), "Auto-index", commandVerboseProgress(cmd))
	}
	result, err := scanner.RunWithOptions(ctx, repoRoot, cfg, scanOpts)
	if err != nil {
		return nil, indexOperationError("auto-index", autoIndexDeadlineLabel, scanTraversalError(repoRoot, err))
	}
	return result, nil
}

func runTaskScanQuiet(parent context.Context, cmd *cobra.Command, db *store.DB, repoRoot string) (*scan.Result, error) {
	ctx, cancel := context.WithTimeout(parent, autoIndexDeadline)
	defer cancel()
	writeIndexDeadline(cmd, "Task auto-index", autoIndexDeadlineLabel)
	lease, err := db.AcquireIndexWriter(ctx, indexWaitNotice(cmd, "Task auto-index"))
	if err != nil {
		return nil, indexOperationError("task auto-index", autoIndexDeadlineLabel, err)
	}
	defer func() { _ = lease.Release() }()
	repoRoot = canonicalRepoRoot(repoRoot)
	status, err := freshness.CheckContext(ctx, db, repoRoot)
	if err != nil {
		return nil, indexOperationError("task auto-index", autoIndexDeadlineLabel, err)
	}
	substrateReady, _ := taskIndexSubstrateReady(db, repoRoot)
	if status != nil && !status.Stale && substrateReady {
		debugLog("runTaskScanQuiet: queued refresh became redundant for %s", repoRoot)
		return nil, nil
	}
	db.SetMaxOpenConns(1)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		debugLog("runTaskScanQuiet: LoadRepoConfig error: %v", err)
	}
	cfg = config.WithDefaultIntentCandidateDiscovery(cfg, true)
	cfg = config.WithTestCaseArtifacts(cfg, true)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}, &sourcecontext.Adapter{}, &testcase.Adapter{}}
	if cfg != nil && cfg.CodeCommentArtifactsEnabled(false) {
		adpts = append(adpts, &codecomment.Adapter{})
	}
	scanner := scan.New(db, ids, adpts)
	scanOpts, err := liveScanRunOptionsContext(ctx, db, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("task auto-index options: %w", err)
	}
	scanOpts.SourceManifest = true
	if cmd != nil {
		scanOpts.Progress = scanProgressStderr(cmd.ErrOrStderr(), "Task auto-index", commandVerboseProgress(cmd))
	}
	result, err := scanner.RunWithOptions(ctx, repoRoot, cfg, scanOpts)
	if err != nil {
		return nil, indexOperationError("task auto-index", autoIndexDeadlineLabel, scanTraversalError(repoRoot, err))
	}
	return result, nil
}

func writeIndexDeadline(cmd *cobra.Command, label, deadlineLabel string) {
	if cmd != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s: starting (deadline %s)\n", label, deadlineLabel)
	}
}

func indexWaitNotice(cmd *cobra.Command, label string) func() {
	if cmd == nil {
		return nil
	}
	return func() {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s: waiting for another index update\n", label)
	}
}

func indexOperationError(label, deadlineLabel string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s exceeded its %s deadline: %w", label, deadlineLabel, err)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s canceled: %w", label, err)
	default:
		return err
	}
}

// debugLog prints to stderr only when DS_DEBUG=1 is set.
func debugLog(format string, args ...any) {
	if os.Getenv("DS_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[ds:debug] "+format+"\n", args...)
	}
}

// canonicalRepoRoot returns an absolute, cleaned path suitable for comparing
// against repos.root_path (which scan stores via filepath.Abs).
func canonicalRepoRoot(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// resolveIndexedRepoRoot picks the repo row to scope to: prefer the canonical
// cwd when it is indexed, otherwise the git/.devspecs walk-up root if indexed.
// This avoids mis-binding to a parent directory's .devspecs when cwd is an
// unrelated tree that was scanned as its own root (e.g. tests under /tmp).
func resolveIndexedRepoRoot(db *store.DB, wd string) string {
	cwdRoot := canonicalRepoRoot(wd)
	if meta := db.GetRepoByRoot(cwdRoot); meta != nil {
		return meta.RootPath
	}
	resolved := canonicalRepoRoot(resolveRepoRootFromWd(wd))
	if meta := db.GetRepoByRoot(resolved); meta != nil {
		return meta.RootPath
	}
	return ""
}

// resolveRepoScope returns the absolute repo root to filter by, or empty string
// when allRepos is true (no repo filter). When repoName is non-empty it overrides
// the root detected from the current working directory.
func resolveRepoScope(db *store.DB, repoName string, allRepos bool) string {
	if allRepos {
		return ""
	}
	if repoName != "" {
		return resolveRepoRootByName(db, repoName)
	}
	wd, _ := os.Getwd()
	if root := resolveIndexedRepoRoot(db, wd); root != "" {
		return root
	}
	return canonicalRepoRoot(resolveRepoRootFromWd(wd))
}

// resolveRepoRootByName finds the repo root_path whose basename matches name.
func resolveRepoRootByName(db *store.DB, name string) string {
	rows, err := db.Query("SELECT root_path FROM repos")
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var rootPath string
		rows.Scan(&rootPath)
		if filepath.Base(rootPath) == name {
			return rootPath
		}
	}
	return ""
}
