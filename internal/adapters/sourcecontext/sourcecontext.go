// Package sourcecontext indexes bounded source files as retrieval context.
package sourcecontext

import (
	"context"
	"fmt"
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

const (
	sourceType   = "source_context"
	maxFileBytes = 256 * 1024
)

// Adapter discovers source files that are useful as query-focused AI context.
type Adapter struct{}

func (a *Adapter) Name() string { return sourceType }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	paths, rootCoverage := sourcePaths(cfg)
	var candidates []adapters.Candidate
	seen := map[string]bool{}
	add := func(absPath string) {
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return
		}
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			return
		}
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() || info.Size() > maxFileBytes {
			return
		}
		if !isSourceContextFile(rel) {
			return
		}
		seen[rel] = true
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath: absPath,
			RelPath:     rel,
			AdapterName: sourceType,
		})
	}
	if rootCoverage {
		_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if isBuiltinIgnoredDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			add(path)
			return nil
		})
	} else {
		for _, p := range paths {
			dir := filepath.Join(repoRoot, filepath.FromSlash(p))
			_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				rel, relErr := filepath.Rel(repoRoot, path)
				if relErr != nil {
					return nil
				}
				rel = filepath.ToSlash(rel)
				if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, d.IsDir()) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if d.IsDir() {
					if isBuiltinIgnoredDir(d.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				add(path)
				return nil
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].RelPath < candidates[j].RelPath })
	return candidates, nil
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
	}
	body := string(data)
	title := sourceTitle(c.RelPath)
	art := adapters.Artifact{
		SourceIdentity: c.RelPath + "|" + sourceType,
		Kind:           config.KindSourceContext,
		Title:          title,
		Status:         "unknown",
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
		Extracted: map[string]any{
			"language": sourceLanguage(c.RelPath),
		},
		FormatProfile: format.ProfileGeneric,
	}
	src := adapters.Source{
		SourceType:     sourceType,
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileGeneric,
	}
	return art, []adapters.Source{src}, todoparse.ParseResult{}, nil
}

func sourcePaths(cfg *config.RepoConfig) ([]string, bool) {
	if cfg == nil {
		return nil, true
	}
	for _, src := range cfg.Sources {
		if src.Type != sourceType {
			continue
		}
		if len(src.Paths) > 0 {
			return src.Paths, false
		}
		if src.Path != "" {
			return []string{src.Path}, false
		}
		return nil, true
	}
	return nil, true
}

func isSourceContextFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".ts", ".tsx", ".js", ".jsx", ".sql":
		return true
	default:
		return false
	}
}

func isBuiltinIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".devspecs", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func sourceTitle(rel string) string {
	rel = filepath.ToSlash(rel)
	lang := sourceLanguage(rel)
	if lang == "" {
		return rel
	}
	return fmt.Sprintf("%s (%s)", rel, lang)
}

func sourceLanguage(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescript-react"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascript-react"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}
