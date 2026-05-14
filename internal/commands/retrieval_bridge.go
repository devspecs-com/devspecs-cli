package commands

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const commandTokenCounterName = "approx_chars_div_4"

func loadRetrievalCandidates(db *store.DB, fp store.FilterParams) ([]retrieval.Candidate, error) {
	artifacts, err := db.ListArtifacts(fp)
	if err != nil {
		return nil, err
	}
	candidates := make([]retrieval.Candidate, 0, len(artifacts))
	for _, art := range artifacts {
		sources, _ := db.GetSourcesForArtifact(art.ID)
		todos, _ := db.GetTodosForArtifact(art.ID)
		var body string
		if art.CurrentRevID != "" {
			if rev, err := db.GetRevision(art.CurrentRevID); err == nil && rev != nil {
				body = rev.Body
			}
		}
		candidates = append(candidates, artifactCandidate(art, sources, todos, body))
	}
	return candidates, nil
}

func artifactCandidate(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body string) retrieval.Candidate {
	sourcePath := firstSourcePath(sources)
	path := sourcePath
	if path == "" {
		path = art.Title
	}
	if path == "" {
		path = art.ID
	}
	return retrieval.Candidate{
		ID:      art.ID,
		Path:    filepath.ToSlash(path),
		Kind:    art.Kind,
		Subtype: art.Subtype,
		Title:   art.Title,
		Status:  art.Status,
		Source:  filepath.ToSlash(sourcePath),
		Body:    renderRetrievalCandidateBody(art, sources, todos, body),
		Metadata: map[string]string{
			"repo_id":              art.RepoID,
			"short_id":             art.ShortID,
			"current_revision_id":  art.CurrentRevID,
			"created_at":           art.CreatedAt,
			"updated_at":           art.UpdatedAt,
			"last_observed_at":     art.LastObservedAt,
			"token_counter":        commandTokenCounterName,
			"retrieval_candidate":  "sqlite_artifact",
			"source_context_scope": "indexed_artifacts",
		},
	}
}

func renderRetrievalCandidateBody(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", art.Title)
	fmt.Fprintf(&b, "Kind: %s\n", art.Kind)
	if art.Subtype != "" {
		fmt.Fprintf(&b, "Subtype: %s\n", art.Subtype)
	}
	fmt.Fprintf(&b, "Status: %s\n", art.Status)
	for _, src := range sources {
		if src.Path != "" {
			fmt.Fprintf(&b, "Source: %s\n", filepath.ToSlash(src.Path))
		}
		if src.FormatProfile != "" {
			fmt.Fprintf(&b, "Format profile: %s\n", src.FormatProfile)
		}
		if src.LayoutGroup != "" {
			fmt.Fprintf(&b, "Layout group: %s\n", src.LayoutGroup)
		}
	}
	if len(todos) > 0 {
		fmt.Fprintln(&b, "\nTasks:")
		for _, td := range todos {
			marker := "[ ]"
			if td.Done {
				marker = "[x]"
			}
			fmt.Fprintf(&b, "- %s %s\n", marker, td.Text)
		}
	}
	if strings.TrimSpace(body) != "" {
		fmt.Fprintf(&b, "\n%s", strings.TrimRight(body, "\r\n"))
	}
	return b.String()
}

func firstSourcePath(sources []store.SourceRow) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" {
			return filepath.ToSlash(src.Path)
		}
	}
	return ""
}

func approximateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func capCandidates(candidates []retrieval.Candidate, limit int) []retrieval.Candidate {
	if limit <= 0 || len(candidates) <= limit {
		return candidates
	}
	return candidates[:limit]
}

func shortCandidateID(c retrieval.Candidate) string {
	if c.Metadata != nil && c.Metadata["short_id"] != "" {
		return c.Metadata["short_id"]
	}
	if len(c.ID) > 8 {
		return c.ID[:8]
	}
	return c.ID
}
