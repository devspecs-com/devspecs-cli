// Package adr implements the ADR (Architecture Decision Record) adapter.
package adr

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

var statusLineRe = regexp.MustCompile(`(?i)^[*\-]?\s*status:\s*(.+)$`)
var statusHeadingRe = regexp.MustCompile(`(?i)^#+\s*status\s*$`)

// Adapter discovers and parses ADR files.
type Adapter struct{}

func (a *Adapter) Name() string { return "adr" }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	paths := defaultADRPaths()
	if cfg != nil {
		for _, src := range cfg.Sources {
			if src.Type == "adr" {
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
		entries, err := walkMD(ctx, repoRoot, dir)
		if err != nil {
			continue
		}
		for _, absPath := range entries {
			rel, _ := filepath.Rel(repoRoot, absPath)
			rel = filepath.ToSlash(rel)
			candidates = append(candidates, adapters.Candidate{
				PrimaryPath: absPath,
				RelPath:     rel,
				AdapterName: "adr",
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

	title := extractTitle(content, c.RelPath)
	status := extractStatus(content)

	art := adapters.Artifact{
		SourceIdentity: c.RelPath + "|adr",
		Kind:           "adr",
		Title:          title,
		Status:         status,
		PrimaryPath:    c.PrimaryPath,
		Body:           content,
		Extracted:      make(map[string]any),
		FormatProfile:  format.ProfileADR,
	}

	src := adapters.Source{
		SourceType:     "adr",
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileADR,
	}

	todos := todoparse.Parse(content, c.RelPath)
	return art, []adapters.Source{src}, todos, nil
}

func defaultADRPaths() []string {
	return []string{"docs/adr", "docs/adrs", "adr", "adrs", "architecture/decisions"}
}

func walkMD(ctx context.Context, repoRoot, dir string) ([]string, error) {
	m := ignore.FromContext(ctx)
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(repoRoot, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if m != nil && m.ShouldSkip(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func extractTitle(content, relPath string) string {
	// Try frontmatter title
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		scanner := bufio.NewScanner(strings.NewReader(content))
		scanner.Scan() // skip ---
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "---" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "title:") {
				val := strings.TrimSpace(line[6:])
				if val != "" {
					return val
				}
			}
		}
	}

	// Try first H1
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}

	// Fallback to filename
	base := filepath.Base(relPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

func extractStatus(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inStatusSection := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check "Status: <value>" line pattern (common in ADRs)
		if m := statusLineRe.FindStringSubmatch(trimmed); m != nil {
			return normalizeStatus(m[1])
		}

		// Check status heading followed by content
		if statusHeadingRe.MatchString(trimmed) {
			inStatusSection = true
			continue
		}

		if inStatusSection && trimmed != "" {
			return normalizeStatus(trimmed)
		}
		if inStatusSection && strings.HasPrefix(trimmed, "#") {
			inStatusSection = false
		}
	}

	// Check frontmatter
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		fmScanner := bufio.NewScanner(strings.NewReader(content))
		fmScanner.Scan() // skip ---
		for fmScanner.Scan() {
			line := fmScanner.Text()
			if strings.TrimSpace(line) == "---" {
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "status:") {
				val := strings.TrimSpace(line[7:])
				if val != "" {
					return normalizeStatus(val)
				}
			}
		}
	}

	return "unknown"
}

func normalizeStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "accepted"), strings.Contains(s, "approved"):
		return "accepted"
	case strings.Contains(s, "proposed"):
		return "proposed"
	case strings.Contains(s, "rejected"), strings.Contains(s, "declined"):
		return "rejected"
	case strings.Contains(s, "deprecated"), strings.Contains(s, "superseded"):
		return "superseded"
	case strings.Contains(s, "draft"):
		return "draft"
	default:
		return s
	}
}
