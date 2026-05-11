// Package openspec implements the OpenSpec change proposal adapter.
package openspec

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

// Adapter discovers and parses OpenSpec change proposals.
type Adapter struct{}

func (a *Adapter) Name() string { return "openspec" }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	basePath := "openspec"
	if cfg != nil {
		for _, src := range cfg.Sources {
			if src.Type == "openspec" {
				if src.Path != "" {
					basePath = src.Path
				}
				break
			}
		}
	}

	changesDir := filepath.Join(repoRoot, basePath, "changes")
	if m := ignore.FromContext(ctx); m != nil {
		relBase := filepath.ToSlash(filepath.Join(basePath, "changes"))
		if m.ShouldSkip(relBase, true) {
			return nil, nil
		}
	}
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return nil, nil
	}

	var candidates []adapters.Candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		proposalPath := filepath.Join(changesDir, entry.Name(), "proposal.md")
		if _, err := os.Stat(proposalPath); err != nil {
			continue
		}
		rel, _ := filepath.Rel(repoRoot, proposalPath)
		rel = filepath.ToSlash(rel)
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			continue
		}
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath: proposalPath,
			RelPath:     rel,
			AdapterName: "openspec",
		})
	}
	return candidates, nil
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
	}
	content := string(data)

	changeDir := filepath.Dir(c.PrimaryPath)
	changeID := filepath.Base(changeDir)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(changeDir)))

	layoutGroup := filepath.ToSlash(filepath.Dir(c.RelPath))

	title := extractH1(content)
	if title == "" {
		title = humanize(changeID)
	}

	status := inferStatus(content)

	art := adapters.Artifact{
		SourceIdentity: filepath.ToSlash(c.RelPath[:strings.LastIndex(c.RelPath, "/")]) + "|openspec",
		Kind:           config.KindSpec,
		Subtype:        config.SubtypeOpenspecChange,
		Title:          title,
		Status:         status,
		PrimaryPath:    c.PrimaryPath,
		Body:           content,
		Extracted:      make(map[string]any),
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}

	src := adapters.Source{
		SourceType:     "openspec",
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}

	pr := todoparse.Parse(content, c.RelPath)

	tasksPath := filepath.Join(changeDir, "tasks.md")
	if tasksData, err := os.ReadFile(tasksPath); err == nil {
		tasksRel, _ := filepath.Rel(repoRoot, tasksPath)
		tasksRel = filepath.ToSlash(tasksRel)
		taskPR := todoparse.Parse(string(tasksData), tasksRel)
		off := len(pr.Todos)
		for i := range taskPR.Todos {
			taskPR.Todos[i].Ordinal = off + i
		}
		pr.Todos = append(pr.Todos, taskPR.Todos...)
		offC := len(pr.Criteria)
		for i := range taskPR.Criteria {
			taskPR.Criteria[i].Ordinal = offC + i
		}
		pr.Criteria = append(pr.Criteria, taskPR.Criteria...)
	}

	return art, []adapters.Source{src}, pr, nil
}

func extractH1(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

func humanize(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func inferStatus(content string) string {
	// Check for status in frontmatter or body
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "status: accepted") || strings.Contains(lower, "status: approved"):
		return "approved"
	case strings.Contains(lower, "status: rejected"):
		return "rejected"
	case strings.Contains(lower, "status: implementing"):
		return "implementing"
	case strings.Contains(lower, "status: implemented"):
		return "implemented"
	default:
		return "proposed"
	}
}
