// Package commands implements all ds CLI subcommands.
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the ds init command.
func NewInitCmd() *cobra.Command {
	var (
		force bool
		hooks bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DevSpecs in the current repository",
		Long:  "Creates the global DevSpecs directory and database, and optionally a repo-local .devspecs/config.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, force, hooks)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config if present")
	cmd.Flags().BoolVar(&hooks, "hooks", false, "Install git post-commit hook for auto-indexing")
	return cmd
}

func runInit(cmd *cobra.Command, force, hooks bool) error {
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

	configPath := config.RepoConfigPath(wd)
	_, statErr := os.Stat(configPath)
	configExists := statErr == nil

	if configExists && !force {
		fmt.Fprintln(cmd.OutOrStdout(), "DevSpecs already initialized.")
		fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)
		fmt.Fprintln(cmd.OutOrStdout(), "\nUse --force to overwrite existing config.")

		if hooks {
			installHook(cmd, wd)
		}
		return nil
	}

	cfg := config.DefaultRepoConfig()
	if err := config.WriteRepoConfig(wd, cfg); err != nil {
		return fmt.Errorf("write repo config: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Initialized DevSpecs.")
	fmt.Fprintf(cmd.OutOrStdout(), "\nGlobal index:\n  %s\n", homeDir)
	fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)

	if hooks {
		installHook(cmd, wd)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nNext:\n  ds scan")
	return nil
}

const hookMarker = "# DevSpecs auto-index"
const hookScript = `#!/bin/sh
# DevSpecs auto-index
ds scan --quiet --if-changed 2>/dev/null || true
`

func installHook(cmd *cobra.Command, wd string) {
	info := repo.Detect(wd)
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
		// Append to existing hook
		content := string(existing) + "\n" + hookMarker + "\nds scan --quiet --if-changed 2>/dev/null || true\n"
		os.WriteFile(hookPath, []byte(content), 0o755)
	} else {
		os.WriteFile(hookPath, []byte(hookScript), 0o755)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nInstalled git post-commit hook for auto-indexing.")
}
