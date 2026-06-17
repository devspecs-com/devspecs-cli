package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// NewScanCmd creates the ds scan command.
func NewScanCmd() *cobra.Command {
	var (
		path                           string
		verbose                        bool
		asJSON                         bool
		quiet                          bool
		ifChanged                      bool
		rebuild                        bool
		experimentalIntentDiscovery    bool
		experimentalGitEvidence        bool
		experimentalWorkstreamEvidence bool
		experimentalRichTypedIndex     bool
		experimentalSupportDocs        bool
		experimentalRecentSource       bool
		experimentalFirstPartySource   bool
		experimentalSourceManifest     bool
		includeTests                   bool
		includeCodeComments            bool
		noGitignore                    bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan repository for specs, plans, and ADRs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd, path, verbose, asJSON, quiet, ifChanged, rebuild, experimentalIntentDiscovery, experimentalGitEvidence, experimentalWorkstreamEvidence, experimentalRichTypedIndex, experimentalSupportDocs, experimentalRecentSource, experimentalFirstPartySource, experimentalSourceManifest, includeTests, includeCodeComments, noGitignore)
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to scan")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed scan output")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress human scan summary and empty-scan hints (redundant when --json is set)")
	cmd.Flags().BoolVar(&ifChanged, "if-changed", false, "Only scan if source paths were touched in the last commit")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Remove the global index database and create a fresh index (requires re-scan)")
	cmd.Flags().BoolVar(&experimentalIntentDiscovery, "experimental-intent-discovery", false, "Deprecated: broad scored markdown intent candidate discovery is enabled by default")
	cmd.Flags().BoolVar(&experimentalGitEvidence, "experimental-git-evidence", false, "Index bounded local git history facts as diagnostic evidence")
	cmd.Flags().BoolVar(&experimentalWorkstreamEvidence, "experimental-workstream-evidence", false, "Index bounded local workstream anchors as diagnostic evidence (implies --experimental-git-evidence)")
	cmd.Flags().BoolVar(&experimentalRichTypedIndex, "experimental-rich-typed-index", false, "Index bounded richer source/test/symbol graph evidence as diagnostic substrate")
	cmd.Flags().BoolVar(&experimentalSupportDocs, "experimental-support-docs", false, "Index bounded support docs as diagnostic context")
	cmd.Flags().BoolVar(&experimentalRecentSource, "experimental-recent-source-context", false, "Index bounded recently changed source files as experimental implementation context")
	cmd.Flags().BoolVar(&experimentalFirstPartySource, "experimental-first-party-source-context", false, "Index broad first-party source/test files as experimental implementation context")
	cmd.Flags().BoolVar(&experimentalSourceManifest, "experimental-source-manifest", false, "Index compact first-party source/test metadata as experimental manifest substrate")
	cmd.Flags().BoolVar(&includeTests, "include-tests", false, "Index executable test cases as behavioral intent artifacts")
	cmd.Flags().BoolVar(&includeTests, "experimental-test-cases", false, "Deprecated alias for --include-tests")
	cmd.Flags().BoolVar(&includeCodeComments, "include-code-comments", false, "Index high-signal code comments as implementation intent artifacts")
	cmd.Flags().BoolVar(&noGitignore, "no-gitignore", false, "Do not apply .gitignore, .git/info/exclude, or .aiignore during scan walks")
	_ = cmd.Flags().MarkDeprecated("experimental-test-cases", "use --include-tests")
	_ = cmd.Flags().MarkHidden("experimental-recent-source-context")
	_ = cmd.Flags().MarkHidden("experimental-first-party-source-context")
	_ = cmd.Flags().MarkHidden("experimental-source-manifest")
	return cmd
}

