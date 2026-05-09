package commands

import (
	"encoding/json"
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

// NewVersionCmd creates the ds version subcommand with --json support.
func NewVersionCmd() *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				obj := map[string]string{
					"version": version.Version,
					"commit":  version.Commit,
					"date":    version.Date,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(obj)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ds %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}
