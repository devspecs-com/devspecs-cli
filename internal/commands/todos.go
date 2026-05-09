package commands

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewTodosCmd creates the ds todos command.
func NewTodosCmd() *cobra.Command {
	var (
		openOnly bool
		doneOnly bool
		asJSON   bool
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
			return runTodos(cmd, artifactID, openOnly, doneOnly, asJSON)
		},
	}

	cmd.Flags().BoolVar(&openOnly, "open", false, "Show only incomplete todos")
	cmd.Flags().BoolVar(&doneOnly, "done", false, "Show only completed todos")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runTodos(cmd *cobra.Command, artifactID string, openOnly, doneOnly, asJSON bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

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
		return outputTodos(cmd, todos, asJSON)
	}

	todos, err := db.ListAllTodos("", openOnly, doneOnly)
	if err != nil {
		return fmt.Errorf("list todos: %w", err)
	}
	return outputTodos(cmd, todos, asJSON)
}

func outputTodos(cmd *cobra.Command, todos []store.TodoRow, asJSON bool) error {
	if asJSON {
		type jsonTodo struct {
			ArtifactID string `json:"artifact_id"`
			RevisionID string `json:"revision_id"`
			Ordinal    int    `json:"ordinal"`
			Text       string `json:"text"`
			Done       bool   `json:"done"`
			SourceFile string `json:"source_file"`
			SourceLine int    `json:"source_line"`
		}
		out := make([]jsonTodo, len(todos))
		for i, t := range todos {
			out[i] = jsonTodo{
				ArtifactID: t.ArtifactID,
				RevisionID: t.RevisionID,
				Ordinal:    t.Ordinal,
				Text:       t.Text,
				Done:       t.Done,
				SourceFile: t.SourceFile,
				SourceLine: t.SourceLine,
			}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "STATUS\tTEXT\tSOURCE\n")
	for _, t := range todos {
		marker := "[ ]"
		if t.Done {
			marker = "[x]"
		}
		source := fmt.Sprintf("%s:%d", t.SourceFile, t.SourceLine)
		fmt.Fprintf(w, "%s\t%s\t%s\n", marker, t.Text, source)
	}
	w.Flush()
	return nil
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
