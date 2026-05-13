package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewCriteriaCmd creates the ds criteria command.
func NewCriteriaCmd() *cobra.Command {
	var (
		openOnly     bool
		doneOnly     bool
		tag          string
		branch       string
		user         string
		repoName     string
		criteriaKind string
		asJSON       bool
		noRefresh    bool
	)

	cmd := &cobra.Command{
		Use:   "criteria [artifact-id]",
		Short: "List extracted acceptance/success/OKR checklist criteria from artifacts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var artifactID string
			if len(args) > 0 {
				artifactID = args[0]
			}
			fp := store.FilterParams{Tag: tag, Branch: branch, User: user}
			return runCriteria(cmd, artifactID, fp, repoName, criteriaKind, openOnly, doneOnly, asJSON, noRefresh)
		},
	}

	cmd.Flags().BoolVar(&openOnly, "open", false, "Show only incomplete criteria")
	cmd.Flags().BoolVar(&doneOnly, "done", false, "Show only satisfied criteria")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().StringVar(&criteriaKind, "kind", "", "Filter by criteria kind (acceptance, success, okr)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runCriteria(cmd *cobra.Command, artifactID string, fp store.FilterParams, repoName, criteriaKind string, openOnly, doneOnly, asJSON, noRefresh bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if !noRefresh {
		ensureFresh(cmd, db)
	}

	if artifactID != "" {
		art, err := db.GetArtifact(artifactID)
		if err != nil {
			return err
		}
		rows, err := db.GetCriteriaForArtifact(art.ID)
		if err != nil {
			return fmt.Errorf("get criteria: %w", err)
		}
		rows = filterCriterionRows(rows, criteriaKind, openOnly, doneOnly)
		enrichCriteriaWithArtifact(rows, art)
		return outputCriteria(cmd, rows, asJSON)
	}

	if repoName != "" {
		fp.RepoRoot = resolveRepoRootByName(db, repoName)
	}

	rows, err := db.ListAllCriteria(fp, openOnly, doneOnly, criteriaKind)
	if err != nil {
		return fmt.Errorf("list criteria: %w", err)
	}
	return outputCriteria(cmd, rows, asJSON)
}

func enrichCriteriaWithArtifact(rows []store.CriterionRow, art *store.ArtifactRow) {
	for i := range rows {
		rows[i].ArtifactTitle = art.Title
		rows[i].ArtifactKind = art.Kind
		rows[i].ArtifactShortID = art.ShortID
	}
}

func outputCriteria(cmd *cobra.Command, rows []store.CriterionRow, asJSON bool) error {
	if asJSON {
		type jsonCriterion struct {
			ArtifactID      string `json:"artifact_id"`
			ArtifactTitle   string `json:"artifact_title"`
			ArtifactKind    string `json:"artifact_kind"`
			ArtifactShortID string `json:"artifact_short_id"`
			RevisionID      string `json:"revision_id"`
			Ordinal         int    `json:"ordinal"`
			Text            string `json:"text"`
			Done            bool   `json:"done"`
			SourceFile      string `json:"source_file"`
			SourceLine      int    `json:"source_line"`
			CriteriaKind    string `json:"criteria_kind"`
		}
		out := make([]jsonCriterion, len(rows))
		for i, c := range rows {
			out[i] = jsonCriterion{
				ArtifactID:      c.ArtifactID,
				ArtifactTitle:   c.ArtifactTitle,
				ArtifactKind:    c.ArtifactKind,
				ArtifactShortID: c.ArtifactShortID,
				RevisionID:      c.RevisionID,
				Ordinal:         c.Ordinal,
				Text:            c.Text,
				Done:            c.Done,
				SourceFile:      c.SourceFile,
				SourceLine:      c.SourceLine,
				CriteriaKind:    c.CriteriaKind,
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	writeCriteriaHuman(cmd.OutOrStdout(), rows)
	return nil
}

func writeCriteriaHuman(out io.Writer, rows []store.CriterionRow) {
	wd, _ := os.Getwd()
	repoLabel := filepath.Base(resolveRepoRootFromWd(wd))
	fmt.Fprintf(out, "DevSpecs Criteria (%s)\n", repoLabel)
	open, total := countCriterionOpenTotal(rows)
	fmt.Fprintf(out, "%d open / %d total\n\n", open, total)

	for _, g := range groupCriteriaByArtifact(rows) {
		head := g[0]
		title := head.ArtifactTitle
		if title == "" {
			title = head.ArtifactID
		}
		gOpen, gTot := countCriterionOpenTotal(g)
		fmt.Fprintf(out, "  %s (%s)  [%d open / %d total]\n", title, head.ArtifactKind, gOpen, gTot)
		for _, c := range g {
			marker := "[ ]"
			if c.Done {
				marker = "[x]"
			}
			fmt.Fprintf(out, "    %s %s  %s\n", marker, c.CriteriaKind, c.Text)
		}
		fmt.Fprintln(out)
	}
}

func groupCriteriaByArtifact(rows []store.CriterionRow) [][]store.CriterionRow {
	var out [][]store.CriterionRow
	for _, c := range rows {
		if len(out) == 0 || out[len(out)-1][0].ArtifactID != c.ArtifactID {
			out = append(out, []store.CriterionRow{c})
		} else {
			out[len(out)-1] = append(out[len(out)-1], c)
		}
	}
	return out
}

func countCriterionOpenTotal(rows []store.CriterionRow) (open, total int) {
	for _, c := range rows {
		total++
		if !c.Done {
			open++
		}
	}
	return
}

func filterCriterionRows(rows []store.CriterionRow, criteriaKind string, openOnly, doneOnly bool) []store.CriterionRow {
	var out []store.CriterionRow
	for _, c := range rows {
		if criteriaKind != "" && c.CriteriaKind != criteriaKind {
			continue
		}
		if openOnly && c.Done {
			continue
		}
		if doneOnly && !c.Done {
			continue
		}
		out = append(out, c)
	}
	return out
}
