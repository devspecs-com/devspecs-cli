package commands

import (
	"encoding/json"
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewShowCmd creates the ds show command.
func NewShowCmd() *cobra.Command {
	var (
		asJSON    bool
		content   bool
		noContent bool
		noRefresh bool
	)

	cmd := &cobra.Command{
		Use:     "show <id>",
		Aliases: []string{"get"},
		Short:   "Show artifact details",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args[0], asJSON, content, noContent, noRefresh)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&content, "content", false, "Include full content")
	cmd.Flags().BoolVar(&noContent, "no-content", false, "Exclude content")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	return cmd
}

func runShow(cmd *cobra.Command, idOrPrefix string, asJSON, showContent, noContent, noRefresh bool) error {
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
	links, _ := db.GetLinksForArtifact(art.ID)
	todos, _ := db.GetTodosForArtifact(art.ID)

	var rev *store.RevisionRow
	if art.CurrentRevID != "" {
		rev, _ = db.GetRevision(art.CurrentRevID)
	}

	if asJSON {
		obj := buildShowJSON(art, rev, sources, links, todos)
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "DevSpec: %s\n", art.ID)
	fmt.Fprintf(out, "\nTitle:\n  %s\n", art.Title)
	fmt.Fprintf(out, "\nKind:\n  %s\n", art.Kind)
	fmt.Fprintf(out, "\nStatus:\n  %s\n", art.Status)

	if len(sources) > 0 {
		fmt.Fprintf(out, "\nSource:\n  %s\n", sources[0].Path)
	}
	if rev != nil {
		fmt.Fprintf(out, "\nCurrent revision:\n  %s\n", rev.ContentHash)
	}

	if len(links) > 0 {
		fmt.Fprintln(out, "\nLinks:")
		for _, l := range links {
			fmt.Fprintf(out, "  [%s] %s\n", l.LinkType, l.Target)
		}
	}

	if len(todos) > 0 {
		open, done := countTodos(todos)
		fmt.Fprintf(out, "\nTodos: %d open, %d done, %d total\n", open, done, len(todos))
		for _, td := range todos {
			marker := "[ ]"
			if td.Done {
				marker = "[x]"
			}
			fmt.Fprintf(out, "  - %s %s\n", marker, td.Text)
		}
	}

	if !noContent && (showContent || rev != nil) && rev != nil {
		fmt.Fprintf(out, "\nContent:\n%s\n", rev.Body)
	}
	return nil
}

func buildShowJSON(art *store.ArtifactRow, rev *store.RevisionRow, sources []store.SourceRow, links []store.LinkRow, todos []store.TodoRow) map[string]any {
	obj := map[string]any{
		"id":     art.ID,
		"kind":   art.Kind,
		"title":  art.Title,
		"status": art.Status,
	}
	if len(sources) > 0 {
		obj["source_path"] = sources[0].Path
	}
	if rev != nil {
		obj["current_revision_hash"] = rev.ContentHash
		obj["body"] = rev.Body
	}
	if len(links) > 0 {
		linkObjs := make([]map[string]string, len(links))
		for i, l := range links {
			linkObjs[i] = map[string]string{"type": l.LinkType, "target": l.Target}
		}
		obj["links"] = linkObjs
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

func countTodos(todos []store.TodoRow) (open, done int) {
	for _, t := range todos {
		if t.Done {
			done++
		} else {
			open++
		}
	}
	return
}
