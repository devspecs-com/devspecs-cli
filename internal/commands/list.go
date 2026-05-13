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
		subtype    string
		status     string
		sourceType string
		tag        string
		branch     string
		user       string
		repoName   string
		allRepos   bool
		asJSON     bool
		noRefresh  bool
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List indexed artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			fp := store.FilterParams{
				Kind: kind, Subtype: subtype, Status: status, SourceType: sourceType,
				Tag: tag, Branch: branch, User: user,
			}
			return runList(cmd, fp, repoName, allRepos, asJSON, noRefresh)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&subtype, "subtype", "", "Filter by subtype")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&sourceType, "source", "", "Filter by source type")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name (basename of root_path)")
	cmd.Flags().BoolVar(&allRepos, "all", false, "List artifacts from all indexed repos (ignore cwd scope)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runList(cmd *cobra.Command, fp store.FilterParams, repoName string, allRepos, asJSON, noRefresh bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	fp.RepoRoot = resolveRepoScope(db, repoName, allRepos)

	artifacts, err := db.ListArtifacts(fp)
	if err != nil {
		return fmt.Errorf("list artifacts: %w", err)
	}

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(artifacts)
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tSUBTYPE\tSTATUS\tTITLE\n")
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", displayID, a.Kind, sub, a.Status, a.Title)
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
