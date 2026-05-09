package commands

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// NewFindCmd creates the ds find command.
func NewFindCmd() *cobra.Command {
	var (
		kind   string
		asJSON bool
	)

	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search artifacts by title, path, or body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFind(cmd, args[0], kind, asJSON)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runFind(cmd *cobra.Command, query, kind string, asJSON bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	artifacts, err := db.FindArtifacts(query, kind)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(artifacts)
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tTITLE\n")
	for _, a := range artifacts {
		shortID := a.ID
		if len(shortID) > 13 {
			shortID = shortID[:13] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", shortID, a.Kind, a.Title)
	}
	w.Flush()
	return nil
}
