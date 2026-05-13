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

	repoRoot := resolveIndexedRepoRoot(db, wd)
	if repoRoot == "" {
		repoRoot = canonicalRepoRoot(resolveRepoRootFromWd(wd))
	}
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
