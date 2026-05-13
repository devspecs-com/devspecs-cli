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

// NewTodosCmd creates the ds todos command.
func NewTodosCmd() *cobra.Command {
	var (
		openOnly  bool
		doneOnly  bool
		tag       string
		branch    string
		user      string
		repoName  string
		asJSON    bool
		noRefresh bool
	)

	cmd := &cobra.Command{
		Use:   "todos [artifact-id]",
		Short: "List extracted todos from artifacts",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var artifactID string
			if len(args) > 0 {
				artifactID = args[0]
			}
			fp := store.FilterParams{Tag: tag, Branch: branch, User: user}
			return runTodos(cmd, artifactID, fp, repoName, openOnly, doneOnly, asJSON, noRefresh)
		},
	}

	cmd.Flags().BoolVar(&openOnly, "open", false, "Show only incomplete todos")
	cmd.Flags().BoolVar(&doneOnly, "done", false, "Show only completed todos")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by tag")
	cmd.Flags().StringVar(&branch, "branch", "", "Filter by git branch")
	cmd.Flags().StringVar(&user, "user", "", "Filter by scanned-by user")
	cmd.Flags().StringVar(&repoName, "repo", "", "Filter by repo name")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runTodos(cmd *cobra.Command, artifactID string, fp store.FilterParams, repoName string, openOnly, doneOnly, asJSON, noRefresh bool) error {
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
		todos, err := db.GetTodosForArtifact(art.ID)
		if err != nil {
			return fmt.Errorf("get todos: %w", err)
		}
		todos = filterTodoRows(todos, openOnly, doneOnly)
		enrichTodosWithArtifact(todos, art)
		return outputTodos(cmd, todos, asJSON)
	}

	if repoName != "" {
		fp.RepoRoot = resolveRepoRootByName(db, repoName)
	}

	todos, err := db.ListAllTodos(fp, openOnly, doneOnly)
	if err != nil {
		return fmt.Errorf("list todos: %w", err)
	}
	return outputTodos(cmd, todos, asJSON)
}

func enrichTodosWithArtifact(todos []store.TodoRow, art *store.ArtifactRow) {
	for i := range todos {
		todos[i].ArtifactTitle = art.Title
		todos[i].ArtifactKind = art.Kind
		todos[i].ArtifactShortID = art.ShortID
	}
}

func outputTodos(cmd *cobra.Command, todos []store.TodoRow, asJSON bool) error {
	if asJSON {
		type jsonTodo struct {
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
		}
		out := make([]jsonTodo, len(todos))
		for i, t := range todos {
			out[i] = jsonTodo{
				ArtifactID:      t.ArtifactID,
				ArtifactTitle:   t.ArtifactTitle,
				ArtifactKind:    t.ArtifactKind,
				ArtifactShortID: t.ArtifactShortID,
				RevisionID:      t.RevisionID,
				Ordinal:         t.Ordinal,
				Text:            t.Text,
				Done:            t.Done,
				SourceFile:      t.SourceFile,
				SourceLine:      t.SourceLine,
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	writeTodosHuman(cmd.OutOrStdout(), todos)
	return nil
}

func writeTodosHuman(out io.Writer, todos []store.TodoRow) {
	wd, _ := os.Getwd()
	repoLabel := filepath.Base(resolveRepoRootFromWd(wd))
	fmt.Fprintf(out, "DevSpecs Todos (%s)\n", repoLabel)
	open, total := countTodoOpenTotal(todos)
	fmt.Fprintf(out, "%d open / %d total\n\n", open, total)

	for _, g := range groupTodosByArtifact(todos) {
		head := g[0]
		title := head.ArtifactTitle
		if title == "" {
			title = head.ArtifactID
		}
		gOpen, gTot := countTodoOpenTotal(g)
		fmt.Fprintf(out, "  %s (%s)  [%d open / %d total]\n", title, head.ArtifactKind, gOpen, gTot)
		for _, t := range g {
			marker := "[ ]"
			if t.Done {
				marker = "[x]"
			}
			fmt.Fprintf(out, "    %s %s\n", marker, t.Text)
		}
		fmt.Fprintln(out)
	}
}

func groupTodosByArtifact(todos []store.TodoRow) [][]store.TodoRow {
	var out [][]store.TodoRow
	for _, t := range todos {
		if len(out) == 0 || out[len(out)-1][0].ArtifactID != t.ArtifactID {
			out = append(out, []store.TodoRow{t})
		} else {
			out[len(out)-1] = append(out[len(out)-1], t)
		}
	}
	return out
}

func countTodoOpenTotal(todos []store.TodoRow) (open, total int) {
	for _, t := range todos {
		total++
		if !t.Done {
			open++
		}
	}
	return
}

func filterTodoRows(todos []store.TodoRow, openOnly, doneOnly bool) []store.TodoRow {
	if !openOnly && !doneOnly {
		return todos
	}
	var result []store.TodoRow
	for _, t := range todos {
		if openOnly && !t.Done {
			result = append(result, t)
		}
		if doneOnly && t.Done {
			result = append(result, t)
		}
	}
	return result
}
