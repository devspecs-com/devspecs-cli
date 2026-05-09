// Package commands implements all ds CLI subcommands.
package commands

import (
	"fmt"
	"os"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the ds init command.
func NewInitCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize DevSpecs in the current repository",
		Long:  "Creates the global DevSpecs directory and database, and optionally a repo-local .devspecs/config.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config if present")
	return cmd
}

func runInit(cmd *cobra.Command, force bool) error {
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
		return nil
	}

	cfg := config.DefaultRepoConfig()
	if err := config.WriteRepoConfig(wd, cfg); err != nil {
		return fmt.Errorf("write repo config: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Initialized DevSpecs.")
	fmt.Fprintf(cmd.OutOrStdout(), "\nGlobal index:\n  %s\n", homeDir)
	fmt.Fprintf(cmd.OutOrStdout(), "\nRepo config:\n  %s\n", configPath)
	fmt.Fprintln(cmd.OutOrStdout(), "\nNext:\n  ds scan")
	return nil
}
