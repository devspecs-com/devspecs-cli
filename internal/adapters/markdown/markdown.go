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
	supportDocMinScore         = 4.0
	supportDocMaxFiles         = 200
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
		seenKey := rel
		if os.PathSeparator == '\\' {
			seenKey = strings.ToLower(seenKey)
		}
		if seen[seenKey] {
			return
		}
		seen[seenKey] = true
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

	for _, absPath := range rootStandardMarkdownFiles(repoRoot) {
		addCandidate(absPath, discoveryEvidence{
			score:   8,
			reasons: []string{"root_standard_intent_doc"},
		})
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

	if cfg != nil && cfg.Experiments.IntentCandidateDiscoveryEnabled(false) {
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
	if cfg != nil && cfg.Experiments.SupportDocDiscoveryEnabled(false) {
		supportEntries, err := walkSupportMarkdownCandidates(ctx, repoRoot)
		if err == nil {
			for _, entry := range supportEntries {
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
		".claude/notes", ".claude/plans", ".codex/plans", ".codex/notes",
		".agents/skills", ".claude/skills", ".codex/skills", ".cursor/commands", ".windsurf/workflows", "agents",
		"docs/prd", "docs/product-specs", "product-specs", "docs/requirements", "requirements",
		"rfcs", "rfc", "RFCS", "docs/rfcs", "docs/rfc", "docs/RFCS",
		"roadmaps", "docs/roadmaps",
		"docs/design", "docs/design-docs", "design-docs", "docs/technical",
		"architecture", "docs/architecture",
		"_bmad-output", ".specify/memory",
	}
}

func rootGlobs() []string {
	return []string{
		"*.spec.md", "*.plan.md", "*.prd.md",
		"*.rfc.md", "*.roadmap.md", "*.design.md", "*.contract.md", "*.requirements.md",
		"REQ_*.md", "REQ-*.md", "*_REQ.md", "*-REQ.md",
		"*.spec.mdx", "*.plan.mdx", "*.prd.mdx",
		"*.rfc.mdx", "*.roadmap.mdx", "*.design.mdx", "*.contract.mdx", "*.requirements.mdx",
		"REQ_*.mdx", "REQ-*.mdx", "*_REQ.mdx", "*-REQ.mdx",
	}
}

func rootStandardMarkdownFiles(repoRoot string) []string {
	standard := map[string]bool{
		"ROADMAP.md":       true,
		"PLAN.md":          true,
		"DESIGN.md":        true,
		"ARCHITECTURE.md":  true,
		"PRD.md":           true,
		"RFC.md":           true,
		"SPEC.md":          true,
		"REQUIREMENTS.md":  true,
		"ROADMAP.mdx":      true,
		"PLAN.mdx":         true,
		"DESIGN.mdx":       true,
		"ARCHITECTURE.mdx": true,
		"PRD.mdx":          true,
		"RFC.mdx":          true,
		"SPEC.mdx":         true,
		"REQUIREMENTS.mdx": true,
	}
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !standard[entry.Name()] {
			continue
		}
		out = append(out, filepath.Join(repoRoot, entry.Name()))
	}
	sort.Strings(out)
	return out
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
		if isMarkdownLikeFilename(info.Name()) {
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
		if isOpenSpecOwnedPath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
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
			return nil
		}
		if isMarkdownLikeFilename(info.Name()) && isDefaultHighSignalMarkdownFile(rel) {
			files = append(files, path)
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
		if isOpenSpecOwnedPath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
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
		if !isMarkdownLikeFilename(info.Name()) {
			return nil
		}
		if isDefaultHighSignalMarkdownFile(rel) {
			candidates = append(candidates, intentMarkdownCandidate{
				absPath: path,
				relPath: rel,
				score:   8,
				reasons: []string{"intent_high_signal_filename"},
			})
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

func walkSupportMarkdownCandidates(ctx context.Context, repoRoot string) ([]intentMarkdownCandidate, error) {
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
		if isOpenSpecOwnedPath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
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
		if !isMarkdownLikeFilename(info.Name()) {
			return nil
		}
		score, reasons := scoreSupportMarkdownCandidate(path, rel)
		if score < supportDocMinScore {
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
	if len(candidates) > supportDocMaxFiles {
		candidates = candidates[:supportDocMaxFiles]
	}
	return candidates, err
}

func isMarkdownLikeFilename(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdx":
		return true
	default:
		return false
	}
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

func scoreSupportMarkdownCandidate(absPath, relPath string) (float64, []string) {
	score, reasons := scoreSupportPath(relPath)
	if score >= supportDocMinScore {
		header := readIntentHeader(absPath)
		contentScore, contentReasons := scoreSupportContent(header)
		score += contentScore
		reasons = append(reasons, contentReasons...)
	}
	reasons = append(reasons, fmt.Sprintf("support_doc_score:%.2f", score))
	return score, reasons
}

func scoreSupportPath(relPath string) (float64, []string) {
	relPath = filepath.ToSlash(relPath)
	segments := strings.Split(relPath, "/")
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	lowerBase := strings.ToLower(base)
	var tokens []string
	for _, segment := range segments {
		tokens = append(tokens, supportTokens(segment)...)
	}
	tokenSet := stringSet(tokens)
	var score float64
	var reasons []string
	pathWeights := map[string]float64{
		"access":         1.1,
		"ansible":        4.0,
		"authentication": 3.2,
		"authorization":  2.6,
		"automation":     1.6,
		"awx":            3.4,
		"control":        1.0,
		"deployment":     1.5,
		"failover":       2.5,
		"integration":    1.8,
		"logging":        2.0,
		"manager":        1.8,
		"metric":         3.5,
		"observability":  2.8,
		"operator":       2.2,
		"playbook":       3.0,
		"rbac":           3.0,
		"runtime":        1.6,
		"security":       1.8,
		"service":        0.8,
		"statefulset":    2.0,
		"telemetry":      2.8,
		"tracing":        2.0,
		"instance":       1.8,
	}
	for token, weight := range pathWeights {
		if !tokenSet[token] {
			continue
		}
		score += weight
		reasons = append(reasons, "support_path_token:"+token)
	}
	if tokenSet["access"] && tokenSet["control"] {
		score += 2.4
		reasons = append(reasons, "support_path_phrase:access_control")
	}
	if tokenSet["core"] && tokenSet["service"] {
		score += 1.2
		reasons = append(reasons, "support_path_phrase:core_service")
	}
	if tokenSet["instance"] && tokenSet["manager"] {
		score += 2.2
		reasons = append(reasons, "support_path_phrase:instance_manager")
	}
	if strings.Contains(strings.ToLower(relPath), "docs/src/") {
		score += 0.8
		reasons = append(reasons, "support_path_convention:docs_src")
	}
	switch {
	case len(segments) == 1 && lowerBase == "readme":
		score -= 4.0
		reasons = append(reasons, "support_negative:root_readme")
	case lowerBase == "readme":
		score -= 1.5
		reasons = append(reasons, "support_negative:readme")
	}
	for _, token := range []string{"changelog", "license", "news", "release", "template", "prompt", "skill", "fixture", "sample", "example", "tutorial"} {
		if tokenSet[token] {
			score -= 2.0
			reasons = append(reasons, "support_negative:"+token)
		}
	}
	return score, reasons
}

func scoreSupportContent(content string) (float64, []string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	headingWeights := map[string]float64{
		"access control":   2.6,
		"authentication":   2.6,
		"authorization":    2.2,
		"configuration":    1.1,
		"deployment":       1.4,
		"failover":         2.0,
		"instance manager": 2.4,
		"integration":      1.4,
		"logging":          1.6,
		"metric":           2.4,
		"observability":    2.2,
		"operator":         1.8,
		"playbook":         2.4,
		"rbac":             2.3,
		"security":         1.3,
		"statefulset":      1.8,
		"telemetry":        2.0,
		"tracing":          1.6,
	}
	var score float64
	var reasons []string
	seen := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "do not edit") ||
			strings.Contains(lower, "generated file") {
			score -= 2.0
			reasons = append(reasons, "support_negative:generated_marker")
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := normalizeSupportPhrase(strings.TrimLeft(trimmed, "# "))
		for phrase, weight := range headingWeights {
			if seen[phrase] || !strings.Contains(heading, phrase) {
				continue
			}
			seen[phrase] = true
			score += weight
			reasons = append(reasons, "support_heading:"+strings.ReplaceAll(phrase, " ", "_"))
		}
	}
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
		"bep":            4.0,
		"kep":            4.0,
		"osep":           4.0,
		"ship":           4.0,
		"sip":            4.0,
		"tep":            4.0,
		"plan":           3.5,
		"design":         3.5,
		"proposal":       3.5,
		"enhancement":    3.4,
		"decision":       3.5,
		"requirement":    3.2,
		"architecture":   3.1,
		"spec":           2.2,
		"implementation": 1.8,
		"migration":      1.8,
		"rollout":        1.8,
		"task":           1.7,
		"story":          1.7,
		"epic":           1.7,
		"milestone":      1.6,
		"roadmap":        2.6,
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
		effectiveWeight := weight
		if isProposalFamilyPathToken(token) && dirSet[token] && !filenameSet[token] {
			effectiveWeight = 2.1
		}
		score += effectiveWeight
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
	if lowerBase == "skill" {
		score -= 6.0
		reasons = append(reasons, "intent_negative:skill_file")
	}
	if hasProposalSupportPath(segments) {
		score -= 4.0
		reasons = append(reasons, "intent_negative:proposal_support_path")
	}
	for _, token := range []string{"changelog", "license", "security", "contributing", "conduct", "release", "news", "template", "prompt", "skill", "reference", "guide", "tutorial", "example", "fixture", "sample"} {
		if allTokens[token] {
			score -= 1.5
			reasons = append(reasons, "intent_negative:"+token)
		}
	}
	return score, reasons
}

func isProposalFamilyPathToken(token string) bool {
	switch token {
	case "bep", "enhancement", "kep", "osep", "proposal", "ship", "sip", "tep":
		return true
	default:
		return false
	}
}

func hasProposalSupportPath(segments []string) bool {
	if len(segments) < 3 {
		return false
	}
	hasProposalFamily := false
	hasSupportSegment := false
	for _, segment := range segments[:len(segments)-1] {
		for _, token := range intentTokens(segment) {
			if isProposalFamilyPathToken(token) {
				hasProposalFamily = true
			}
			switch token {
			case "asset", "context", "example", "experiment", "fixture", "idea", "legacy", "reference", "research", "sample", "script", "template", "update":
				hasSupportSegment = true
			}
		}
	}
	return hasProposalFamily && hasSupportSegment
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
		"summary":              1.3,
		"abstract":             1.3,
		"motivation":           1.5,
		"context":              1.2,
		"background":           1.0,
		"decision":             1.8,
		"alternative":          1.5,
		"drawback":             1.3,
		"unresolved question":  1.4,
		"implementation plan":  1.8,
		"detailed design":      1.5,
		"technical design":     1.5,
		"design consideration": 1.4,
		"task":                 1.2,
		"acceptance criterion": 1.6,
		"risk":                 1.3,
		"rollout":              1.3,
		"open question":        1.3,
		"requirement":          1.5,
		"proposal":             1.4,
		"design":               1.4,
		"architecture":         1.4,
		"constraint":           1.2,
		"component":            1.1,
		"dependency":           1.1,
		"milestone":            1.3,
		"timeline":             1.2,
		"now next later":       1.6,
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

func supportTokens(value string) []string {
	var raw []string
	for _, part := range splitIntentTokenParts(value) {
		token := normalizeSupportToken(part)
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
	case "roadmaps":
		return "roadmap"
	case "designs":
		return "design"
	case "proposals":
		return "proposal"
	case "enhancements":
		return "enhancement"
	case "beps":
		return "bep"
	case "keps":
		return "kep"
	case "oseps":
		return "osep"
	case "ships":
		return "ship"
	case "sips":
		return "sip"
	case "teps":
		return "tep"
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
	case "skills":
		return "skill"
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
	case "constraints":
		return "constraint"
	case "components":
		return "component"
	case "dependencies":
		return "dependency"
	case "drawbacks":
		return "drawback"
	case "milestones":
		return "milestone"
	case "timelines":
		return "timeline"
	case "guides":
		return "guide"
	case "tutorials":
		return "tutorial"
	case "examples":
		return "example"
	case "fixtures":
		return "fixture"
	case "samples":
		return "sample"
	case "assets":
		return "asset"
	case "ideas":
		return "idea"
	case "scripts":
		return "script"
	case "updates":
		return "update"
	}
	if strings.HasPrefix(token, "plan") {
		return "plan"
	}
	return token
}

func normalizeSupportToken(value string) string {
	token := normalizeIntentToken(value)
	switch token {
	case "auth", "authenticate", "authenticated", "authenticating", "authentication", "login", "signin", "signon":
		return "authentication"
	case "authorize", "authorized", "authorizing", "authorization", "permission", "permissions":
		return "authorization"
	case "metrics":
		return "metric"
	case "observable", "observability":
		return "observability"
	case "telemetry":
		return "telemetry"
	case "trace", "traces", "tracing":
		return "tracing"
	case "log", "logs", "logging":
		return "logging"
	case "operators":
		return "operator"
	case "instances":
		return "instance"
	case "managers", "management":
		return "manager"
	case "statefulsets", "stateful-set", "stateful-sets":
		return "statefulset"
	case "deploy", "deployment", "deployments":
		return "deployment"
	case "integrate", "integrated", "integration", "integrations":
		return "integration"
	case "playbooks":
		return "playbook"
	case "services":
		return "service"
	case "controls":
		return "control"
	case "examples":
		return "example"
	case "samples":
		return "sample"
	case "fixtures":
		return "fixture"
	case "tutorials":
		return "tutorial"
	default:
		return token
	}
}

func normalizeIntentPhrase(value string) string {
	tokens := intentTokens(value)
	return strings.Join(tokens, " ")
}

func normalizeSupportPhrase(value string) string {
	tokens := supportTokens(value)
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
		".agents/skills",
		".claude/notes",
		".claude/skills",
		".codex/skills",
		".cursor/commands",
		".windsurf/workflows",
		"agents",
		"docs/prd",
		"docs/product-specs",
		"product-specs",
		"docs/requirements",
		"requirements",
		"docs/rfcs",
		"docs/rfc",
		"docs/RFCS",
		"RFCS",
		"roadmaps",
		"docs/roadmaps",
		"docs/design",
		"docs/design-docs",
		"design-docs",
		"docs/technical",
		"architecture",
		"docs/architecture",
	} {
		if rel == suffix || strings.HasSuffix(rel, "/"+suffix) {
			return true
		}
	}
	return false
}

func isDefaultHighSignalMarkdownFile(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(strings.ToLower(rel)), "/")
	base := filepath.Base(rel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	switch stem {
	case "agents", "agent", "claude", "codex", "gemini", "memento",
		"maintainers", "governance", "proposal_template", "proposal-template",
		"prd_template", "prd-template", "requirements_template", "requirements-template":
		return true
	default:
		return strings.HasSuffix(stem, ".agent")
	}
}

func isOpenSpecOwnedPath(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "openspec" || strings.HasPrefix(rel, "openspec/") || strings.HasSuffix(rel, "/openspec") {
		return true
	}
	return strings.Contains(rel, "/openspec/")
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
	case isSkillPath(lower):
		return config.KindMarkdownArtifact, config.SubtypeSkill
	case isAgentInstructionPath(lower):
		return config.KindMarkdownArtifact, config.SubtypeAgentInstruction
	case isProtocolPolicyPath(lower):
		return config.KindMarkdownArtifact, config.SubtypeGovernancePolicy
	case isTemplatePath(lower):
		return config.KindMarkdownArtifact, config.SubtypeDocumentTemplate
	case strings.Contains(lower, "prd"):
		return config.KindRequirements, config.SubtypePRD
	case strings.Contains(lower, "product-spec") || isReqPath(lower):
		return config.KindRequirements, config.SubtypePRD
	case isRFCProposalPath(lower):
		return config.KindDesign, ""
	case strings.Contains(lower, "plan") || strings.Contains(lower, "roadmap") || isStoryPath(lower):
		return config.KindPlan, ""
	case strings.Contains(lower, "spec"):
		return config.KindSpec, ""
	case strings.Contains(lower, "requirement"):
		return config.KindRequirements, ""
	case strings.Contains(lower, "design") || strings.Contains(lower, "architecture"):
		return config.KindDesign, ""
	case strings.Contains(lower, "contract"):
		return config.KindContract, ""
	default:
		return config.KindMarkdownArtifact, ""
	}
}

func isSkillPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	base := filepath.Base(relPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return stem == "skill" ||
		stem == "skills" ||
		strings.Contains(relPath, "/skills/")
}

func isAgentInstructionPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	base := filepath.Base(relPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	switch stem {
	case "agents", "agent", "claude", "codex", "gemini", "cursor", "memento", ".cursorrules":
		return true
	default:
		return strings.HasSuffix(stem, ".agent") ||
			strings.Contains(relPath, "/agents/") ||
			strings.Contains(relPath, ".cursor/commands/") ||
			strings.Contains(relPath, ".windsurf/workflows/")
	}
}

func isTemplatePath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	base := filepath.Base(relPath)
	return strings.Contains(relPath, "/templates/") ||
		strings.Contains(relPath, "/template/") ||
		strings.Contains(base, "template")
}

func isProtocolPolicyPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	base := filepath.Base(relPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	switch stem {
	case "maintainers", "governance":
		return true
	default:
		return false
	}
}

func isReqPath(relPath string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(relPath)))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.HasPrefix(base, "req_") ||
		strings.HasPrefix(base, "req-") ||
		strings.HasSuffix(stem, "_req") ||
		strings.HasSuffix(stem, "-req")
}

func isStoryPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	base := filepath.Base(relPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.HasSuffix(stem, ".story") ||
		strings.Contains(relPath, "/stories/") ||
		strings.Contains(relPath, "/story/")
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

func isRFCProposalPath(relPath string) bool {
	relPath = strings.Trim(filepath.ToSlash(strings.ToLower(relPath)), "/")
	if relPath == "" {
		return false
	}
	if isRFCPath(relPath) {
		return true
	}
	segments := strings.Split(relPath, "/")
	for _, segment := range segments[:len(segments)-1] {
		switch segment {
		case "proposal", "proposals", "enhancement", "enhancements",
			"kep", "keps", "tep", "teps", "bep", "beps",
			"sip", "sips", "ship", "ships", "osep", "oseps",
			"design", "designs", "design-docs":
			return true
		}
	}
	base := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	return base == "proposal" ||
		strings.HasPrefix(base, "proposal-") ||
		strings.HasPrefix(base, "proposal_") ||
		strings.HasSuffix(base, "-proposal") ||
		strings.Contains(base, "request-for-feedback")
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

	if format.FromPath(norm) == format.ProfileSpeckit {
		return "speckit"
	}

	if strings.Contains(norm, ".cursor/plans/") {
		return "cursor-plan"
	}

	if strings.Contains(norm, ".claude/") {
		return "claude"
	}
	if strings.Contains(norm, ".codex/") {
		return "codex"
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
