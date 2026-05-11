package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewScanCmd creates the ds scan command.
func NewScanCmd() *cobra.Command {
	var (
		path      string
		verbose   bool
		asJSON    bool
		quiet     bool
		ifChanged bool
		rebuild   bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan repository for specs, plans, and ADRs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd, path, verbose, asJSON, quiet, ifChanged, rebuild)
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to scan")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed scan output")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress human scan summary and empty-scan hints (redundant when --json is set)")
	cmd.Flags().BoolVar(&ifChanged, "if-changed", false, "Only scan if source paths were touched in the last commit")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Remove the global index database and create a fresh index (requires re-scan)")
	return cmd
}

func runScan(cmd *cobra.Command, path string, verbose, asJSON, quiet, ifChanged, rebuild bool) error {
	repoRoot, err := resolveRepoRoot(path)
	if err != nil {
		return err
	}

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if ifChanged && !sourcePathsChanged(repoRoot, cfg) {
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

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}}

	scanner := scan.New(db, ids, adpts)
	if verbose && !quiet {
		fmt.Fprintf(cmd.ErrOrStderr(), "Respecting repo-root .gitignore, .git/info/exclude, and .aiignore during configured walks\n")
	}
	result, err := scanner.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	if scanTotalFound(result) == 0 {
		matcher, _ := ignore.NewMatcher(repoRoot)
		attachScanHints(result, repoRoot, matcher)
	}

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
	fmt.Fprintln(out, "\nRun:\n  ds list")
	return nil
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
