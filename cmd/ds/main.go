package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/commands"
	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

const (
	rootGroupHumanOrientation = "human-orientation"
	rootGroupHumanWorkSetup   = "human-work-setup"
	rootGroupAIExecution      = "ai-execution"
	rootGroupAdvanced         = "advanced-maintenance"
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
		Short: "DevSpecs - start bounded AI coding tasks from repo intent",
		Long: `DevSpecs indexes planning and specification artifacts in your repository,
assigns stable IDs, and makes them easy to reference from agents, PRs,
issues, and future workflows.

Default workflow: use ds task to create bounded task workspaces with packed
source/test/docs context, one-slice prompts, checkpoints, result receipts, and
decision gates for AI-assisted coding work. Use ds apply next or ds apply
<target> to emit the next bounded one-slice agent prompt.

Human orientation: start with ds recent to recover the local thread, active
branches, and likely follow-up commands. Use ds find for a focused question and
ds map when you need subsystem boundaries.

Human work setup: use ds task for repo-local bounded work and ds workspace for
explicit multi-repo coordination.

AI execution: agents should consume bounded prompts with ds apply and record
evidence with ds task checkpoint, ds task evaluate, or ds task audit.

Setup: run ds init once per repo to create local config and optional Codex,
Cursor, Claude, or Windsurf adapter files for ds task and ds apply.

Diagnostic layer: start with ds recent when the target is unclear. Use ds find
to pack focused evidence and ds map to verify subsystem boundaries before
creating or continuing a task.

Telemetry: DevSpecs sends minimal anonymous usage counts for install, init,
scan, and query flows. It never sends repo names, file paths, git remotes,
document text, or raw queries. Disable with DEVSPECS_TELEMETRY=0.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
	}

	rootCmd.AddGroup(
		&cobra.Group{ID: rootGroupHumanOrientation, Title: "Human orientation"},
		&cobra.Group{ID: rootGroupHumanWorkSetup, Title: "Human work setup"},
		&cobra.Group{ID: rootGroupAIExecution, Title: "AI execution"},
		&cobra.Group{ID: rootGroupAdvanced, Title: "Advanced and maintenance"},
	)

	rootCmd.AddCommand(commands.NewInitCmd())
	rootCmd.AddCommand(commands.NewScanCmd())
	rootCmd.AddCommand(commands.NewShowCmd())
	rootCmd.AddCommand(commands.NewFindCmd())
	rootCmd.AddCommand(commands.NewRecentCmd())
	rootCmd.AddCommand(commands.NewMapCmd())
	rootCmd.AddCommand(commands.NewTaskCmd())
	rootCmd.AddCommand(commands.NewApplyCmd())
	rootCmd.AddCommand(commands.NewWorkspaceCmd())
	addHiddenWorkspaceCompatibilityCommand(rootCmd, commands.NewChangeCmd(), "ds workspace change")
	addHiddenWorkspaceCompatibilityCommand(rootCmd, commands.NewSliceCmd(), "ds workspace slice")
	addHiddenWorkspaceCompatibilityCommand(rootCmd, commands.NewTraceCmd(), "ds workspace trace")
	rootCmd.AddCommand(commands.NewTLDRCmd())
	rootCmd.AddCommand(commands.NewUpdateCmd())
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
	assignRootCommandGroups(rootCmd)

	return rootCmd
}

func addHiddenWorkspaceCompatibilityCommand(rootCmd, cmd *cobra.Command, preferred string) {
	cmd.Hidden = true
	cmd.Short = fmt.Sprintf("Compatibility alias for %s", preferred)
	cmd.Long = fmt.Sprintf("Compatibility alias. Prefer `%s` for workspace coordination.", preferred)
	rootCmd.AddCommand(cmd)
}

func assignRootCommandGroups(rootCmd *cobra.Command) {
	groups := map[string]string{
		"recent":    rootGroupHumanOrientation,
		"find":      rootGroupHumanOrientation,
		"map":       rootGroupHumanOrientation,
		"context":   rootGroupHumanOrientation,
		"show":      rootGroupHumanOrientation,
		"init":      rootGroupHumanWorkSetup,
		"task":      rootGroupHumanWorkSetup,
		"workspace": rootGroupHumanWorkSetup,
		"apply":     rootGroupAIExecution,
		"tldr":      rootGroupAIExecution,
		"change":    rootGroupAdvanced,
		"slice":     rootGroupAdvanced,
		"trace":     rootGroupAdvanced,
		"scan":      rootGroupAdvanced,
		"config":    rootGroupAdvanced,
		"update":    rootGroupAdvanced,
		"version":   rootGroupAdvanced,
	}
	for _, cmd := range rootCmd.Commands() {
		name := strings.TrimSpace(cmd.Name())
		if group, ok := groups[name]; ok {
			cmd.GroupID = group
		}
	}
}
