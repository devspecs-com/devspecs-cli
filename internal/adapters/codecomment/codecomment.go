// Package codecomment indexes high-signal source comments as intent artifacts.
package codecomment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

const (
	sourceType      = "code_comment"
	maxFileBytes    = 256 * 1024
	maxCommentLines = 40
)

// Adapter discovers implementation-local rationale, constraints, and TODOs.
type Adapter struct{}

func (a *Adapter) Name() string { return sourceType }

func (a *Adapter) AcceptsFile(rel string, size int64, cfg *config.RepoConfig) bool {
	if cfg != nil && !cfg.CodeCommentArtifactsEnabled(false) {
		return false
	}
	return size <= maxFileBytes && isSupportedCodeFile(rel)
}

func (a *Adapter) DiscoverFile(ctx context.Context, file adapters.FileCandidate, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.AcceptsFile(file.RelPath, file.Size, cfg) {
		return nil, nil
	}
	var candidates []adapters.Candidate
	for _, unit := range extractComments(file.RelPath, string(file.Body)) {
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath:   file.PrimaryPath,
			RelPath:       file.RelPath,
			AdapterName:   sourceType,
			UnitName:      unit.Title,
			UnitBody:      unit.Text,
			UnitLanguage:  unit.Language,
			UnitStartLine: unit.StartLine,
			UnitEndLine:   unit.EndLine,
			UnitSymbols:   unit.Symbols,
			Role:          unit.Role,
		})
	}
	return candidates, nil
}

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	if cfg != nil && !cfg.CodeCommentArtifactsEnabled(false) {
		return nil, nil
	}
	var candidates []adapters.Candidate
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
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
		info, statErr := d.Info()
		if statErr != nil || info.Size() > maxFileBytes || !isSupportedCodeFile(rel) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, unit := range extractComments(rel, string(data)) {
			candidates = append(candidates, adapters.Candidate{
				PrimaryPath:   path,
				RelPath:       rel,
				AdapterName:   sourceType,
				UnitName:      unit.Title,
				UnitBody:      unit.Text,
				UnitLanguage:  unit.Language,
				UnitStartLine: unit.StartLine,
				UnitEndLine:   unit.EndLine,
				UnitSymbols:   unit.Symbols,
				Role:          unit.Role,
			})
		}
		return nil
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RelPath == candidates[j].RelPath {
			return candidates[i].UnitStartLine < candidates[j].UnitStartLine
		}
		return candidates[i].RelPath < candidates[j].RelPath
	})
	return candidates, err
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	title := c.UnitName
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s comment at line %d", c.RelPath, c.UnitStartLine)
	}
	sourceIdentity := fmt.Sprintf("%s|%s|%d|%s", c.RelPath, sourceType, c.UnitStartLine, slug(title))
	extracted := map[string]any{
		"mode":              "intent",
		"subtype":           config.SubtypeCodeComment,
		"comment_role":      c.Role,
		"language":          c.UnitLanguage,
		"source_path":       c.RelPath,
		"start_line":        c.UnitStartLine,
		"end_line":          c.UnitEndLine,
		"symbols":           c.UnitSymbols,
		"artifact_scope":    "section",
		"source_line_range": fmt.Sprintf("%d-%d", c.UnitStartLine, c.UnitEndLine),
	}
	art := adapters.Artifact{
		SourceIdentity: sourceIdentity,
		Kind:           config.KindSourceContext,
		Subtype:        config.SubtypeCodeComment,
		Title:          title,
		Status:         "unknown",
		PrimaryPath:    c.PrimaryPath,
		Body:           renderCommentBody(c),
		Extracted:      extracted,
		FormatProfile:  format.ProfileGeneric,
		LayoutGroup:    c.RelPath,
	}
	src := adapters.Source{
		SourceType:     sourceType,
		Path:           c.RelPath,
		SourceIdentity: sourceIdentity,
		FormatProfile:  format.ProfileGeneric,
		LayoutGroup:    c.RelPath,
	}
	return art, []adapters.Source{src}, todoparse.ParseResult{}, nil
}

type commentUnit struct {
	Title     string
	Text      string
	Role      string
	Language  string
	StartLine int
	EndLine   int
	Symbols   []string
}

func extractComments(rel, content string) []commentUnit {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	language := sourceLanguage(rel)
	var out []commentUnit
	add := func(start, end int, items []string) {
		text := cleanCommentText(items)
		role, ok := classifyIntentComment(text, start)
		if !ok {
			return
		}
		if end-start+1 > maxCommentLines {
			end = start + maxCommentLines - 1
		}
		out = append(out, commentUnit{
			Title:     commentTitle(text, rel, start),
			Text:      text,
			Role:      role,
			Language:  language,
			StartLine: start,
			EndLine:   end,
			Symbols:   extractSymbols(text),
		})
	}

	var group []string
	groupStart := 0
	flushGroup := func(end int) {
		if len(group) > 0 {
			add(groupStart, end, group)
			group = nil
			groupStart = 0
		}
	}

	inBlock := false
	blockStart := 0
	var block []string
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if inBlock {
			text := trimmed
			if idx := strings.Index(text, "*/"); idx >= 0 {
				block = append(block, text[:idx])
				add(blockStart, lineNo, block)
				block = nil
				inBlock = false
				continue
			}
			block = append(block, text)
			continue
		}
		if strings.HasPrefix(trimmed, "/*") {
			flushGroup(lineNo - 1)
			blockStart = lineNo
			text := strings.TrimPrefix(trimmed, "/*")
			if idx := strings.Index(text, "*/"); idx >= 0 {
				add(blockStart, lineNo, []string{text[:idx]})
				continue
			}
			inBlock = true
			block = []string{text}
			continue
		}
		if text, ok := lineCommentText(trimmed, language); ok {
			if len(group) == 0 {
				groupStart = lineNo
			}
			group = append(group, text)
			continue
		}
		flushGroup(lineNo - 1)
	}
	flushGroup(len(lines))
	if inBlock {
		add(blockStart, len(lines), block)
	}
	return out
}

