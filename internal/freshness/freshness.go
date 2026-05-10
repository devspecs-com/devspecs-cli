// Package freshness detects whether the local index is stale relative to
// the repository's current state (git HEAD or source directory mtime).
package freshness

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func debugLog(format string, args ...any) {
	if os.Getenv("DS_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[ds:freshness] "+format+"\n", args...)
	}
}

// Status describes whether the index is stale for a given repo.
type Status struct {
	Stale    bool
	Reason   string
	RepoID   string
	RepoRoot string
}

// Check determines if the index for repoRoot is stale.
// Returns nil if the repo has never been scanned (uninitialized, not stale).
func Check(db *store.DB, repoRoot string) *Status {
	meta := db.GetRepoByRoot(repoRoot)
	if meta == nil {
		return nil
	}

	info := repo.Detect(repoRoot)
	if info.IsGit {
		return checkGit(meta, repoRoot)
	}
	return checkMtime(meta, repoRoot)
}

func checkGit(meta *store.RepoMeta, repoRoot string) *Status {
	head := repo.HeadCommit(repoRoot)
	if head == "" {
		debugLog("git HEAD unresolvable (not a git repo or empty?) — treating as fresh")
		return &Status{Stale: false, RepoID: meta.ID, RepoRoot: meta.RootPath}
	}
	debugLog("HEAD=%s stored=%s", head, meta.LastScanCommit)
	if meta.LastScanCommit == head {
		return &Status{Stale: false, RepoID: meta.ID, RepoRoot: meta.RootPath}
	}
	return &Status{
		Stale:    true,
		Reason:   "git HEAD changed",
		RepoID:   meta.ID,
		RepoRoot: meta.RootPath,
	}
}

func checkMtime(meta *store.RepoMeta, repoRoot string) *Status {
	if meta.LastScanAt == "" {
		return &Status{Stale: true, Reason: "never scanned", RepoID: meta.ID, RepoRoot: meta.RootPath}
	}

	lastScan, err := time.Parse(time.RFC3339, meta.LastScanAt)
	if err != nil {
		return &Status{Stale: true, Reason: "invalid last_scan_at", RepoID: meta.ID, RepoRoot: meta.RootPath}
	}

	cfg, _ := config.LoadRepoConfig(repoRoot)
	dirs := sourceDirs(cfg)

	for _, dir := range dirs {
		absDir := filepath.Join(repoRoot, dir)
		if dirModifiedAfter(absDir, lastScan) {
			return &Status{Stale: true, Reason: "source dirs modified", RepoID: meta.ID, RepoRoot: meta.RootPath}
		}
	}

	return &Status{Stale: false, RepoID: meta.ID, RepoRoot: meta.RootPath}
}

func dirModifiedAfter(dir string, threshold time.Time) bool {
	modified := false
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.ModTime().After(threshold) {
			modified = true
			return filepath.SkipAll
		}
		return nil
	})
	return modified
}

func sourceDirs(cfg *config.RepoConfig) []string {
	if cfg == nil {
		cfg = config.DefaultRepoConfig()
	}
	var dirs []string
	for _, src := range cfg.Sources {
		if src.Path != "" {
			dirs = append(dirs, src.Path)
		}
		dirs = append(dirs, src.Paths...)
	}
	return dirs
}
