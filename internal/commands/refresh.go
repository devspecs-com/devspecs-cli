package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/freshness"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// ensureFresh checks if the index is stale and auto-scans if needed.
// Prints a one-line notice to stderr when updates occur.
func ensureFresh(cmd *cobra.Command, db *store.DB) {
	wd, err := os.Getwd()
	if err != nil {
		return
	}

	status := freshness.Check(db, wd)
	if status == nil || !status.Stale {
		return
	}

	result := runScanQuiet(db, wd)
	if result != nil && (result.New > 0 || result.Updated > 0) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Index updated (%d new, %d updated)\n", result.New, result.Updated)
	}
}

func runScanQuiet(db *store.DB, repoRoot string) *scan.Result {
	cfg, _ := config.LoadRepoConfig(repoRoot)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}}

	scanner := scan.New(db, ids, adpts)
	result, err := scanner.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		return nil
	}
	return result
}
