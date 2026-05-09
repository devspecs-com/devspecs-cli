// Package markdown implements the generic markdown plan/spec adapter.
package markdown

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

// Adapter discovers and parses generic markdown plans/specs.
type Adapter struct{}

func (a *Adapter) Name() string { return "markdown" }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	paths := defaultPaths()
	if cfg != nil {
		for _, src := range cfg.Sources {
			if src.Type == "markdown" {
				if len(src.Paths) > 0 {
					paths = src.Paths
				} else if src.Path != "" {
					paths = []string{src.Path}
				}
				break
			}
		}
	}

	var candidates []adapters.Candidate
	for _, p := range paths {
		dir := filepath.Join(repoRoot, p)
		entries, err := walkMarkdownFiles(dir)
		if err != nil {
			continue
		}
		for _, absPath := range entries {
			rel, _ := filepath.Rel(repoRoot, absPath)
			rel = filepath.ToSlash(rel)
			candidates = append(candidates, adapters.Candidate{
				PrimaryPath: absPath,
				RelPath:     rel,
				AdapterName: "markdown",
			})
		}
	}
	return candidates, nil
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, []todoparse.Todo, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, nil, err
	}
	content := string(data)

	fm := parseFrontmatter(content)
	body := stripFrontmatter(content)

	title := fm["title"]
	if title == "" {
		title = extractFirstH1(body)
	}
	if title == "" {
		title = filenameTitle(c.RelPath)
	}

	kind := fm["kind"]
	if kind == "" {
		kind = inferKind(c.RelPath)
	}

	status := fm["status"]
	if status == "" {
		status = "unknown"
	}

	art := adapters.Artifact{
		SourceIdentity: c.RelPath + "|markdown",
		Kind:           kind,
		Title:          title,
		Status:         status,
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
		Extracted:      make(map[string]any),
	}

	if len(fm) > 0 {
		art.Extracted["frontmatter"] = fm
	}

	src := adapters.Source{
		SourceType:     "markdown",
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
	}

	todos := todoparse.Parse(content, c.RelPath)

	return art, []adapters.Source{src}, todos, nil
}

func defaultPaths() []string {
	return []string{"specs", "docs/specs", "plans", "docs/plans"}
}

func walkMarkdownFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func parseFrontmatter(content string) map[string]string {
	fm := make(map[string]string)
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fm
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Scan() // skip first ---
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			fm[key] = val
		}
	}
	return fm
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return content
	}
	// Find second ---
	idx := strings.Index(content[4:], "\n---")
	if idx < 0 {
		return content
	}
	rest := content[4+idx+4:]
	return strings.TrimLeft(rest, "\r\n")
}

func extractFirstH1(body string) string {
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

func filenameTitle(relPath string) string {
	base := filepath.Base(relPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	// Capitalize first letter of each word
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func inferKind(relPath string) string {
	lower := strings.ToLower(relPath)
	switch {
	case strings.Contains(lower, "plan"):
		return "plan"
	case strings.Contains(lower, "spec"):
		return "spec"
	case strings.Contains(lower, "requirement"):
		return "requirements"
	default:
		return "markdown_artifact"
	}
}
