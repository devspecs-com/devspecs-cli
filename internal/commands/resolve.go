package commands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// NewResolveCmd creates the ds resolve command.
func NewResolveCmd() *cobra.Command {
	var (
		asJSON    bool
		noRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve an artifact ID to its source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResolve(cmd, args[0], asJSON, noRefresh)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runResolve(cmd *cobra.Command, idOrPrefix string, asJSON, noRefresh bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	art, err := db.GetArtifact(idOrPrefix)
	if err != nil {
		return err
	}

	sources, _ := db.GetSourcesForArtifact(art.ID)
	var sourcePath string
	if len(sources) > 0 {
		sourcePath = sources[0].Path
	}

	if asJSON {
		obj := map[string]string{
			"id":                    art.ID,
			"kind":                  art.Kind,
			"title":                 art.Title,
			"source_path":           sourcePath,
			"current_revision_hash": "",
		}
		if art.CurrentRevID != "" {
			rev, _ := db.GetRevision(art.CurrentRevID)
			if rev != nil {
				obj["current_revision_hash"] = rev.ContentHash
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, art.ID)
	fmt.Fprintln(out, sourcePath)
	return nil
}
