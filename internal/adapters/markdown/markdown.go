// Package markdown implements the generic markdown plan/spec adapter.
package markdown

import (
	"bufio"
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

// Adapter discovers and parses generic markdown plans/specs.
type Adapter struct{}

func (a *Adapter) Name() string { return "markdown" }

const (
	intentCandidateMinScore    = 4.0
	intentCandidateHeaderBytes = 32768
	intentCandidateMaxFiles    = 2000
)

type discoveryEvidence struct {
	score   float64
	reasons []string
}

type intentMarkdownCandidate struct {
	absPath string
	relPath string
	score   float64
	reasons []string
}

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	paths, rules, useDefaultCoverage := markdownSource(cfg)

	var candidates []adapters.Candidate
	seen := make(map[string]bool)
	addCandidate := func(absPath string, evidence discoveryEvidence) {
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			return
		}
		if seen[rel] {
			return
		}
		seen[rel] = true
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath:    absPath,
			RelPath:        rel,
			AdapterName:    "markdown",
			MarkdownPaths:  paths,
			MarkdownRules:  rules,
			DiscoveryScore: evidence.score,
			DiscoveryReasons: append([]string(nil),
				evidence.reasons...),
		})
	}

	for _, p := range paths {
		dir := filepath.Join(repoRoot, p)
		entries, err := walkMarkdownFiles(ctx, repoRoot, dir)
		if err != nil {
			continue
		}
		for _, absPath := range entries {
			addCandidate(absPath, discoveryEvidence{
				score:   10,
				reasons: []string{"configured_markdown_path"},
			})
		}
	}

	if useDefaultCoverage {
		entries, err := walkNestedDefaultMarkdownFiles(ctx, repoRoot)
		if err == nil {
			for _, absPath := range entries {
				addCandidate(absPath, discoveryEvidence{
					score:   8,
					reasons: []string{"default_nested_markdown_convention"},
				})
			}
		}
	}

	// Root-level glob patterns
	for _, pattern := range rootGlobs() {
		matches, _ := filepath.Glob(filepath.Join(repoRoot, pattern))
		for _, absPath := range matches {
			addCandidate(absPath, discoveryEvidence{
				score:   8,
				reasons: []string{"root_markdown_glob:" + pattern},
			})
		}
	}

	if cfg != nil && cfg.Experiments.IntentCandidateDiscovery {
		entries, err := walkIntentMarkdownCandidates(ctx, repoRoot)
		if err == nil {
			for _, entry := range entries {
				addCandidate(entry.absPath, discoveryEvidence{
					score:   entry.score,
					reasons: entry.reasons,
				})
			}
		}
	}

	return candidates, nil
}

func markdownSource(cfg *config.RepoConfig) (paths []string, rules []config.SourceRule, useDefaultCoverage bool) {
	paths = defaultPaths()
	if cfg == nil {
		return paths, nil, true
	}
	for _, src := range cfg.Sources {
		if src.Type != "markdown" {
			continue
		}
		if len(src.Paths) > 0 {
			return src.Paths, src.Rules, sameStrings(src.Paths, paths) && len(src.Rules) == 0
		}
		if src.Path != "" {
			return []string{src.Path}, src.Rules, false
		}
		break
	}
	return paths, nil, true
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
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

	kind := strings.TrimSpace(fm["kind"])
	subtype := strings.TrimSpace(fm["subtype"])
	if kind != "" {
		if err := config.ValidateKind(kind); err != nil {
			return adapters.Artifact{}, nil, todoparse.ParseResult{}, fmt.Errorf("%s: frontmatter kind: %w", c.RelPath, err)
		}
	}
	if subtype != "" {
		if kind == "" {
			return adapters.Artifact{}, nil, todoparse.ParseResult{}, fmt.Errorf("%s: frontmatter subtype requires kind", c.RelPath)
		}
		if err := config.ValidateSubtype(kind, subtype); err != nil {
			return adapters.Artifact{}, nil, todoparse.ParseResult{}, fmt.Errorf("%s: frontmatter subtype: %w", c.RelPath, err)
		}
	}

	var ruleTags []string
	if kind == "" {
		if rk, rs, rt, ok := MatchSourceRules(c.RelPath, c.MarkdownPaths, c.MarkdownRules); ok {
			kind, subtype, ruleTags = rk, rs, rt
		} else {
			kind, subtype = inferKindSubtype(c.RelPath)
		}
	}

	status := fm["status"]
	if status == "" {
		status = "unknown"
	}

	tags := parseFrontmatterTags(fm)
	tags = append(tags, ruleTags...)

	pathGen := pathGeneratorForExtract(c.RelPath)
	extracted := make(map[string]any)
	genExtract := pickGeneratorExtract(fm, pathGen)
	if genExtract != "" {
		extracted["generator"] = genExtract
	}
	if len(fm) > 0 {
		extracted["frontmatter"] = fm
	}

	prof := format.FromFrontmatterTool(fm["generator"], fm["tool"], fm["source"])
	if prof == format.ProfileGeneric {
		prof = format.FromPath(c.RelPath)
	}
	layout := format.LayoutGroup(c.RelPath)

	art := adapters.Artifact{
		SourceIdentity: c.RelPath + "|markdown",
		Kind:           kind,
		Subtype:        subtype,
		Title:          title,
		Status:         status,
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
		Extracted:      extracted,
		Tags:           tags,
		FormatProfile:  prof,
		LayoutGroup:    layout,
	}

	src := adapters.Source{
		SourceType:     "markdown",
		Path:           c.RelPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  prof,
		LayoutGroup:    layout,
	}

	pr := todoparse.Parse(content, c.RelPath)

	return art, []adapters.Source{src}, pr, nil
}