func runScan(cmd *cobra.Command, path string, verbose, asJSON, quiet, ifChanged, rebuild, experimentalIntentDiscovery, experimentalGitEvidence, experimentalWorkstreamEvidence, experimentalRichTypedIndex, experimentalSupportDocs, experimentalRecentSource, experimentalFirstPartySource, experimentalSourceManifest, includeTests, includeCodeComments, noGitignore bool) error {
	start := time.Now()
	success := false
	props := map[string]any{
		"include_tests":                    includeTests,
		"include_code_comments":            includeCodeComments,
		"no_gitignore":                     noGitignore,
		"experimental_git_evidence":        experimentalGitEvidence,
		"experimental_workstream_evidence": experimentalWorkstreamEvidence,
		"experimental_rich_typed_index":    experimentalRichTypedIndex,
		"experimental_support_docs":        experimentalSupportDocs,
		"experimental_recent_source":       experimentalRecentSource,
		"experimental_first_party_source":  experimentalFirstPartySource,
		"experimental_source_manifest":     experimentalSourceManifest,
		"if_changed":                       ifChanged,
		"rebuild":                          rebuild,
		"json":                             asJSON,
		"quiet":                            quiet,
	}
	defer func() {
		telemetry.RecordCommand("scan", success, time.Since(start), props)
	}()

	repoRoot, err := resolveRepoRoot(path)
	if err != nil {
		return err
	}
	rootWarning := maybeWarnWorkspaceRoot(cmd, repoRoot)

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg = config.WithDefaultIntentCandidateDiscovery(cfg, true)
	if experimentalIntentDiscovery {
		cfg = config.WithIntentCandidateDiscovery(cfg, true)
	}
	if experimentalSupportDocs {
		cfg = config.WithSupportDocDiscovery(cfg, true)
	}
	if includeTests {
		cfg = config.WithTestCaseArtifacts(cfg, true)
	}
	if includeCodeComments {
		cfg = config.WithCodeCommentArtifacts(cfg, true)
	}

	if ifChanged && !sourcePathsChanged(repoRoot, cfg) {
		success = true
		props["artifact_count_bucket"] = telemetry.CountBucket(0)
		props["found_any"] = false
		return nil
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}

	if rebuild {
		if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove database for --rebuild: %w", err)
		}
		if verbose && !quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "Removed existing index for rebuild: %s\n", dbPath)
		}
	}

	db, err := openDBAtPath(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}, &sourcecontext.Adapter{}}
	if cfg.TestCaseArtifactsEnabled(false) {
		adpts = append(adpts, &testcase.Adapter{})
	}
	if cfg.CodeCommentArtifactsEnabled(false) {
		adpts = append(adpts, &codecomment.Adapter{})
	}

	scanner := scan.New(db, ids, adpts)
	if verbose && !quiet && noGitignore {
		fmt.Fprintf(cmd.ErrOrStderr(), "Ignoring repo-root .gitignore, .git/info/exclude, and .aiignore during configured walks\n")
	}
	if verbose && !quiet && !noGitignore {
		fmt.Fprintf(cmd.ErrOrStderr(), "Respecting repo-root .gitignore, .git/info/exclude, and .aiignore during configured walks\n")
	}
	scanOpts, err := liveScanRunOptions(db, repoRoot)
	if err != nil {
		return fmt.Errorf("inspect index state: %w", err)
	}
	props["transaction_enabled"] = scanOpts.UseTransaction
	props["fresh_index"] = scanOpts.FreshIndex
	props["skip_authored_at_lookup"] = scanOpts.SkipAuthoredAtLookup
	scanOpts.IncludeGitEvidence = experimentalGitEvidence || experimentalWorkstreamEvidence
	scanOpts.IncludeWorkstreamEvidence = experimentalWorkstreamEvidence
	scanOpts.RichTypedIndex = experimentalRichTypedIndex
	scanOpts.RecentSourceContext = experimentalRecentSource
	scanOpts.FirstPartySourceContext = experimentalFirstPartySource
	scanOpts.SourceManifest = experimentalSourceManifest
	scanOpts.IgnoreRules = noGitignore
	if !quiet {
		scanOpts.Progress = scanProgressStderr(cmd.ErrOrStderr(), "Scan")
	}
	if verbose && !quiet && scanOpts.FreshIndex {
		fmt.Fprintf(cmd.ErrOrStderr(), "Using fresh-index scan path for empty/rebuilt index\n")
	}
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, cfg, scanOpts)
	if err != nil {
		return scanTraversalError(repoRoot, err)
	}
	result.RootWarning = rootWarning

	if scanTotalFound(result) == 0 {
		var matcher *ignore.Matcher
		if !noGitignore {
			matcher, _ = ignore.NewMatcher(repoRoot)
		}
		attachScanHints(result, repoRoot, matcher)
	}
	totalFound := scanTotalFound(result)
	success = true
	props["artifact_count_bucket"] = telemetry.CountBucket(totalFound)
	props["new_count_bucket"] = telemetry.CountBucket(result.New)
	props["updated_count_bucket"] = telemetry.CountBucket(result.Updated)
	props["unchanged_count_bucket"] = telemetry.CountBucket(result.Unchanged)
	props["source_count_bucket"] = telemetry.CountBucket(len(result.SourcesBreakdown))
	props["found_any"] = totalFound > 0

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
		return nil
	}
	if quiet {
		return nil
	}

	out := cmd.OutOrStdout()
	if scanTotalFound(result) == 0 {
		printEmptyScanHints(cmd, repoRoot, result.Hints)
		return nil
	}

	fmt.Fprintf(out, "Scanned repository: %s\n", repoRoot)
	fmt.Fprintln(out, "\nIndexed by source:")
	for _, row := range result.SourcesBreakdown {
		fmt.Fprintf(out, "  %-16s %3d", row.Label, row.Count)
		if s := formatScanFormatsHuman(row.Formats); s != "" {
			fmt.Fprintf(out, "   formats: %s", s)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "\nIndexed:")
	fmt.Fprintf(out, "  %d new artifacts\n", result.New)
	fmt.Fprintf(out, "  %d updated artifacts\n", result.Updated)
	fmt.Fprintf(out, "  %d unchanged artifacts\n", result.Unchanged)
	if result.SourceManifest != nil {
		fmt.Fprintln(out, "\nSource manifest (experimental):")
		fmt.Fprintf(out, "  %d indexed files\n", result.SourceManifest.IndexedFiles)
		fmt.Fprintf(out, "  %d indexed tests\n", result.SourceManifest.IndexedTests)
		fmt.Fprintf(out, "  %d symbols, %d test names, %d imports\n", result.SourceManifest.SymbolRows, result.SourceManifest.TestRows, result.SourceManifest.ImportRows)
	}
	if result.Traversal != nil && result.Traversal.SkippedDirs > 0 {
		fmt.Fprintln(out, "\nTraversal:")
		fmt.Fprintf(out, "  %d candidate files considered\n", result.Traversal.InventoryFiles)
		fmt.Fprintf(out, "  %d ignored/heavy directories skipped", result.Traversal.SkippedDirs)
		if reasons := formatTraversalReasonsHuman(result.Traversal.SkippedByReason); reasons != "" {
			fmt.Fprintf(out, " (reasons: %s)", reasons)
		}
		fmt.Fprintln(out)
		if len(result.Traversal.TopSkippedDirs) > 0 {
			fmt.Fprintln(out, "  Examples:")
			for _, skipped := range result.Traversal.TopSkippedDirs {
				fmt.Fprintf(out, "    - %s (%s)\n", skipped.Path, traversalReasonLabel(skipped.Reason))
			}
		}
	}
	fmt.Fprintln(out, "\nRun:\n  ds find \"<topic>\"\n  ds recent")
	return nil
}

