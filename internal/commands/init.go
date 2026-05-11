// Package commands implements all ds CLI subcommands.
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the ds init command.
func NewInitCmd() *cobra.Command {
	var (
		force          bool
		hooks          bool
		noDetect       bool
		yes            bool
		nonInteractive bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DevSpecs in the current repository",
		Long: `Creates the global DevSpecs directory and database, and optionally a repo-local .devspecs/config.yaml.

Layout detection runs by default (unless --no-detect). Init never opens an interactive prompt in v0.1; --yes and --non-interactive are accepted for CI and script parity with other tools but do not change behavior today.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = yes, nonInteractive
			return runInit(cmd, force, hooks, noDetect)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config if present")
	cmd.Flags().BoolVar(&hooks, "hooks", false, "Install git post-commit hook for auto-indexing")
	cmd.Flags().BoolVar(&noDetect, "no-detect", false, "Skip repository layout detection (defaults-only config)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Non-interactive init (default behavior; same as --non-interactive)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Non-interactive init (alias for --yes)")
	return cmd
}

func runInit(cmd *cobra.Command, force, hooks, noDetect bool) error {
	homeDir, err := config.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	db.Close()

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}

	repoRoot := wd
	if info := repo.Detect(wd); info.IsGit {
		repoRoot = info.RootPath
	}
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}

	configPath := config.RepoConfigPath(repoRoot)
	_, statErr := os.Stat(configPath)
	configExists := statErr == nil

	if configExists && !force {
		fmt.Fprintln(cmd.OutOrStdout(), "DevSpecs already initialized.")
		fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)
		fmt.Fprintln(cmd.OutOrStdout(), "\nUse --force to overwrite existing config.")

		if hooks {
			installHook(cmd, repoRoot)
		}
		return nil
	}

	cfg := config.DefaultRepoConfig()

	var dres *discover.Result
	if !noDetect {
		matcher, _ := ignore.NewMatcher(repoRoot)
		dres = discover.Run(repoRoot, matcher)
		mergeDiscovery(cfg, dres)
	}

	if err := config.WriteRepoConfig(repoRoot, cfg); err != nil {
		return fmt.Errorf("write repo config: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Initialized DevSpecs.")
	fmt.Fprintf(cmd.OutOrStdout(), "\nGlobal index:\n  %s\n", homeDir)
	fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)

	if dres != nil {
		if dres.SkippedIgnored > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nDiscovery skipped %d ignored paths (.gitignore / .git/info/exclude / .aiignore).\n", dres.SkippedIgnored)
		}
		for _, s := range dres.Suggestions {
			fmt.Fprintf(cmd.OutOrStdout(), "Suggestion: %s\n", s)
		}
	}

	if hooks {
		installHook(cmd, repoRoot)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nNext:\n  ds scan")
	return nil
}

func mergeDiscovery(cfg *config.RepoConfig, d *discover.Result) {
	if d == nil {
		return
	}
	for i := range cfg.Sources {
		switch cfg.Sources[i].Type {
		case "markdown":
			cfg.Sources[i].Paths = mergeSortedUniquePaths(cfg.Sources[i].Path, cfg.Sources[i].Paths, d.MergeMarkdown)
			cfg.Sources[i].Path = ""
		case "adr":
			cfg.Sources[i].Paths = mergeSortedUniquePaths(cfg.Sources[i].Path, cfg.Sources[i].Paths, d.MergeADR)
			cfg.Sources[i].Path = ""
		}
	}
}

func mergeSortedUniquePaths(single string, base, extra []string) []string {
	seen := make(map[string]bool)
	var acc []string
	if single != "" {
		if !seen[single] {
			seen[single] = true
			acc = append(acc, single)
		}
	}
	for _, p := range base {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		acc = append(acc, p)
	}
	for _, p := range extra {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		acc = append(acc, p)
	}
	sort.Strings(acc)
	return acc
}

const hookMarker = "# DevSpecs auto-index"

func hookScriptContent() string {
	bin := resolveDsBinary()
	return fmt.Sprintf("#!/bin/sh\n%s\n%s scan --quiet --if-changed 2>/dev/null || true\n", hookMarker, bin)
}

func hookAppendContent() string {
	bin := resolveDsBinary()
	return fmt.Sprintf("\n%s\n%s scan --quiet --if-changed 2>/dev/null || true\n", hookMarker, bin)
}

// resolveDsBinary returns the absolute path to the ds binary if found in PATH,
// falling back to "ds" if not resolvable (e.g. during tests).
func resolveDsBinary() string {
	path, err := exec.LookPath("ds")
	if err == nil {
		return path
	}
	return "ds"
}

func installHook(cmd *cobra.Command, repoRoot string) {
	info := repo.Detect(repoRoot)
	if !info.IsGit {
		fmt.Fprintln(cmd.ErrOrStderr(), "Not a git repository — skipping hook installation.")
		return
	}

	hooksDir := filepath.Join(info.RootPath, ".git", "hooks")
	hookPath := filepath.Join(hooksDir, "post-commit")

	os.MkdirAll(hooksDir, 0o755)

	existing, err := os.ReadFile(hookPath)
	if err == nil && strings.Contains(string(existing), hookMarker) {
		fmt.Fprintln(cmd.OutOrStdout(), "\nGit hook already installed.")
		return
	}

	if err == nil && len(existing) > 0 {
		os.WriteFile(hookPath, []byte(string(existing)+hookAppendContent()), 0o755)
	} else {
		os.WriteFile(hookPath, []byte(hookScriptContent()), 0o755)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nInstalled git post-commit hook for auto-indexing.")
}