func defaultPaths() []string {
	return []string{
		"specs", "docs/specs", "plans", "docs/plans", ".cursor/plans",
		".claude/notes", "docs/prd", "rfcs", "rfc", "docs/rfcs", "docs/rfc",
		"docs/design", "docs/technical",
		"_bmad-output", ".specify/memory",
	}
}

func rootGlobs() []string {
	return []string{
		"*.spec.md", "*.plan.md", "*.prd.md",
		"*.rfc.md", "*.design.md", "*.contract.md", "*.requirements.md",
	}
}

func walkMarkdownFiles(ctx context.Context, repoRoot, dir string) ([]string, error) {
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

func walkNestedDefaultMarkdownFiles(ctx context.Context, repoRoot string) ([]string, error) {
	m := ignore.FromContext(ctx)
	var files []string
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, rerr := filepath.Rel(repoRoot, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if m != nil && m.ShouldSkip(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if isBuiltinIgnoredDir(info.Name()) {
				return filepath.SkipDir
			}
			if isDefaultNestedMarkdownDir(rel) {
				entries, walkErr := walkMarkdownFiles(ctx, repoRoot, path)
				if walkErr != nil {
					return walkErr
				}
				files = append(files, entries...)
				return filepath.SkipDir
			}
		}
		return nil
	})
	return files, err
}

func walkIntentMarkdownCandidates(ctx context.Context, repoRoot string) ([]intentMarkdownCandidate, error) {
	m := ignore.FromContext(ctx)
	var candidates []intentMarkdownCandidate
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, rerr := filepath.Rel(repoRoot, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if m != nil && m.ShouldSkip(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if isBuiltinIgnoredDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			return nil
		}
		score, reasons := scoreIntentMarkdownCandidate(path, rel)
		if score < intentCandidateMinScore {
			return nil
		}
		candidates = append(candidates, intentMarkdownCandidate{
			absPath: path,
			relPath: rel,
			score:   score,
			reasons: reasons,
		})
		return nil
	})
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].relPath < candidates[j].relPath
	})
	if len(candidates) > intentCandidateMaxFiles {
		candidates = candidates[:intentCandidateMaxFiles]
	}
	return candidates, err
}

func scoreIntentMarkdownCandidate(absPath, relPath string) (float64, []string) {
	score, reasons := scoreIntentPath(relPath)
	header := readIntentHeader(absPath)
	if header != "" {
		contentScore, contentReasons := scoreIntentContent(header)
		score += contentScore
		reasons = append(reasons, contentReasons...)
	}
	reasons = append(reasons, fmt.Sprintf("intent_candidate_score:%.2f", score))
	return score, reasons
}