const scanProgressInventoryThreshold = 200

func scanProgressStderr(out io.Writer, label string) func(scan.ProgressEvent) {
	if out == nil {
		return nil
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Scan"
	}
	emitted := false
	return func(event scan.ProgressEvent) {
		switch {
		case event.Phase == "shared_discovery" && event.Event == "inventory_done" && shouldPrintScanProgress(event):
			fmt.Fprintf(out, "%s progress: discovered %d candidate file(s)", label, event.InventoryFiles)
			if event.SkippedDirs > 0 {
				fmt.Fprintf(out, "; skipped %d ignored/heavy directories", event.SkippedDirs)
				if reasons := formatTraversalReasonsHuman(event.SkippedByReason); reasons != "" {
					fmt.Fprintf(out, " (%s)", reasons)
				}
			}
			fmt.Fprintln(out)
			emitted = true
		case event.Phase == "shared_discovery" && event.Event == "" && (emitted || shouldPrintScanProgress(event)):
			fmt.Fprintf(out, "%s progress: scanned %d/%d candidate file(s)\n", label, event.FilesScanned, event.FilesTotal)
			emitted = true
		case event.Phase == "scan" && event.Event == "done" && emitted:
			fmt.Fprintf(out, "%s progress: complete\n", label)
		}
	}
}

func shouldPrintScanProgress(event scan.ProgressEvent) bool {
	if event.InventoryFiles >= scanProgressInventoryThreshold || event.FilesTotal >= scanProgressInventoryThreshold {
		return true
	}
	return event.ElapsedMS >= int64((5 * time.Second).Milliseconds())
}

func scanTraversalError(repoRoot string, err error) error {
	if err == nil {
		return nil
	}
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		root = "."
	}
	if abs, absErr := filepath.Abs(root); absErr == nil {
		root = abs
	}
	return fmt.Errorf("scan failed while walking %s: %w; try running DevSpecs from one focused project root or pass --path <repo-dir> to narrow the scan", root, err)
}

func liveScanRunOptions(db *store.DB, repoRoot string) (scan.RunOptions, error) {
	opts := scan.RunOptions{UseTransaction: true}
	hasArtifacts, err := scanTargetHasArtifacts(db, repoRoot)
	if err != nil {
		return opts, err
	}
	if !hasArtifacts {
		opts.FreshIndex = true
		opts.SkipAuthoredAtLookup = true
	}
	return opts, nil
}

func scanTargetHasArtifacts(db *store.DB, repoRoot string) (bool, error) {
	if strings.TrimSpace(repoRoot) != "" {
		count, err := db.CountArtifacts(store.FilterParams{RepoRoot: repoRoot})
		if err != nil {
			return false, err
		}
		return count > 0, nil
	}
	return scanIndexHasArtifacts(db)
}

