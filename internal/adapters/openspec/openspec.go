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
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath: proposalPath,
			RelPath:     rel,
			AdapterName: "openspec",
		})
	}
	return candidates, nil
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, []todoparse.Todo, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, nil, err
	}
	content := string(data)

	changeDir := filepath.Dir(c.PrimaryPath)
	changeID := filepath.Base(changeDir)
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(changeDir)))

	title := extractH1(content)
	if title == "" {
		title = humanize(changeID)
	}

	status := inferStatus(content, changeDir)

	art := adapters.Artifact{
		SourceIdentity: filepath.ToSlash(c.RelPath[:strings.LastIndex(c.RelPath, "/")]) + "|openspec",
		Kind:           "openspec_change",
		Title:          title,
		Status:         status,
		PrimaryPath:    c.PrimaryPath,
		Body:           content,
		Extracted:      make(map[string]any),
	}

	// Extract acceptance criteria from proposal
	criteria := extractCriteria(content)
	if len(criteria) > 0 {
		art.Extracted["acceptance_criteria"] = criteria
	}

	src := adapters.Source{
		SourceType:     "openspec",
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
	}

	// Collect todos from proposal
	todos := todoparse.Parse(content, c.RelPath)

	// Also parse tasks.md if it exists
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if tasksData, err := os.ReadFile(tasksPath); err == nil {
		tasksRel, _ := filepath.Rel(repoRoot, tasksPath)
		tasksRel = filepath.ToSlash(tasksRel)
		taskTodos := todoparse.Parse(string(tasksData), tasksRel)
		// Re-ordinal relative to existing todos
		offset := len(todos)
		for i := range taskTodos {
			taskTodos[i].Ordinal = offset + i
		}
		todos = append(todos, taskTodos...)
	}

	return art, []adapters.Source{src}, todos, nil
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

func inferStatus(content, changeDir string) string {
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

var criteriaHeadings = []string{
	"acceptance criteria",
	"requirements",
	"scenarios",
	"success criteria",
}

func extractCriteria(content string) []string {
	var criteria []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	inCriteriaSection := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "#") {
			heading := strings.TrimLeft(lower, "# ")
			isCriteria := false
			for _, h := range criteriaHeadings {
				if strings.Contains(heading, h) {
					isCriteria = true
					break
				}
			}
			if isCriteria {
				inCriteriaSection = true
				continue
			}
			if inCriteriaSection {
				break
			}
		}

		if inCriteriaSection && strings.HasPrefix(trimmed, "- ") {
			criteria = append(criteria, strings.TrimPrefix(trimmed, "- "))
		}
	}
	return criteria
}