func scoreIntentPath(relPath string) (float64, []string) {
	segments := strings.Split(filepath.ToSlash(relPath), "/")
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	lowerBase := strings.ToLower(base)
	filenameTokens := intentTokens(base)
	filenameSet := stringSet(filenameTokens)
	var dirTokens []string
	if len(segments) > 1 {
		for _, segment := range segments[:len(segments)-1] {
			dirTokens = append(dirTokens, intentTokens(segment)...)
		}
	}
	dirSet := stringSet(dirTokens)

	tokenWeights := map[string]float64{
		"adr":            4.5,
		"rfc":            4.5,
		"plan":           3.5,
		"design":         3.5,
		"proposal":       3.5,
		"decision":       3.5,
		"requirement":    3.2,
		"architecture":   2.4,
		"spec":           2.2,
		"implementation": 1.8,
		"migration":      1.8,
		"rollout":        1.8,
		"task":           1.7,
		"story":          1.7,
		"epic":           1.7,
		"milestone":      1.6,
		"roadmap":        1.6,
		"risk":           1.4,
	}

	allTokens := stringSet(append(append([]string{}, filenameTokens...), dirTokens...))
	var score float64
	var reasons []string
	if isAgentEntrypointMarkdownBase(lowerBase) {
		score += 4.5
		reasons = append(reasons, "intent_agent_entrypoint_filename:"+lowerBase)
	}
	for token, weight := range tokenWeights {
		if !allTokens[token] {
			continue
		}
		score += weight
		reasons = append(reasons, "intent_path_token:"+token)
		if filenameSet[token] {
			score += 1.0
			reasons = append(reasons, "intent_filename_token:"+token)
		}
		if dirSet[token] {
			score += 0.75
			reasons = append(reasons, "intent_directory_token:"+token)
		}
	}

	switch {
	case len(segments) == 1 && lowerBase == "readme":
		score -= 4.0
		reasons = append(reasons, "intent_negative:root_readme")
	case lowerBase == "readme":
		score -= 1.0
		reasons = append(reasons, "intent_negative:readme")
	}
	for _, token := range []string{"changelog", "license", "security", "contributing", "conduct", "release", "news", "template", "prompt"} {
		if allTokens[token] {
			score -= 1.5
			reasons = append(reasons, "intent_negative:"+token)
		}
	}
	return score, reasons
}

func isAgentEntrypointMarkdownBase(base string) bool {
	switch strings.Trim(base, " _.") {
	case "agents", "claude", "cursor", "codex", "gemini", "copilot", "memento":
		return true
	default:
		return false
	}
}

func scoreIntentContent(content string) (float64, []string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	headingWeights := map[string]float64{
		"goal":                 1.4,
		"non goal":             1.4,
		"context":              1.2,
		"background":           1.0,
		"decision":             1.8,
		"alternative":          1.5,
		"implementation plan":  1.8,
		"task":                 1.2,
		"acceptance criterion": 1.6,
		"risk":                 1.3,
		"rollout":              1.3,
		"open question":        1.3,
		"requirement":          1.5,
		"proposal":             1.4,
		"design":               1.4,
	}
	var score float64
	var reasons []string
	seen := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(trimmed), "do not edit") ||
			strings.Contains(strings.ToLower(trimmed), "generated file") {
			score -= 2.0
			reasons = append(reasons, "intent_negative:generated_marker")
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := normalizeIntentPhrase(strings.TrimLeft(trimmed, "# "))
		for phrase, weight := range headingWeights {
			if seen[phrase] || !strings.Contains(heading, phrase) {
				continue
			}
			seen[phrase] = true
			score += weight
			reasons = append(reasons, "intent_heading:"+strings.ReplaceAll(phrase, " ", "_"))
		}
	}
	return score, reasons
}

