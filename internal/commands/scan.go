package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewScanCmd creates the ds scan command.
func NewScanCmd() *cobra.Command {
	var (
		path    string
		verbose bool
		asJSON  bool
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan repository for specs, plans, and ADRs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd, path, verbose, asJSON)
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to scan")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show detailed scan output")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runScan(cmd *cobra.Command, path string, verbose, asJSON bool) error {
	repoRoot, err := resolveRepoRoot(path)
	if err != nil {
		return err
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	cfg, _ := config.LoadRepoConfig(repoRoot)

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}

	scanner := scan.New(db, ids, adpts)
	result, err := scanner.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Scanned repository: %s\n", repoRoot)
	fmt.Fprintln(out, "\nFound:")
	for adapter, count := range result.Found {
		fmt.Fprintf(out, "  %d %s\n", count, adapter)
	}
	fmt.Fprintln(out, "\nIndexed:")
	fmt.Fprintf(out, "  %d new artifacts\n", result.New)
	fmt.Fprintf(out, "  %d updated artifacts\n", result.Updated)
	fmt.Fprintf(out, "  %d unchanged artifacts\n", result.Unchanged)
	fmt.Fprintln(out, "\nRun:\n  ds list")
	return nil
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
