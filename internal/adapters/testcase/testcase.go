// Package testcase indexes executable test cases as behavioral intent artifacts.
package testcase

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
	sourceType   = "test_case"
	maxFileBytes = 256 * 1024
	maxUnitLines = 120
)

// Adapter discovers and parses individual test cases as behavioral intent.
type Adapter struct{}

func (a *Adapter) Name() string { return sourceType }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	if cfg != nil && !cfg.Experiments.TestCaseArtifactsEnabled(false) {
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
		if statErr != nil || info.Size() > maxFileBytes || !isLikelyTestFile(rel) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, unit := range extractUnits(rel, string(data)) {
			candidates = append(candidates, adapters.Candidate{
				PrimaryPath:    path,
				RelPath:        rel,
				AdapterName:    sourceType,
				UnitName:       unit.Name,
				UnitParent:     unit.Parent,
				UnitBody:       unit.Body,
				UnitLanguage:   unit.Language,
				UnitFramework:  unit.Framework,
				UnitStartLine:  unit.StartLine,
				UnitEndLine:    unit.EndLine,
				UnitSymbols:    unit.Symbols,
				UnitAssertions: unit.Assertions,
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
	title := testTitle(c.UnitParent, c.UnitName)
	body := renderTestCaseBody(c)
	sourceIdentity := fmt.Sprintf("%s|%s|%d|%s", c.RelPath, sourceType, c.UnitStartLine, slug(c.UnitName))
	extracted := map[string]any{
		"mode":              "intent",
		"subtype":           config.SubtypeTestCase,
		"language":          c.UnitLanguage,
		"framework":         c.UnitFramework,
		"source_path":       c.RelPath,
		"start_line":        c.UnitStartLine,
		"end_line":          c.UnitEndLine,
		"test_name":         c.UnitName,
		"parent_title":      c.UnitParent,
		"symbols":           c.UnitSymbols,
		"assertion_terms":   c.UnitAssertions,
		"artifact_scope":    "section",
		"source_line_range": fmt.Sprintf("%d-%d", c.UnitStartLine, c.UnitEndLine),
	}
	art := adapters.Artifact{
		SourceIdentity: sourceIdentity,
		Kind:           config.KindSourceContext,
		Subtype:        config.SubtypeTestCase,
		Title:          title,
		Status:         "unknown",
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
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

type testUnit struct {
	Name       string
	Parent     string
	Body       string
	Language   string
	Framework  string
	StartLine  int
	EndLine    int
	Symbols    []string
	Assertions []string
}

type unitMarker struct {
	name      string
	parent    string
	startLine int
}

func extractUnits(rel, content string) []testUnit {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	language, framework := inferLanguageFramework(rel)
	markers := extractMarkers(rel, lines, language)
	if len(markers) == 0 {
		return nil
	}
	var units []testUnit
	for i, marker := range markers {
		endLine := len(lines)
		if i+1 < len(markers) {
			endLine = markers[i+1].startLine - 1
		}
		if endLine > marker.startLine+maxUnitLines-1 {
			endLine = marker.startLine + maxUnitLines - 1
		}
		body := strings.TrimRight(strings.Join(lines[marker.startLine-1:endLine], "\n"), "\r\n")
		symbols := extractSymbols(marker.name + "\n" + marker.parent + "\n" + body)
		assertions := extractAssertionTerms(body)
		units = append(units, testUnit{
			Name:       marker.name,
			Parent:     marker.parent,
			Body:       body,
			Language:   language,
			Framework:  framework,
			StartLine:  marker.startLine,
			EndLine:    endLine,
			Symbols:    symbols,
			Assertions: assertions,
		})
	}
	return units
}

func extractMarkers(rel string, lines []string, language string) []unitMarker {
	switch language {
	case "go":
		return extractGoMarkers(lines)
	case "python":
		return extractPythonMarkers(lines)
	case "ruby":
		return extractRubyMarkers(lines)
	case "php":
		return extractPHPMarkers(lines)
	default:
		if isJSTestFile(rel) {
			return extractJSMarkers(lines)
		}
		return nil
	}
}

func extractGoMarkers(lines []string) []unitMarker {
	var out []unitMarker
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "func Test") || !strings.Contains(trimmed, "(") {
			continue
		}
		name := strings.TrimSpace(trimmed[len("func "):strings.Index(trimmed, "(")])
		if strings.HasPrefix(name, "Test") {
			out = append(out, unitMarker{name: name, startLine: i + 1})
		}
	}
	return out
}

func extractPythonMarkers(lines []string) []unitMarker {
	var out []unitMarker
	parent := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "class Test") && strings.Contains(trimmed, ":") {
			parent = strings.TrimSuffix(strings.Fields(trimmed)[1], ":")
			parent = strings.TrimSuffix(parent, "(")
			continue
		}
		if strings.HasPrefix(trimmed, "def test_") && strings.Contains(trimmed, "(") {
			name := strings.TrimSpace(trimmed[len("def "):strings.Index(trimmed, "(")])
			out = append(out, unitMarker{name: name, parent: parent, startLine: i + 1})
		}
	}
	return out
}

func extractJSMarkers(lines []string) []unitMarker {
	var out []unitMarker
	parent := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name, ok := quotedCallTitle(trimmed, "describe"); ok {
			parent = name
			continue
		}
		if name, ok := quotedCallTitle(trimmed, "context"); ok {
			parent = name
			continue
		}
		if name, ok := quotedCallTitle(trimmed, "it"); ok {
			out = append(out, unitMarker{name: name, parent: parent, startLine: i + 1})
			continue
		}
		if name, ok := quotedCallTitle(trimmed, "test"); ok {
			out = append(out, unitMarker{name: name, parent: parent, startLine: i + 1})
		}
	}
	return out
}