func scanIndexHasArtifacts(db *store.DB) (bool, error) {
	var one int
	err := db.QueryRow("SELECT 1 FROM artifacts LIMIT 1").Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func scanTotalFound(r *scan.Result) int {
	n := 0
	for _, c := range r.Found {
		n += c
	}
	return n
}

func attachScanHints(result *scan.Result, repoRoot string, m *ignore.Matcher) {
	cands := discover.ScanHintCandidates(repoRoot, m)
	if len(cands) == 0 {
		result.Hints = nil
		return
	}
	result.Hints = make([]scan.ScanHint, 0, len(cands))
	for _, c := range cands {
		result.Hints = append(result.Hints, scan.ScanHint{
			Path:           c.RelPath,
			SourceType:     c.SourceType,
			SuggestCommand: discover.FormatSuggestCommand(c),
		})
	}
}

func printEmptyScanHints(cmd *cobra.Command, repoRoot string, hints []scan.ScanHint) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Scanned repository: %s\n\n", repoRoot)
	fmt.Fprintln(out, "No artifacts found in configured paths.")
	fmt.Fprintln(out)
	if len(hints) == 0 {
		fmt.Fprintln(out, "No on-disk candidate directories matched built-in heuristics.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Add a markdown source, for example:")
		fmt.Fprintln(out, "  ds config add-source markdown plans")
		return
	}
	fmt.Fprintln(out, "Possible candidates:")
	for _, h := range hints {
		fmt.Fprintf(out, "  %s\n", discover.HintDisplayPath(h.Path))
	}
	fmt.Fprintln(out, "Add one:")
	fmt.Fprintf(out, "  %s\n", hints[0].SuggestCommand)
}

// formatScanFormatsHuman renders format_profile counts sorted by profile name (stable output).
func formatScanFormatsHuman(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s %d", k, m[k])
	}
	return b.String()
}

func formatTraversalReasonsHuman(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	labeled := make(map[string]int, len(m))
	for reason, count := range m {
		labeled[traversalReasonLabel(reason)] += count
	}
	return formatScanFormatsHuman(labeled)
}

func traversalReasonLabel(reason string) string {
	switch reason {
	case "generated_vendor_or_build":
		return "default heavy dirs"
	case "ignore_rules":
		return "ignore rules"
	default:
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return "skipped"
		}
		return strings.ReplaceAll(reason, "_", " ")
	}
}

func sourcePathsChanged(repoRoot string, cfg *config.RepoConfig) bool {
	changedFiles := repo.ChangedFiles(repoRoot)
	if len(changedFiles) == 0 {
		return false
	}

	if cfg == nil {
		cfg = config.DefaultRepoConfig()
	}

	var sourcePrefixes []string
	for _, src := range cfg.Sources {
		if src.Path != "" {
			sourcePrefixes = append(sourcePrefixes, src.Path+"/")
		}
		for _, p := range src.Paths {
			sourcePrefixes = append(sourcePrefixes, p+"/")
		}
	}

	for _, f := range changedFiles {
		f = filepath.ToSlash(f)
		if cfg.TestCaseArtifactsEnabled(false) && looksLikeTestArtifactPath(f) {
			return true
		}
		if cfg.CodeCommentArtifactsEnabled(false) && looksLikeCodeCommentArtifactPath(f) {
			return true
		}
		// Root-level spec/plan files always count
		if strings.HasSuffix(f, ".spec.md") || strings.HasSuffix(f, ".plan.md") {
			if !strings.Contains(f, "/") {
				return true
			}
		}
		for _, prefix := range sourcePrefixes {
			if strings.HasPrefix(f, prefix) {
				return true
			}
		}
	}
	return false
}

func looksLikeCodeCommentArtifactPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	switch filepath.Ext(rel) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php", ".java", ".kt", ".kts", ".rs":
		return true
	default:
		return false
	}
}

func looksLikeTestArtifactPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	base := filepath.Base(rel)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	case isJSTestArtifactPath(rel, ext, name):
		return true
	}
	for _, segment := range strings.Split(rel, "/") {
		switch segment {
		case "tests", "__tests__", "spec", "cypress", "e2e":
			return true
		}
	}
	return false
}

func isJSTestArtifactPath(rel, ext, name string) bool {
	switch ext {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return strings.HasSuffix(name, ".test") ||
			strings.HasSuffix(name, ".spec") ||
			strings.Contains(rel, "/__tests__/") ||
			strings.Contains(rel, "/tests/") ||
			strings.Contains(rel, "/cypress/") ||
			strings.Contains(rel, "/e2e/")
	default:
		return false
	}
}

func resolveRepoRoot(path string) (string, error) {
	if path == "." {
		return os.Getwd()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}
