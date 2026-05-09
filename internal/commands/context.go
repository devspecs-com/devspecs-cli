package commands

import (
	"encoding/json"
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewContextCmd creates the ds context command.
func NewContextCmd() *cobra.Command {
	var (
		asJSON bool
		copy_  bool
	)

	cmd := &cobra.Command{
		Use:   "context <id>",
		Short: "Export agent-ready context for an artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContext(cmd, args[0], asJSON, copy_)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&copy_, "copy", false, "Copy output to clipboard")
	return cmd
}

func runContext(cmd *cobra.Command, idOrPrefix string, asJSON, copyToClipboard bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	art, err := db.GetArtifact(idOrPrefix)
	if err != nil {
		return err
	}

	sources, _ := db.GetSourcesForArtifact(art.ID)
	todos, _ := db.GetTodosForArtifact(art.ID)

	var rev *store.RevisionRow
	if art.CurrentRevID != "" {
		rev, _ = db.GetRevision(art.CurrentRevID)
	}

	var sourcePath string
	if len(sources) > 0 {
		sourcePath = sources[0].Path
	}

	if asJSON {
		obj := buildContextJSON(art, rev, sourcePath, todos)
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	output := buildContextMarkdown(art, rev, sourcePath, todos)
	fmt.Fprint(cmd.OutOrStdout(), output)
	return nil
}

func buildContextMarkdown(art *store.ArtifactRow, rev *store.RevisionRow, sourcePath string, todos []store.TodoRow) string {
	var s string
	s += fmt.Sprintf("# DevSpecs Context: %s\n\n", art.Title)
	s += fmt.Sprintf("DevSpec ID: %s\n", art.ID)
	s += fmt.Sprintf("Kind: %s\n", art.Kind)
	s += fmt.Sprintf("Status: %s\n", art.Status)
	s += fmt.Sprintf("Source: %s\n", sourcePath)

	s += "\n## Instructions for Agent\n\n"
	s += "Use this artifact as the source of truth for the requested implementation or review.\n\n"
	s += "Preserve the acceptance criteria.\n"
	s += "Do not silently change scope.\n"
	s += "If implementation diverges from this artifact, explicitly record the deviation.\n"

	openTodos := filterTodos(todos, false)
	doneTodos := filterTodos(todos, true)

	if len(openTodos) > 0 || len(doneTodos) > 0 {
		s += "\n## Extracted Tasks\n\n"
		for _, td := range todos {
			marker := "- [ ]"
			if td.Done {
				marker = "- [x]"
			}
			s += fmt.Sprintf("%s %s\n", marker, td.Text)
		}
	}

	if rev != nil {
		s += "\n## Source Content\n\n"
		s += rev.Body + "\n"
	}

	return s
}

func buildContextJSON(art *store.ArtifactRow, rev *store.RevisionRow, sourcePath string, todos []store.TodoRow) map[string]any {
	obj := map[string]any{
		"id":          art.ID,
		"kind":        art.Kind,
		"title":       art.Title,
		"status":      art.Status,
		"source_path": sourcePath,
	}
	if rev != nil {
		obj["body"] = rev.Body
	}
	if len(todos) > 0 {
		todoObjs := make([]map[string]any, len(todos))
		for i, td := range todos {
			todoObjs[i] = map[string]any{
				"ordinal":     td.Ordinal,
				"text":        td.Text,
				"done":        td.Done,
				"source_file": td.SourceFile,
				"source_line": td.SourceLine,
			}
		}
		obj["todos"] = todoObjs
	}
	return obj
}

func filterTodos(todos []store.TodoRow, done bool) []store.TodoRow {
	var result []store.TodoRow
	for _, t := range todos {
		if t.Done == done {
			result = append(result, t)
		}
	}
	return result
}