func extractRubyMarkers(lines []string) []unitMarker {
	var out []unitMarker
	parent := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name, ok := rubyCallTitle(trimmed, "describe"); ok {
			parent = name
			continue
		}
		if name, ok := rubyCallTitle(trimmed, "context"); ok {
			parent = name
			continue
		}
		if name, ok := rubyCallTitle(trimmed, "it"); ok {
			out = append(out, unitMarker{name: name, parent: parent, startLine: i + 1})
		}
	}
	return out
}

func extractPHPMarkers(lines []string) []unitMarker {
	var out []unitMarker
	attributeTest := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#[Test") {
			attributeTest = true
			continue
		}
		idx := strings.Index(trimmed, "function ")
		if idx < 0 || !strings.Contains(trimmed[idx:], "(") {
			attributeTest = false
			continue
		}
		after := trimmed[idx+len("function "):]
		name := strings.TrimSpace(after[:strings.Index(after, "(")])
		if strings.HasPrefix(strings.ToLower(name), "test") || attributeTest {
			out = append(out, unitMarker{name: name, startLine: i + 1})
		}
		attributeTest = false
	}
	return out
}

func quotedCallTitle(line, call string) (string, bool) {
	for _, prefix := range []string{call + "(", call + ".only(", call + ".skip("} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return leadingQuoted(rest)
	}
	return "", false
}

func rubyCallTitle(line, call string) (string, bool) {
	if !strings.HasPrefix(line, call+" ") && !strings.HasPrefix(line, call+"(") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, call), "("))
	return leadingQuoted(rest)
}

func leadingQuoted(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	quote := s[0]
	if quote != '\'' && quote != '"' && quote != '`' {
		return "", false
	}
	for i := 1; i < len(s); i++ {
		if s[i] == quote && s[i-1] != '\\' {
			return s[1:i], true
		}
	}
	return "", false
}

func renderTestCaseBody(c adapters.Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Test: %s\n", c.UnitName)
	if c.UnitParent != "" {
		fmt.Fprintf(&b, "Parent: %s\n", c.UnitParent)
	}
	fmt.Fprintf(&b, "Source: %s\n", c.RelPath)
	fmt.Fprintf(&b, "Lines: %d-%d\n", c.UnitStartLine, c.UnitEndLine)
	if c.UnitLanguage != "" {
		fmt.Fprintf(&b, "Language: %s\n", c.UnitLanguage)
	}
	if c.UnitFramework != "" {
		fmt.Fprintf(&b, "Framework: %s\n", c.UnitFramework)
	}
	if len(c.UnitSymbols) > 0 {
		fmt.Fprintf(&b, "Symbols: %s\n", strings.Join(c.UnitSymbols, ", "))
	}
	if len(c.UnitAssertions) > 0 {
		fmt.Fprintf(&b, "Assertion vocabulary: %s\n", strings.Join(c.UnitAssertions, ", "))
	}
	fmt.Fprintf(&b, "\n```%s\n%s\n```\n", codeFenceLanguage(c.UnitLanguage), strings.TrimRight(c.UnitBody, "\r\n"))
	return b.String()
}

func isLikelyTestFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	lower := strings.ToLower(rel)
	base := strings.ToLower(filepath.Base(lower))
	ext := strings.ToLower(filepath.Ext(lower))
	if !supportedExt(ext) {
		return false
	}
	if isJSTestFile(lower) {
		return true
	}
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(strings.TrimSuffix(base, ext), "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	}
	parts := strings.Split(lower, "/")
	dirParts := []string{}
	if len(parts) > 1 {
		dirParts = parts[:len(parts)-1]
	}
	for _, segment := range dirParts {
		switch segment {
		case "tests", "__tests__", "spec", "cypress", "e2e":
			return true
		}
	}
	return false
}

func isJSTestFile(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".js" && ext != ".jsx" && ext != ".ts" && ext != ".tsx" && ext != ".mjs" && ext != ".cjs" {
		return false
	}
	name := strings.TrimSuffix(base, ext)
	return strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec") ||
		strings.Contains(filepath.ToSlash(strings.ToLower(rel)), "/tests/") ||
		strings.Contains(filepath.ToSlash(strings.ToLower(rel)), "/__tests__/") ||
		strings.Contains(filepath.ToSlash(strings.ToLower(rel)), "/cypress/") ||
		strings.Contains(filepath.ToSlash(strings.ToLower(rel)), "/e2e/")
}

func supportedExt(ext string) bool {
	switch ext {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php":
		return true
	default:
		return false
	}
}

func inferLanguageFramework(rel string) (string, string) {
	lower := strings.ToLower(filepath.ToSlash(rel))
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".go":
		return "go", "go test"
	case ".py":
		return "python", "pytest"
	case ".rb":
		return "ruby", "rspec"
	case ".php":
		return "php", "phpunit"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript", jsFramework(lower)
	case ".ts", ".tsx":
		return "typescript", jsFramework(lower)
	default:
		return "", ""
	}
}

func jsFramework(rel string) string {
	switch {
	case strings.Contains(rel, "/cypress/"):
		return "cypress"
	case strings.Contains(rel, "/e2e/") || strings.Contains(rel, "playwright"):
		return "playwright"
	default:
		return "javascript-test"
	}
}

func testTitle(parent, name string) string {
	if parent != "" {
		return parent + " > " + name
	}
	return name
}

func codeFenceLanguage(language string) string {
	switch language {
	case "typescript":
		return "ts"
	case "javascript":
		return "js"
	default:
		return language
	}
}

func extractSymbols(text string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(token string) {
		token = strings.Trim(token, "_-.")
		if len(token) < 3 || seen[token] || isStopToken(token) {
			return
		}
		seen[token] = true
		out = append(out, token)
	}
	for _, token := range tokenizeSymbolText(text) {
		add(strings.ToLower(token))
		for _, part := range splitCamelAndSeparators(token) {
			add(strings.ToLower(part))
		}
		if len(out) >= 24 {
			break
		}
	}
	sort.Strings(out)
	return out
}

func tokenizeSymbolText(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
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

func splitCamelAndSeparators(token string) []string {
	token = strings.ReplaceAll(token, "_", " ")
	token = strings.ReplaceAll(token, "-", " ")
	token = strings.ReplaceAll(token, ".", " ")
	var fields []string
	for _, field := range strings.Fields(token) {
		var b strings.Builder
		for i, r := range field {
			if i > 0 && unicode.IsUpper(r) && b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
			b.WriteRune(unicode.ToLower(r))
		}
		if b.Len() > 0 {
			fields = append(fields, b.String())
		}
	}
	return fields
}

func extractAssertionTerms(body string) []string {
	vocabulary := []string{
		"assert", "expect", "require", "should", "equal", "equals", "contain", "contains",
		"error", "exception", "panic", "status", "response", "retry", "permission",
		"auth", "billing", "analytics", "validation", "mock", "fixture", "snapshot",
	}
	bodyLower := strings.ToLower(body)
	var out []string
	for _, term := range vocabulary {
		if strings.Contains(bodyLower, term) {
			out = append(out, term)
		}
	}
	return out
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
		return "test"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func isStopToken(token string) bool {
	switch strings.ToLower(token) {
	case "test", "tests", "should", "when", "then", "with", "without", "from", "that", "this", "const", "return", "true", "false", "nil", "null", "none":
		return true
	default:
		return false
	}
}

func isBuiltinIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".devspecs", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor", ".venv", "__pycache__":
		return true
	default:
		return false
	}
}
