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

func (a *Adapter) AcceptsFile(rel string, size int64, cfg *config.RepoConfig) bool {
	if size > maxFileBytes {
		return false
	}
	if _, ok := sourceContextAdmissionReason(rel); !ok {
		return false
	}
	paths, rootCoverage := sourcePaths(cfg)
	if rootCoverage {
		return true
	}
	return withinConfiguredSourcePath(rel, paths)
}

func (a *Adapter) DiscoverFile(ctx context.Context, file adapters.FileCandidate, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.AcceptsFile(file.RelPath, file.Size, cfg) {
		return nil, nil
	}
	reason, _ := sourceContextAdmissionReason(file.RelPath)
	return []adapters.Candidate{{
		PrimaryPath: file.PrimaryPath,
		RelPath:     file.RelPath,
		AdapterName: sourceType,
		UnitBody:    string(file.Body),
		Metadata: map[string]any{
			"admission_reason": reason,
		},
	}}, nil
}

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
		reason, ok := sourceContextAdmissionReason(rel)
		if !ok {
			return
		}
		seen[rel] = true
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath: absPath,
			RelPath:     rel,
			AdapterName: sourceType,
			Metadata: map[string]any{
				"admission_reason": reason,
			},
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
	body := c.UnitBody
	if body == "" {
		data, err := os.ReadFile(c.PrimaryPath)
		if err != nil {
			return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
		}
		body = string(data)
	}
	title := sourceTitle(c.RelPath)
	extracted := map[string]any{
		"language":         sourceLanguage(c.RelPath),
		"admission_reason": "default_source_context",
	}
	for key, value := range c.Metadata {
		if key != "" && value != nil {
			extracted[key] = value
		}
	}
	art := adapters.Artifact{
		SourceIdentity: c.RelPath + "|" + sourceType,
		Kind:           config.KindSourceContext,
		Title:          title,
		Status:         "unknown",
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
		Extracted:      extracted,
		FormatProfile:  format.ProfileGeneric,
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

func withinConfiguredSourcePath(rel string, paths []string) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	for _, p := range paths {
		p = filepath.ToSlash(strings.Trim(strings.TrimSpace(p), "/"))
		if p == "" || rel == p || strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func isSourceContextFile(rel string) bool {
	_, ok := sourceContextAdmissionReason(rel)
	return ok
}

func sourceContextAdmissionReason(rel string) (string, bool) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	base := strings.ToLower(filepath.Base(rel))
	if isDefaultSourceContextBase(base) {
		return "default_source_context", true
	}
	if !isExpandedSourceContextBase(base) {
		return "", false
	}
	if isIntentBearingSourceContextPath(rel) {
		return "intent_bearing_source_context", true
	}
	if isTestFileLikeExpandedSourcePath(rel) {
		return "", false
	}
	if isImplementationRootSourceContextPath(rel) {
		return "implementation_root_source_context", true
	}
	if isTestSupportSourceContextPath(rel) {
		return "test_support_source_context", true
	}
	if isLikelyPackageSourceContextPath(rel) {
		return "package_source_context", true
	}
	return "", false
}

func isDefaultSourceContextBase(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".sql":
		return true
	default:
		return false
	}
}

func isExpandedSourceContextBase(base string) bool {
	switch base {
	case "dockerfile", "containerfile":
		return true
	}
	if strings.HasPrefix(base, "dockerfile.") || strings.HasPrefix(base, "containerfile.") ||
		strings.HasSuffix(base, ".dockerfile") || strings.HasSuffix(base, ".containerfile") {
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".py", ".go", ".rs", ".java", ".kt", ".kts", ".rb", ".php", ".toml":
		return true
	default:
		return false
	}
}

func isIntentBearingSourceContextPath(rel string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	base := filepath.Base(rel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	tokens := sourceContextIntentTokens(rel + "/" + stem)
	for _, token := range tokens {
		switch token {
		case "adr", "architecture", "behavior", "behaviour", "constraint", "contract",
			"decision", "design", "devspec", "devspecs", "intent", "invariant",
			"plan", "policy", "proposal", "requirement", "requirements", "rfc",
			"roadmap", "rule", "rules", "spec":
			return true
		}
	}
	return false
}

func sourceContextIntentTokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == '/' || r == '\\' || r == '-' || r == '_' || r == '.' || r == ' ' || r == '@'
	})
}

func isImplementationRootSourceContextPath(rel string) bool {
	parts := sourceContextPathParts(rel)
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "cmd", "internal", "pkg", "src", "lib", "app", "apps", "packages":
		return true
	case "crates":
		for _, part := range parts[1:] {
			if part == "src" {
				return true
			}
		}
	}
	return false
}

func isTestSupportSourceContextPath(rel string) bool {
	for _, part := range sourceContextPathParts(rel) {
		switch part {
		case "test", "tests", "__tests__", "spec", "integration", "e2e", "e2e-tests", "fixtures", "fixture", "valid_configs":
			return true
		}
	}
	return false
}

func isLikelyPackageSourceContextPath(rel string) bool {
	parts := sourceContextPathParts(rel)
	if len(parts) < 2 {
		return false
	}
	base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
	if filepath.Ext(base) != ".py" {
		return false
	}
	switch parts[0] {
	case "docs", "doc", "examples", "example", "scripts", "tools", "test", "tests", "e2e", "fixtures", "fixture", "node_modules", "vendor", "dist", "build":
		return false
	}
	return strings.Contains(parts[0], "_") || hasPythonPackageMarker(rel)
}

func hasPythonPackageMarker(rel string) bool {
	for _, part := range sourceContextPathParts(rel) {
		if part == "__init__" {
			return true
		}
	}
	return false
}

func isTestFileLikeExpandedSourcePath(rel string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	case ext == ".java" && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "it")):
		return true
	case (ext == ".kt" || ext == ".kts") && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "spec")):
		return true
	case ext == ".rs" && strings.HasSuffix(name, "_test"):
		return true
	default:
		return false
	}
}

func sourceContextPathParts(rel string) []string {
	rel = strings.Trim(strings.ToLower(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return nil
	}
	raw := strings.Split(rel, "/")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(strings.TrimSuffix(part, filepath.Ext(part)))
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
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
	base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
	switch base {
	case "dockerfile", "containerfile":
		return "dockerfile"
	}
	if strings.HasPrefix(base, "dockerfile.") || strings.HasPrefix(base, "containerfile.") ||
		strings.HasSuffix(base, ".dockerfile") || strings.HasSuffix(base, ".containerfile") {
		return "dockerfile"
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescript-react"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascript-react"
	case ".mjs", ".cjs":
		return "javascript"
	case ".vue":
		return "vue"
	case ".toml":
		return "toml"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}
