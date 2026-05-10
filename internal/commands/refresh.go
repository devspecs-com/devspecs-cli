package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/freshness"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// ensureFresh checks if the index is stale and auto-scans if needed.
// Resolves the repo root by walking up from cwd (via .git or .devspecs/).
// Prints a one-line notice to stderr when updates occur.
func ensureFresh(cmd *cobra.Command, db *store.DB) {
	wd, err := os.Getwd()
	if err != nil {
		debugLog("ensureFresh: Getwd failed: %v", err)
		return
	}

	repoRoot := resolveRepoRootFromWd(wd)
	debugLog("ensureFresh: wd=%s resolved_root=%s", wd, repoRoot)

	status := freshness.Check(db, repoRoot)
	if status == nil {
		debugLog("ensureFresh: no repo row found for %s — skipping", repoRoot)
		return
	}
	if !status.Stale {
		debugLog("ensureFresh: index is fresh for %s", repoRoot)
		return
	}

	debugLog("ensureFresh: stale — reason=%s, triggering auto-scan", status.Reason)
	result := runScanQuiet(db, repoRoot)
	if result != nil && (result.New > 0 || result.Updated > 0) {
		fmt.Fprintf(cmd.ErrOrStderr(), "Index updated (%d new, %d updated)\n", result.New, result.Updated)
	}
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

func runScanQuiet(db *store.DB, repoRoot string) *scan.Result {
	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		debugLog("runScanQuiet: LoadRepoConfig error: %v", err)
	}
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}}

	scanner := scan.New(db, ids, adpts)
	result, err := scanner.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		debugLog("runScanQuiet: scan error: %v", err)
		return nil
	}
	return result
}

// debugLog prints to stderr only when DS_DEBUG=1 is set.
func debugLog(format string, args ...any) {
	if os.Getenv("DS_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[ds:debug] "+format+"\n", args...)
	}
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
