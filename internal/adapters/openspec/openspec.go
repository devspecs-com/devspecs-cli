// Package openspec implements the OpenSpec change proposal adapter.
package openspec

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
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
		if entry.Name() == "archive" {
			archiveDir := filepath.Join(changesDir, entry.Name())
			archiveEntries, err := os.ReadDir(archiveDir)
			if err != nil {
				continue
			}
			for _, archived := range archiveEntries {
				if archived.IsDir() {
					candidates = append(candidates, discoverChangeCandidates(ctx, repoRoot, filepath.Join(archiveDir, archived.Name()))...)
				}
			}
			continue
		}
		candidates = append(candidates, discoverChangeCandidates(ctx, repoRoot, filepath.Join(changesDir, entry.Name()))...)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].RelPath < candidates[j].RelPath })
	return candidates, nil
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
	}
	content := string(data)

	changeDir := changeDirForPath(c.PrimaryPath)
	changeID := filepath.Base(changeDir)
	repoRoot := repoRootForChangeDir(changeDir)
	role := roleForRelPath(c.RelPath)

	layoutGroup := filepath.ToSlash(filepath.Dir(c.RelPath))
	if relChangeDir, err := filepath.Rel(repoRoot, changeDir); err == nil {
		layoutGroup = filepath.ToSlash(relChangeDir)
	}

	title := extractH1(content)
	if title == "" {
		title = humanize(changeID)
		if role != "" && role != "proposal" {
			title += " " + humanize(role)
		}
	}

	status := inferStatus(content)

	art := adapters.Artifact{
		SourceIdentity: filepath.ToSlash(c.RelPath) + "|openspec",
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
	if role == "proposal" {
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
	}

	return art, []adapters.Source{src}, pr, nil
}

func discoverChangeCandidates(ctx context.Context, repoRoot, changeDir string) []adapters.Candidate {
	var out []adapters.Candidate
	add := func(absPath string) {
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			return
		}
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			return
		}
		out = append(out, adapters.Candidate{
			PrimaryPath: absPath,
			RelPath:     rel,
			AdapterName: "openspec",
		})
	}
	for _, name := range []string{"proposal.md", "design.md", "tasks.md"} {
		add(filepath.Join(changeDir, name))
	}
	specsDir := filepath.Join(changeDir, "specs")
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		add(path)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func changeDirForPath(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(path) == "spec.md" {
		for {
			if filepath.Base(filepath.Dir(dir)) == "specs" {
				return filepath.Dir(filepath.Dir(dir))
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return filepath.Dir(path)
			}
			dir = parent
		}
	}
	return dir
}

func repoRootForChangeDir(changeDir string) string {
	dir := filepath.Clean(changeDir)
	for {
		if filepath.Base(dir) == "changes" {
			return filepath.Dir(filepath.Dir(dir))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(filepath.Dir(filepath.Dir(changeDir)))
		}
		dir = parent
	}
}

func roleForRelPath(rel string) string {
	base := filepath.Base(filepath.ToSlash(rel))
	switch base {
	case "proposal.md":
		return "proposal"
	case "design.md":
		return "design"
	case "tasks.md":
		return "tasks"
	case "spec.md":
		return "spec delta"
	default:
		return ""
	}
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
