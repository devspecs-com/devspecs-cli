package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
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
		allRepos  bool
		asJSON    bool
		noRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search artifacts by title, path, or body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fp := store.FilterParams{Kind: kind, Subtype: subtype, Tag: tag, Branch: branch, User: user}
			return runFind(cmd, args[0], fp, repoName, allRepos, asJSON, noRefresh)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&subtype, "subtype", "", "Filter by subtype")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().BoolVar(&allRepos, "all", false, "Search artifacts in all indexed repos (ignore cwd scope)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runFind(cmd *cobra.Command, query string, fp store.FilterParams, repoName string, allRepos, asJSON, noRefresh bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	fp.RepoRoot = resolveRepoScope(db, repoName, allRepos)

	candidates, err := loadRetrievalCandidates(db, fp)
	if err != nil {
		return fmt.Errorf("find: %w", err)
	}
	retriever := retrieval.WeightedFilesRetrieverV0{}
	matches := retriever.Retrieve(candidates, query)
	if len(matches) == 0 {
		matches = retrieval.QueryBaseline(candidates, query)
	}
	reasons := reasonsByPath(retrieval.ExplainCandidates(matches, query))

	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(findResults(matches, reasons, retriever.Name()))
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tKIND\tSUBTYPE\tTITLE\tSOURCE\n")
	for _, c := range matches {
		displayID := shortCandidateID(c)
		if displayID == "" {
			displayID = c.ID
		}
		if len(displayID) > 13 {
			displayID = displayID[:13] + "..."
		}
		sub := c.Subtype
		if sub == "" {
			sub = "-"
		}
		source := c.Source
		if source == "" {
			source = c.Path
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", displayID, c.Kind, sub, c.Title, source)
		if rs := reasons[c.Path]; len(rs) > 0 {
			fmt.Fprintf(w, "\t\t\tReasons: %s\t\n", strings.Join(rs, "; "))
		}
	}
	w.Flush()
	return nil
}

type FindResult struct {
	ID             string   `json:"ID"`
	RepoID         string   `json:"RepoID"`
	ShortID        string   `json:"ShortID"`
	Kind           string   `json:"Kind"`
	Subtype        string   `json:"Subtype"`
	Title          string   `json:"Title"`
	Status         string   `json:"Status"`
	CurrentRevID   string   `json:"CurrentRevID"`
	CreatedAt      string   `json:"CreatedAt"`
	UpdatedAt      string   `json:"UpdatedAt"`
	LastObservedAt string   `json:"LastObservedAt"`
	SourcePath     string   `json:"source_path,omitempty"`
	Retriever      string   `json:"retriever"`
	Reasons        []string `json:"reasons,omitempty"`
}

func findResults(candidates []retrieval.Candidate, reasons map[string][]string, retrieverName string) []FindResult {
	results := make([]FindResult, 0, len(candidates))
	for _, c := range candidates {
		results = append(results, FindResult{
			ID:             c.ID,
			RepoID:         metadataValue(c, "repo_id"),
			ShortID:        metadataValue(c, "short_id"),
			Kind:           c.Kind,
			Subtype:        c.Subtype,
			Title:          c.Title,
			Status:         c.Status,
			CurrentRevID:   metadataValue(c, "current_revision_id"),
			CreatedAt:      metadataValue(c, "created_at"),
			UpdatedAt:      metadataValue(c, "updated_at"),
			LastObservedAt: metadataValue(c, "last_observed_at"),
			SourcePath:     c.Source,
			Retriever:      retrieverName,
			Reasons:        reasons[c.Path],
		})
	}
	return results
}

func reasonsByPath(reasons []retrieval.Reason) map[string][]string {
	out := make(map[string][]string, len(reasons))
	for _, reason := range reasons {
		out[reason.Path] = reason.Reasons
	}
	return out
}

func metadataValue(c retrieval.Candidate, key string) string {
	if c.Metadata == nil {
		return ""
	}
	return c.Metadata[key]
}
