// Package commands implements all ds CLI subcommands.
package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/initflow"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/mattn/go-isatty"
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
		noTools        bool
		agentTools     []string
		indexMode      string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DevSpecs in the current repository",
		Long: `Creates the global DevSpecs directory and database, and optionally a repo-local .devspecs/config.yaml.

Layout detection runs by default (unless --no-detect). In an interactive terminal, a workflow profile picker runs first to merge common source paths and rules; use --yes or --non-interactive to skip it (CI and scripts).

Agent tooling detection can preselect Codex, Cursor, Claude, or Windsurf setup targets. File generation is handled by the follow-up tooling setup flow; init only detects and reports the selection today.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			opts := initOptions{
				Force:          force,
				Hooks:          hooks,
				NoDetect:       noDetect,
				Yes:            yes,
				NonInteractive: nonInteractive,
				NoTools:        noTools,
				AgentTools:     agentTools,
				IndexMode:      indexMode,
			}
			err := runInit(cmd, opts)
			telemetry.RecordCommand("init", err == nil, time.Since(start), map[string]any{
				"force":     force,
				"hooks":     hooks,
				"no_detect": noDetect,
				"index":     indexMode,
				"tools":     len(agentTools),
				"no_tools":  noTools,
			})
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config if present")
	cmd.Flags().BoolVar(&hooks, "hooks", false, "Install git post-commit hook for auto-indexing")
	cmd.Flags().BoolVar(&noDetect, "no-detect", false, "Skip repository layout detection (defaults-only config)")
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip interactive workflow profile picker (same as --non-interactive)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Skip interactive workflow profile picker (alias for --yes)")
	cmd.Flags().StringArrayVar(&agentTools, "tool", nil, "Agent tooling target to prepare: auto, all, none, codex, cursor, claude, or windsurf (may be repeated or comma-separated)")
	cmd.Flags().BoolVar(&noTools, "no-tools", false, "Skip agent tooling detection and selection")
	cmd.Flags().StringVar(&indexMode, "index", "auto", "Index after init: auto, background, foreground, or manual")
	return cmd
}

type initOptions struct {
	Force          bool
	Hooks          bool
	NoDetect       bool
	Yes            bool
	NonInteractive bool
	NoTools        bool
	AgentTools     []string
	IndexMode      string
}

type initIndexResult struct {
	Mode    string
	Started bool
	PID     int
	Message string
	Err     error
}

var startInitBackgroundScan = startBackgroundScan

func runInit(cmd *cobra.Command, opts initOptions) error {
	indexMode := strings.ToLower(strings.TrimSpace(opts.IndexMode))
	if indexMode == "" {
		indexMode = "auto"
	}
	switch indexMode {
	case "auto", "background", "foreground", "manual", "none":
	default:
		return fmt.Errorf("invalid --index %q (use auto, background, foreground, or manual)", opts.IndexMode)
	}
	if opts.NoTools && len(opts.AgentTools) > 0 {
		return fmt.Errorf("--no-tools cannot be combined with --tool")
	}

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

	stdinTTY := isatty.IsTerminal(os.Stdin.Fd())
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd())
	interactive := stdinTTY && stdoutTTY && !opts.Yes && !opts.NonInteractive && !opts.NoDetect

	if configExists && !opts.Force {
		fmt.Fprintln(cmd.OutOrStdout(), "DevSpecs already initialized.")
		fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)
		fmt.Fprintln(cmd.OutOrStdout(), "\nUse --force to overwrite existing config.")

		if opts.Hooks {
			installHook(cmd, repoRoot)
		}
		printInitNext(cmd)
		return nil
	}

	cfg := config.DefaultRepoConfig()

	if interactive {
		selected, customPaths, customRules, err := initflow.RunProfilePick(repoRoot)
		if err != nil {
			return err
		}
		merged, err := initflow.MergeSelectedProfiles(cfg, selected, customPaths, customRules)
		if err != nil {
			return fmt.Errorf("workflow profiles: %w", err)
		}
		cfg = merged
	}

	var dres *discover.Result
	if !opts.NoDetect {
		matcher, _ := ignore.NewMatcher(repoRoot)
		dres = discover.Run(repoRoot, matcher)
		mergeDiscovery(cfg, dres)
	}

	agentSelection, err := resolveInitAgentTools(repoRoot, opts, interactive)
	if err != nil {
		return err
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

	if opts.Hooks {
		installHook(cmd, repoRoot)
	}

	printInitAgentTools(cmd, agentSelection, opts.NoTools)
	printInitIndexResult(cmd, runInitIndex(cmd, repoRoot, indexMode, interactive))
	printInitNext(cmd)
	return nil
}

func resolveInitAgentTools(repoRoot string, opts initOptions, interactive bool) ([]initflow.AgentTool, error) {
	if interactive && !opts.NoTools && len(opts.AgentTools) == 0 {
		return initflow.RunAgentToolPick(repoRoot)
	}
	return initflow.SelectAgentTools(repoRoot, opts.AgentTools, opts.NoTools)
}

func printInitAgentTools(cmd *cobra.Command, tools []initflow.AgentTool, skipped bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nAgent tooling:")
	if skipped {
		fmt.Fprintln(out, "  Skipped (--no-tools).")
		return
	}
	if len(tools) == 0 {
		fmt.Fprintln(out, "  No Codex/Cursor/Claude/Windsurf project surfaces detected.")
		fmt.Fprintln(out, "  To choose explicitly later: ds init --force --tool codex --tool cursor")
		return
	}
	for _, tool := range tools {
		detected := "not detected"
		if tool.Detected {
			detected = "detected: " + strings.Join(tool.Evidence, ", ")
		}
		fmt.Fprintf(out, "  - %s (%s)\n", tool.Label, detected)
		for _, planned := range tool.Planned {
			fmt.Fprintf(out, "    prepares: %s\n", planned)
		}
	}
}

func runInitIndex(cmd *cobra.Command, repoRoot, mode string, interactive bool) initIndexResult {
	if mode == "none" {
		mode = "manual"
	}
	if mode == "auto" {
		if interactive {
			mode = "background"
		} else {
			mode = "manual"
		}
	}

	switch mode {
	case "manual":
		return initIndexResult{Mode: mode, Message: "Not started automatically. Run `ds scan` when you want to refresh the local index."}
	case "foreground":
		silent := &cobra.Command{}
		silent.SetOut(io.Discard)
		silent.SetErr(io.Discard)
		if err := runScan(
			silent,
			repoRoot,
			false, // verbose
			false, // json
			true,  // quiet
			false, // if changed
			false, // rebuild
			false, // experimental intent discovery
			false, // experimental git evidence
			false, // experimental workstream evidence
			false, // experimental rich typed index
			false, // experimental support docs
			false, // experimental recent source
			false, // experimental first-party source
			false, // experimental source manifest
			false, // include tests
			false, // include code comments
			false, // no gitignore
		); err != nil {
			return initIndexResult{Mode: mode, Err: err}
		}
		return initIndexResult{Mode: mode, Message: "Indexed current repo."}
	case "background":
		pid, err := startInitBackgroundScan(repoRoot)
		if err != nil {
			return initIndexResult{Mode: mode, Err: err}
		}
		return initIndexResult{Mode: mode, Started: true, PID: pid, Message: "Started background index refresh."}
	default:
		return initIndexResult{Mode: mode, Err: fmt.Errorf("invalid init index mode %q", mode)}
	}
}

func printInitIndexResult(cmd *cobra.Command, result initIndexResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nIndexing:")
	if result.Err != nil {
		fmt.Fprintf(out, "  Could not start %s indexing: %v\n", result.Mode, result.Err)
		fmt.Fprintln(out, "  Run manually: ds scan")
		return
	}
	if result.Started {
		fmt.Fprintf(out, "  %s", result.Message)
		if result.PID > 0 {
			fmt.Fprintf(out, " (pid %d)", result.PID)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  If it does not finish cleanly, run: ds scan")
		return
	}
	fmt.Fprintf(out, "  %s\n", result.Message)
}

func printInitNext(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nNext:")
	fmt.Fprintln(cmd.OutOrStdout(), "  ds task \"goal\"")
}

func startBackgroundScan(repoRoot string) (int, error) {
	bin := resolveDsBinary()
	if bin == "ds" {
		if exe, err := os.Executable(); err == nil && strings.TrimSpace(exe) != "" {
			bin = exe
		}
	}
	scanCmd := exec.Command(bin, "scan", "--quiet", "--path", repoRoot)
	scanCmd.Dir = repoRoot
	scanCmd.Stdout = nil
	scanCmd.Stderr = nil
	if err := scanCmd.Start(); err != nil {
		return 0, err
	}
	return scanCmd.Process.Pid, scanCmd.Process.Release()
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
