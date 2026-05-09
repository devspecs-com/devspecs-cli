package commands

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewListCmd creates the ds list command.
func NewListCmd() *cobra.Command {
	var (
		kind       string
		status     string
		sourceType string
		asJSON     bool
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List indexed artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, kind, status, sourceType, asJSON)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&sourceType, "source", "", "Filter by source type")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runList(cmd *cobra.Command, kind, status, sourceType string, asJSON bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	artifacts, err := db.ListArtifacts("", kind, status, sourceType)
	if err != nil {
		return fmt.Errorf("list artifacts: %w", err)
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(artifacts)
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tSTATUS\tTITLE\n")
	for _, a := range artifacts {
		shortID := a.ID
		if len(shortID) > 13 {
			shortID = shortID[:13] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", shortID, a.Kind, a.Status, a.Title)
	}
	w.Flush()
	return nil
}

func openDB() (*store.DB, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, fmt.Errorf("resolve db path: %w", err)
	}
	return store.Open(dbPath)
}