func lineCommentText(trimmed, language string) (string, bool) {
	switch language {
	case "python", "ruby":
		if strings.HasPrefix(trimmed, "#!") {
			return "", false
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimPrefix(trimmed, "#"), true
		}
	case "php":
		if strings.HasPrefix(trimmed, "//") {
			return strings.TrimPrefix(trimmed, "//"), true
		}
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimPrefix(trimmed, "#"), true
		}
	default:
		if strings.HasPrefix(trimmed, "//") {
			return strings.TrimPrefix(trimmed, "//"), true
		}
	}
	return "", false
}

func cleanCommentText(lines []string) string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func classifyIntentComment(text string, startLine int) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" || len(lower) < 12 {
		return "", false
	}
	if startLine <= 40 && containsAny(lower, "copyright", "licensed under", "permission is hereby granted", "apache license", "mit license") &&
		!containsAny(lower, "todo", "fixme", "hack", "workaround", "invariant", "assumption") {
		return "", false
	}
	switch {
	case containsAny(lower, "todo", "fixme", "xxx"):
		return "todo", true
	case containsAny(lower, "hack", "workaround", "temporary", "for now"):
		return "workaround", true
	case containsAny(lower, "invariant", "must always", "never", "do not", "don't", "cannot", "must not"):
		return "invariant", true
	case containsAny(lower, "because", "rationale", "reason", "why", "so that"):
		return "rationale", true
	case containsAny(lower, "assume", "assumption", "guarantee", "constraint", "contract"):
		return "constraint", true
	case containsAny(lower, "security", "permission", "auth", "sanitize", "validate", "validation"):
		return "security", true
	case containsAny(lower, "compatibility", "backwards compatible", "legacy", "migration"):
		return "compatibility", true
	case containsAny(lower, "edge case", "regression", "expected behavior", "behaviour", "behavior"):
		return "behavior", true
	default:
		return "", false
	}
}

func renderCommentBody(c adapters.Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Comment: %s\n", c.UnitName)
	fmt.Fprintf(&b, "Source: %s\n", c.RelPath)
	fmt.Fprintf(&b, "Lines: %d-%d\n", c.UnitStartLine, c.UnitEndLine)
	if c.UnitLanguage != "" {
		fmt.Fprintf(&b, "Language: %s\n", c.UnitLanguage)
	}
	if c.Role != "" {
		fmt.Fprintf(&b, "Role: %s\n", c.Role)
	}
	if len(c.UnitSymbols) > 0 {
		fmt.Fprintf(&b, "Symbols: %s\n", strings.Join(c.UnitSymbols, ", "))
	}
	fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(c.UnitBody))
	return strings.TrimRight(b.String(), "\r\n")
}

func commentTitle(text, rel string, line int) string {
	first := strings.TrimSpace(strings.Split(text, "\n")[0])
	first = strings.Trim(first, "`*_")
	if first == "" {
		return fmt.Sprintf("%s comment at line %d", rel, line)
	}
	if len(first) > 96 {
		first = strings.TrimSpace(first[:96])
	}
	return first
}

func isSupportedCodeFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php", ".java", ".kt", ".kts", ".rs":
		return true
	default:
		return false
	}
}

func sourceLanguage(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	default:
		return ""
	}
}

func extractSymbols(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range tokenize(text) {
		for _, token := range splitIdentifierLike(raw) {
			token = strings.ToLower(strings.Trim(token, "_-."))
			if len(token) < 3 || isStopToken(token) || seen[token] {
				continue
			}
			seen[token] = true
			out = append(out, token)
			if len(out) >= 20 {
				sort.Strings(out)
				return out
			}
		}
	}
	sort.Strings(out)
	return out
}

func tokenize(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func splitIdentifierLike(token string) []string {
	fields := strings.FieldsFunc(token, func(r rune) bool { return r == '_' || r == '-' || r == '.' })
	if len(fields) == 0 {
		return nil
	}
	var out []string
	for _, field := range fields {
		var b strings.Builder
		for i, r := range field {
			if i > 0 && unicode.IsUpper(r) && b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			b.WriteRune(unicode.ToLower(r))
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	return out
}

func isStopToken(token string) bool {
	switch strings.ToLower(token) {
	case "comment", "source", "line", "lines", "this", "that", "with", "from", "into", "return", "true", "false", "null", "none", "todo", "fixme":
		return true
	default:
		return false
	}
}

func slug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() > 0 && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "comment"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func isBuiltinIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".devspecs", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor", ".venv", "__pycache__":
		return true
	default:
		return false
	}
}
