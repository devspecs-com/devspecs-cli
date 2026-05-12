package commands

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewFindCmd creates the ds find command.
func NewFindCmd() *cobra.Command {
	var (
		kind      string
		subtype   string
		tag       string
		branch    string
		user      string
		repoName  string
		asJSON    bool
		noRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search artifacts by title, path, or body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fp := store.FilterParams{Kind: kind, Subtype: subtype, Tag: tag, Branch: branch, User: user}
			return runFind(cmd, args[0], fp, repoName, asJSON, noRefresh)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&subtype, "subtype", "", "Filter by subtype")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runFind(cmd *cobra.Command, query string, fp store.FilterParams, repoName string, asJSON, noRefresh bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	if repoName != "" {
		fp.RepoRoot = resolveRepoRootByName(db, repoName)
	}

	artifacts, err := db.FindArtifacts(query, fp)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(artifacts)
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tSUBTYPE\tTITLE\n")
	for _, a := range artifacts {
		displayID := a.ShortID
		if displayID == "" {
			displayID = a.ID
			if len(displayID) > 13 {
				displayID = displayID[:13] + "..."
			}
		}
		sub := a.Subtype
		if sub == "" {
			sub = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", displayID, a.Kind, sub, a.Title)
	}
	w.Flush()
	return nil
}
