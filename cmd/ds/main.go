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
		Short: "DevSpecs - index intent and start grounded AI coding tasks",
		Long: `DevSpecs indexes planning and specification artifacts in your repository,
assigns stable IDs, and makes them easy to reference from agents, PRs,
issues, and future workflows.

Brownfield: scan, map, and find existing plans, specs, ADRs, and other intent
artifacts so humans and agents can recover the right context.

Greenfield: create bounded task workspaces with packed source/test context,
slice prompts, checkpoints, and audit output for AI-assisted coding work.

Telemetry: DevSpecs sends minimal anonymous usage counts for install, init,
scan, and query flows. It never sends repo names, file paths, git remotes,
document text, or raw queries. Disable with DEVSPECS_TELEMETRY=0.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
	}

	rootCmd.AddCommand(commands.NewInitCmd())
	rootCmd.AddCommand(commands.NewScanCmd())
	rootCmd.AddCommand(commands.NewListCmd())
	rootCmd.AddCommand(commands.NewShowCmd())
	rootCmd.AddCommand(commands.NewFindCmd())
	rootCmd.AddCommand(commands.NewMapCmd())
	rootCmd.AddCommand(commands.NewTaskCmd())
	rootCmd.AddCommand(commands.NewResolveCmd())
	rootCmd.AddCommand(commands.NewContextCmd())
	rootCmd.AddCommand(commands.NewTodosCmd())
	rootCmd.AddCommand(commands.NewCriteriaCmd())
	rootCmd.AddCommand(commands.NewCaptureCmd())
	rootCmd.AddCommand(commands.NewStatusCmd())
	rootCmd.AddCommand(commands.NewLinkCmd())
	rootCmd.AddCommand(commands.NewVersionCmd())
	rootCmd.AddCommand(commands.NewResumeCmd())
	rootCmd.AddCommand(commands.NewConfigCmd())
	rootCmd.AddCommand(commands.NewTagCmd())
	rootCmd.AddCommand(commands.NewUntagCmd())
	rootCmd.AddCommand(commands.NewEvalCmd())

	return rootCmd
}
