package main

import (
	"fmt"
	"os"

	"github.com/devspecs-com/devspecs-cli/internal/commands"
	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "ds",
		Short: "DevSpecs — index and reference your specs, plans, and ADRs",
		Long: `DevSpecs indexes planning and specification artifacts in your repository,
assigns stable IDs, and makes them easy to reference from agents, PRs,
issues, and future workflows.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
	}

	rootCmd.AddCommand(commands.NewInitCmd())
	rootCmd.AddCommand(commands.NewScanCmd())
	rootCmd.AddCommand(commands.NewListCmd())
	rootCmd.AddCommand(commands.NewShowCmd())
	rootCmd.AddCommand(commands.NewFindCmd())
	rootCmd.AddCommand(commands.NewResolveCmd())
	rootCmd.AddCommand(commands.NewContextCmd())
	rootCmd.AddCommand(commands.NewTodosCmd())

	return rootCmd
}