func readIntentHeader(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	buf := make([]byte, intentCandidateHeaderBytes)
	n, _ := file.Read(buf)
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

func intentTokens(value string) []string {
	var raw []string
	for _, part := range splitIntentTokenParts(value) {
		token := normalizeIntentToken(part)
		if token != "" {
			raw = append(raw, token)
		}
	}
	return uniqueStrings(raw)
}

func splitIntentTokenParts(value string) []string {
	var parts []string
	var b strings.Builder
	var prev rune
	flush := func() {
		if b.Len() > 0 {
			parts = append(parts, b.String())
			b.Reset()
		}
	}
	for _, r := range value {
		if r == '/' || r == '\\' || r == '_' || r == '-' || r == '.' || unicode.IsSpace(r) {
			flush()
			prev = 0
			continue
		}
		if prev != 0 && unicode.IsLower(prev) && unicode.IsUpper(r) {
			flush()
		}
		b.WriteRune(r)
		prev = r
	}
	flush()
	return parts
}

func normalizeIntentToken(value string) string {
	token := strings.ToLower(strings.TrimSpace(value))
	token = strings.Trim(token, " _-.")
	if len(token) < 2 {
		return ""
	}
	switch token {
	case "plans", "planning", "planned":
		return "plan"
	case "designs":
		return "design"
	case "proposals":
		return "proposal"
	case "decisions":
		return "decision"
	case "requirements":
		return "requirement"
	case "specs", "specification", "specifications":
		return "spec"
	case "architectural":
		return "architecture"
	case "stories":
		return "story"
	case "tasks":
		return "task"
	case "agents":
		return "agent"
	case "risks":
		return "risk"
	case "goals":
		return "goal"
	case "criteria":
		return "criterion"
	case "alternatives":
		return "alternative"
	case "questions":
		return "question"
	}
	if strings.HasPrefix(token, "plan") {
		return "plan"
	}
	return token
}

func normalizeIntentPhrase(value string) string {
	tokens := intentTokens(value)
	return strings.Join(tokens, " ")
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func isBuiltinIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func isDefaultNestedMarkdownDir(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == "." {
		return false
	}
	for _, suffix := range []string{
		"docs/specs",
		"docs/plans",
		".claude/notes",
		"docs/prd",
		"docs/rfcs",
		"docs/rfc",
		"docs/design",
		"docs/technical",
	} {
		if rel == suffix || strings.HasSuffix(rel, "/"+suffix) {
			return true
		}
	}
	return false
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func inferKindSubtype(relPath string) (kind, subtype string) {
	lower := strings.ToLower(relPath)
	switch {
	case strings.Contains(lower, "prd"):
		return config.KindRequirements, config.SubtypePRD
	case isRFCPath(lower):
		return config.KindDesign, ""
	case strings.Contains(lower, "plan"):
		return config.KindPlan, ""
	case strings.Contains(lower, "spec"):
		return config.KindSpec, ""
	case strings.Contains(lower, "requirement"):
		return config.KindRequirements, ""
	case strings.Contains(lower, "design"):
		return config.KindDesign, ""
	case strings.Contains(lower, "contract"):
		return config.KindContract, ""
	default:
		return config.KindMarkdownArtifact, ""
	}
}

func isRFCPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(relPath), "/")
	if relPath == "" {
		return false
	}
	segments := strings.Split(relPath, "/")
	for _, segment := range segments[:len(segments)-1] {
		if segment == "rfc" || segment == "rfcs" {
			return true
		}
	}
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	return base == "rfc" ||
		strings.HasPrefix(base, "rfc-") ||
		strings.HasPrefix(base, "rfc_") ||
		strings.HasSuffix(base, "-rfc") ||
		strings.HasSuffix(base, ".rfc") ||
		strings.Contains(base, "request-for-comments")
}

func inferKind(relPath string) string {
	k, _ := inferKindSubtype(relPath)
	return k
}

func pickGeneratorExtract(fm map[string]string, pathGen string) string {
	for _, key := range []string{"generator", "tool", "source"} {
		if v := strings.TrimSpace(fm[key]); v != "" {
			return v
		}
	}
	return pathGen
}

// pathGeneratorForExtract supplies Extracted["generator"] hint text from relPath (not user tags).
func pathGeneratorForExtract(relPath string) string {
	norm := filepath.ToSlash(relPath)

	if strings.Contains(norm, "_bmad-output/") {
		return "bmad-method"
	}

	dir := filepath.ToSlash(filepath.Dir(norm))
	base := filepath.Base(norm)
	if base == "spec.md" && strings.HasPrefix(dir, "specs/") && len(dir) > len("specs/") {
		return "speckit"
	}

	if strings.Contains(norm, ".cursor/plans/") {
		return "cursor-plan"
	}

	if strings.Contains(norm, ".claude/") {
		return "claude"
	}

	return ""
}

// parseFrontmatterTags extracts tags from frontmatter "tags" and "labels" keys.
// Supports: [auth, v2], auth, v2 (comma-separated), and single values.
func parseFrontmatterTags(fm map[string]string) []string {
	var tags []string
	for _, key := range []string{"tags", "labels"} {
		val, ok := fm[key]
		if !ok || val == "" {
			continue
		}
		val = strings.TrimPrefix(val, "[")
		val = strings.TrimSuffix(val, "]")
		for _, part := range strings.Split(val, ",") {
			t := strings.TrimSpace(part)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

// InferDirectoryTag extracts a meaningful tag from the source path's directory structure.
// Returns empty string for generic directories and root-level files.
func InferDirectoryTag(relPath string) string {
	genericDirs := map[string]bool{
		"plans": true, "specs": true, "docs": true,
		".cursor": true, "openspec": true, "changes": true,
		"adr": true, "adrs": true,
		"_bmad-output": true, "planning-artifacts": true, "implementation-artifacts": true,
		"checklists": true, "contracts": true,
		".specify": true,
	}

	normalized := filepath.ToSlash(relPath)
	dir := filepath.ToSlash(filepath.Dir(normalized))
	if dir == "." || dir == "" {
		return ""
	}

	parts := strings.Split(dir, "/")
	for _, p := range parts {
		if p == "" || genericDirs[p] {
			continue
		}
		if isSpeckitFeatureSegment(p) {
			continue
		}
		return p
	}
	return ""
}

func isSpeckitFeatureSegment(seg string) bool {
	if len(seg) < 5 {
		return false
	}
	for i := 0; i < 3 && i < len(seg); i++ {
		if seg[i] < '0' || seg[i] > '9' {
			return false
		}
	}
	return seg[3] == '-'
}
