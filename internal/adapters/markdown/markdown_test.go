package markdown

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestDiscover_DefaultPaths(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "refactor.md"), []byte("# Refactor\n"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].RelPath != "plans/refactor.md" {
		t.Errorf("expected rel path 'plans/refactor.md', got %q", candidates[0].RelPath)
	}
}

func TestDiscover_RootStandardIntentDocs(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{"ROADMAP.md", "PLAN.md", "DESIGN.md", "ARCHITECTURE.md", "README.md"} {
		writeMarkdown(t, tmp, rel, "# "+strings.TrimSuffix(rel, ".md")+"\n\n- [ ] follow-up\n")
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"ROADMAP.md", "PLAN.md", "DESIGN.md", "ARCHITECTURE.md"} {
		if findCandidate(candidates, rel).RelPath == "" {
			t.Fatalf("missing root intent doc %s in %#v", rel, candidateRelPaths(candidates))
		}
	}
	if findCandidate(candidates, "README.md").RelPath != "" {
		t.Fatalf("root README should not be included by standard intent globs: %#v", candidateRelPaths(candidates))
	}
}

func TestDiscover_ConfigPaths(t *testing.T) {
	tmp := t.TempDir()
	customDir := filepath.Join(tmp, "my-plans")
	os.MkdirAll(customDir, 0o755)
	os.WriteFile(filepath.Join(customDir, "plan.md"), []byte("# Plan\n"), 0o644)

	cfg := &config.RepoConfig{
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{"my-plans"}},
		},
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestDiscover_DefaultNestedDocsIntentDirs(t *testing.T) {
	tmp := t.TempDir()
	nestedPlan := filepath.Join(tmp, "apps", "desktop", "docs", "plans")
	nestedPRD := filepath.Join(tmp, "services", "api", "docs", "prd")
	nestedRFC := filepath.Join(tmp, "packages", "api", "docs", "rfcs")
	nestedProposal := filepath.Join(tmp, "services", "api", "docs", "proposals")
	nestedArchitecture := filepath.Join(tmp, "platform", "docs", "architecture")
	nestedDesignDocs := filepath.Join(tmp, "runtime", "docs", "design-docs")
	if err := os.MkdirAll(nestedPlan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedPRD, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedRFC, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedProposal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedArchitecture, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedDesignDocs, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(nestedPlan, "pnpm-migration.md"), []byte("# PNPM Migration\n"), 0o644)
	os.WriteFile(filepath.Join(nestedPRD, "billing.md"), []byte("# Billing PRD\n"), 0o644)
	os.WriteFile(filepath.Join(nestedRFC, "token-boundary.md"), []byte("# Token Boundary RFC\n"), 0o644)
	os.WriteFile(filepath.Join(nestedProposal, "search-index.md"), []byte("# Search Index Proposal\n"), 0o644)
	os.WriteFile(filepath.Join(nestedArchitecture, "system-boundaries.md"), []byte("# System Boundaries Architecture\n"), 0o644)
	os.WriteFile(filepath.Join(nestedDesignDocs, "worker-runtime.md"), []byte("# Worker Runtime Design\n"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"apps/desktop/docs/plans/pnpm-migration.md",
		"services/api/docs/prd/billing.md",
		"packages/api/docs/rfcs/token-boundary.md",
		"services/api/docs/proposals/search-index.md",
		"platform/docs/architecture/system-boundaries.md",
		"runtime/docs/design-docs/worker-runtime.md",
	} {
		if !stringSliceContains(got, want) {
			t.Fatalf("missing nested default intent doc %q in %v", want, got)
		}
	}
}

func TestDiscover_CustomConfigDoesNotAddNestedDefaults(t *testing.T) {
	tmp := t.TempDir()
	customDir := filepath.Join(tmp, "my-plans")
	nestedDir := filepath.Join(tmp, "apps", "desktop", "docs", "plans")
	os.MkdirAll(customDir, 0o755)
	os.MkdirAll(nestedDir, 0o755)
	os.WriteFile(filepath.Join(customDir, "plan.md"), []byte("# Plan\n"), 0o644)
	os.WriteFile(filepath.Join(nestedDir, "hidden.md"), []byte("# Hidden\n"), 0o644)

	cfg := &config.RepoConfig{
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{"my-plans"}},
		},
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := candidateRelPaths(candidates)
	if !stringSliceContains(got, "my-plans/plan.md") {
		t.Fatalf("missing configured markdown candidate in %v", got)
	}
	if stringSliceContains(got, "apps/desktop/docs/plans/hidden.md") {
		t.Fatalf("custom config should not add nested defaults, got %v", got)
	}
}

func TestDiscover_ExperimentalIntentDiscoveryFindsGenericCompoundPlanningDirs(t *testing.T) {
	tmp := t.TempDir()
	writeMarkdown(t, tmp, "docs/exec-plans/active/cache-warmup.md", "# Cache Warmup\n\n## Goals\n\n## Implementation Plan\n")
	writeMarkdown(t, tmp, "docs/designDocs/auth-boundary.md", "# Auth Boundary\n\n## Context\n\n## Alternatives\n")
	writeMarkdown(t, tmp, "docs/project_planning/migration.md", "# Migration\n\n## Rollout\n")
	writeMarkdown(t, tmp, ".github/AGENTS.md", "# Agent Rules\n\n## Rules\n")
	writeMarkdown(t, tmp, "examples/agent/browser_agent/build_in_prompt/browser_agent_task_decomposition_prompt.md", "# Browser Automation Task Decomposition\n\n## Objective\n")
	writeMarkdown(t, tmp, "README.md", "# Project\n")
	writeMarkdown(t, tmp, "CHANGELOG.md", "# Changelog\n")
	writeMarkdown(t, tmp, ".github/pull_request_template.md", "# Pull Request\n")

	a := &Adapter{}
	baseline, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	baselinePaths := candidateRelPaths(baseline)
	if stringSliceContains(baselinePaths, "docs/exec-plans/active/cache-warmup.md") {
		t.Fatalf("baseline unexpectedly discovered exec-plans path: %v", baselinePaths)
	}

	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	if err != nil {
		t.Fatal(err)
	}
	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"docs/exec-plans/active/cache-warmup.md",
		"docs/designDocs/auth-boundary.md",
		"docs/project_planning/migration.md",
		".github/AGENTS.md",
	} {
		if !stringSliceContains(got, want) {
			t.Fatalf("missing experimental intent candidate %q in %v", want, got)
		}
	}
	for _, noisy := range []string{
		"README.md",
		"CHANGELOG.md",
		".github/pull_request_template.md",
		"examples/agent/browser_agent/build_in_prompt/browser_agent_task_decomposition_prompt.md",
	} {
		if stringSliceContains(got, noisy) {
			t.Fatalf("experimental discovery should not admit noisy maintenance doc %q in %v", noisy, got)
		}
	}

	candidate := findCandidate(candidates, "docs/exec-plans/active/cache-warmup.md")
	if candidate.DiscoveryScore < intentCandidateMinScore {
		t.Fatalf("discovery score = %.2f, want >= %.2f", candidate.DiscoveryScore, intentCandidateMinScore)
	}
	if !hasReasonPrefix(candidate.DiscoveryReasons, "intent_path_token:plan") {
		t.Fatalf("expected plan path-token reason, got %#v", candidate.DiscoveryReasons)
	}
	if !hasReasonPrefix(candidate.DiscoveryReasons, "intent_heading:implementation_plan") {
		t.Fatalf("expected implementation-plan heading reason, got %#v", candidate.DiscoveryReasons)
	}
}

func TestDiscover_ProposalFamilyDirectoryIndexes(t *testing.T) {
	tmp := t.TempDir()
	writeMarkdown(t, tmp, "beps/0013-ai-skills/README.md", strings.Join([]string{
		"---",
		"status: proposed",
		"---",
		"# AI Skills Proposal",
		"",
		"## Summary",
		"",
		"## Motivation",
		"",
		"## Proposal",
		"",
		"## Detailed Design",
		"",
		"## Drawbacks",
	}, "\n"))
	writeMarkdown(t, tmp, "enhancements/sig-node/2008-checkpointing/README.md", strings.Join([]string{
		"# Node Checkpointing",
		"",
		"## Summary",
		"",
		"## Motivation",
		"",
		"## Proposal",
		"",
		"## Unresolved Questions",
	}, "\n"))
	writeMarkdown(t, tmp, "docs/roadmaps/2026-platform.md", "# Platform Roadmap\n\n## Milestones\n\n## Timeline\n")
	writeMarkdown(t, tmp, "docs/release-notes/v1.md", "# Release Notes\n\n## Highlights\n")
	writeMarkdown(t, tmp, ".github/pull_request_template.md", "# Pull Request\n")
	writeMarkdown(t, tmp, "README.md", "# Project\n\n## Architecture\n")

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	if err != nil {
		t.Fatal(err)
	}
	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"beps/0013-ai-skills/README.md",
		"enhancements/sig-node/2008-checkpointing/README.md",
		"docs/roadmaps/2026-platform.md",
	} {
		if !stringSliceContains(got, want) {
			t.Fatalf("missing proposal/roadmap candidate %q in %v", want, got)
		}
	}
	for _, noisy := range []string{
		"docs/release-notes/v1.md",
		".github/pull_request_template.md",
		"README.md",
	} {
		if stringSliceContains(got, noisy) {
			t.Fatalf("broad discovery admitted noisy doc %q in %v", noisy, got)
		}
	}

	for _, rel := range []string{
		"beps/0013-ai-skills/README.md",
		"enhancements/sig-node/2008-checkpointing/README.md",
	} {
		candidate := findCandidate(candidates, rel)
		if candidate.DiscoveryScore < intentCandidateMinScore {
			t.Fatalf("%s discovery score = %.2f, want >= %.2f", rel, candidate.DiscoveryScore, intentCandidateMinScore)
		}
	}
	score, reasons := scoreIntentMarkdownCandidate(
		filepath.Join(tmp, filepath.FromSlash("beps/0013-ai-skills/README.md")),
		"beps/0013-ai-skills/README.md",
	)
	if score < intentCandidateMinScore {
		t.Fatalf("proposal-family score = %.2f, want >= %.2f", score, intentCandidateMinScore)
	}
	if !hasReasonPrefix(reasons, "intent_heading:proposal") {
		t.Fatalf("expected proposal heading reason, got %#v", reasons)
	}
}

func TestDiscover_ExperimentalIntentDiscoverySkipsNestedOpenSpecRoots(t *testing.T) {
	tmp := t.TempDir()
	writeMarkdown(t, tmp, "services/collector/openspec/changes/add-flow/proposal.md", "# Add Flow\n\n## Proposal\n")
	writeMarkdown(t, tmp, "services/collector/docs/plans/add-flow.md", "# Add Flow Plan\n\n## Implementation Plan\n")

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	if err != nil {
		t.Fatal(err)
	}
	got := candidateRelPaths(candidates)
	if stringSliceContains(got, "services/collector/openspec/changes/add-flow/proposal.md") {
		t.Fatalf("nested OpenSpec files should not be generic markdown candidates: %v", got)
	}
	if !stringSliceContains(got, "services/collector/docs/plans/add-flow.md") {
		t.Fatalf("expected nearby non-OpenSpec planning doc to remain discoverable: %v", got)
	}
}

func TestParse_FrontmatterOverrides(t *testing.T) {
	tmp := t.TempDir()
	content := "---\ntitle: Custom Title\nkind: spec\nstatus: draft\n---\n# Ignored H1\n\nBody here.\n"
	path := filepath.Join(tmp, "test.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, sources, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "test.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}

	if art.Title != "Custom Title" {
		t.Errorf("expected title 'Custom Title', got %q", art.Title)
	}
	if art.Kind != "spec" {
		t.Errorf("expected kind 'spec', got %q", art.Kind)
	}
	if art.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", art.Status)
	}
	if len(sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources))
	}
}

func TestParse_H1Fallback(t *testing.T) {
	tmp := t.TempDir()
	content := "# My Plan Title\n\nBody here.\n"
	path := filepath.Join(tmp, "test.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/test.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.Title != "My Plan Title" {
		t.Errorf("expected 'My Plan Title', got %q", art.Title)
	}
	if art.Kind != "plan" {
		t.Errorf("expected kind 'plan', got %q", art.Kind)
	}
}

func TestParse_ExtractsTodos(t *testing.T) {
	tmp := t.TempDir()
	content := "# Plan\n\n- [ ] First task\n- [x] Done task\n"
	path := filepath.Join(tmp, "plan.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	_, _, pr, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/plan.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	todos := pr.Todos
	if len(todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(todos))
	}
	if todos[0].Text != "First task" || todos[0].Done {
		t.Errorf("first todo wrong: %+v", todos[0])
	}
	if todos[1].Text != "Done task" || !todos[1].Done {
		t.Errorf("second todo wrong: %+v", todos[1])
	}
}

func TestAdapter_Name(t *testing.T) {
	a := &Adapter{}
	if a.Name() != "markdown" {
		t.Errorf("expected 'markdown', got %q", a.Name())
	}
}

func TestParse_FilenameFallback(t *testing.T) {
	tmp := t.TempDir()
	content := "No frontmatter and no H1 heading here.\n"
	path := filepath.Join(tmp, "my-cool-plan.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/my-cool-plan.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.Title != "My Cool Plan" {
		t.Errorf("expected 'My Cool Plan', got %q", art.Title)
	}
}

func TestParse_FileNotFound(t *testing.T) {
	a := &Adapter{}
	_, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: "/nonexistent/file.md",
		RelPath:     "file.md",
		AdapterName: "markdown",
	})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDiscover_SinglePathConfig(t *testing.T) {
	tmp := t.TempDir()
	customDir := filepath.Join(tmp, "single-dir")
	os.MkdirAll(customDir, 0o755)
	os.WriteFile(filepath.Join(customDir, "doc.md"), []byte("# Doc"), 0o644)

	cfg := &config.RepoConfig{
		Sources: []config.SourceConfig{
			{Type: "markdown", Path: "single-dir"},
		},
	}
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestDiscover_NonexistentPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.RepoConfig{
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{"does-not-exist"}},
		},
	}
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates for missing path, got %d", len(candidates))
	}
}

func TestParse_NoFrontmatterStatus(t *testing.T) {
	tmp := t.TempDir()
	content := "# Title Only\n\nContent without status.\n"
	path := filepath.Join(tmp, "test.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "docs/test.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.Status != "unknown" {
		t.Errorf("expected 'unknown' status, got %q", art.Status)
	}
}

func TestStripFrontmatter_UnclosedFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\nno closing marker\n"
	result := stripFrontmatter(content)
	if result != content {
		t.Errorf("unclosed frontmatter should return original, got %q", result)
	}
}

func TestFilenameTitle(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"plans/my-cool-plan.md", "My Cool Plan"},
		{"specs/api_design.md", "Api Design"},
		{"docs/README.md", "README"},
		{"plans/a.md", "A"},
	}
	for _, tt := range tests {
		got := filenameTitle(tt.path)
		if got != tt.want {
			t.Errorf("filenameTitle(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestInferKind(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"plans/refactor.md", "plan"},
		{"specs/api.md", "spec"},
		{"docs/rfcs/0007-auth-session.md", "design"},
		{"rfcs/session-token-handoff.md", "design"},
		{"token-boundary.rfc.md", "design"},
		{"beps/0013-ai-skills/README.md", "design"},
		{"enhancements/sig-node/2008-checkpointing/README.md", "design"},
		{"docs/proposals/search-index.md", "design"},
		{"design-docs/worker-runtime.md", "design"},
		{"docs/architecture/system-boundaries.md", "design"},
		{"docs/requirements/auth.md", "requirements"},
		{"notes/random.md", "markdown_artifact"},
		{"v0.prd.md", "requirements"},
		{"api.design.md", "design"},
		{"api.contract.md", "contract"},
		{"reqs.requirements.md", "requirements"},
		{".cursor/plans/foo.plan.md", "plan"},
	}
	for _, tt := range tests {
		got := inferKind(tt.path)
		if got != tt.want {
			t.Errorf("inferKind(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDefaultPaths_NarrowDocs(t *testing.T) {
	paths := defaultPaths()
	required := []string{
		".claude/notes", ".claude/plans", ".codex/plans", ".codex/notes",
		"docs/specs", "docs/plans", "docs/prd", "docs/rfcs", "rfcs",
		"roadmaps", "docs/roadmaps",
		"proposals", "docs/proposals", "enhancements", "docs/enhancements",
		"keps", "teps", "beps", "sips", "ships", "oseps",
		"docs/design", "docs/design-docs", "design-docs", "docs/technical",
		"architecture", "docs/architecture", "_bmad-output", ".specify/memory",
	}
	for _, req := range required {
		found := false
		for _, p := range paths {
			if p == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("defaultPaths() should include %q", req)
		}
	}
	for _, p := range paths {
		if p == "docs" {
			t.Error("defaultPaths() should not include bare top-level docs/ (use docs/specs, docs/plans, …)")
		}
	}
}

func TestDefaultRepoConfigMarkdownPathsMatchAdapterDefaults(t *testing.T) {
	cfg := config.DefaultRepoConfig()
	var cfgPaths []string
	for _, src := range cfg.Sources {
		if src.Type == "markdown" {
			cfgPaths = src.Paths
			break
		}
	}
	if !sameStrings(cfgPaths, defaultPaths()) {
		t.Fatalf("config.DefaultRepoConfig markdown paths drifted from adapter defaults\nconfig:  %#v\nadapter: %#v", cfgPaths, defaultPaths())
	}
}

func TestRootGlobs_AllPatterns(t *testing.T) {
	globs := rootGlobs()
	expected := []string{"*.spec.md", "*.plan.md", "*.prd.md", "*.rfc.md", "*.roadmap.md", "*.design.md", "*.contract.md", "*.requirements.md"}
	if len(globs) != len(expected) {
		t.Fatalf("expected %d root globs, got %d", len(expected), len(globs))
	}
	for i, g := range globs {
		if g != expected[i] {
			t.Errorf("rootGlobs[%d] = %q, want %q", i, g, expected[i])
		}
	}
}

func TestDiscover_RootGlobs(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "v0.prd.md"), []byte("# PRD"), 0o644)
	os.WriteFile(filepath.Join(tmp, "platform.roadmap.md"), []byte("# Platform Roadmap"), 0o644)
	os.WriteFile(filepath.Join(tmp, "api.design.md"), []byte("# Design"), 0o644)
	os.WriteFile(filepath.Join(tmp, "auth.contract.md"), []byte("# Contract"), 0o644)
	os.WriteFile(filepath.Join(tmp, "reqs.requirements.md"), []byte("# Reqs"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 5 {
		t.Fatalf("expected 5 root glob candidates, got %d", len(candidates))
	}
}

func TestDiscover_DocsDir(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide"), 0o644)

	a := &Adapter{}
	cfg := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{"docs"}},
		},
	}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate from configured docs/, got %d", len(candidates))
	}
	if candidates[0].RelPath != "docs/guide.md" {
		t.Errorf("expected 'docs/guide.md', got %q", candidates[0].RelPath)
	}
}

func TestParseFrontmatterTags_YAMLList(t *testing.T) {
	fm := map[string]string{"tags": "[auth, v2]"}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 2 || tags[0] != "auth" || tags[1] != "v2" {
		t.Errorf("expected [auth v2], got %v", tags)
	}
}

func TestParseFrontmatterTags_CommaSeparated(t *testing.T) {
	fm := map[string]string{"tags": "auth, v2"}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 2 || tags[0] != "auth" || tags[1] != "v2" {
		t.Errorf("expected [auth v2], got %v", tags)
	}
}

func TestParseFrontmatterTags_Labels(t *testing.T) {
	fm := map[string]string{"labels": "security"}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 1 || tags[0] != "security" {
		t.Errorf("expected [security], got %v", tags)
	}
}

func TestParseFrontmatterTags_Empty(t *testing.T) {
	fm := map[string]string{"tags": ""}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 0 {
		t.Errorf("expected empty, got %v", tags)
	}
}

func TestParseFrontmatterTags_NoKey(t *testing.T) {
	fm := map[string]string{"title": "Test"}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 0 {
		t.Errorf("expected empty, got %v", tags)
	}
}

func TestParseFrontmatterTags_Combined(t *testing.T) {
	fm := map[string]string{"tags": "[auth, v2]", "labels": "security, backend"}
	tags := parseFrontmatterTags(fm)
	if len(tags) != 4 {
		t.Errorf("expected 4 tags, got %v", tags)
	}
}

func TestParse_ExtractsTags(t *testing.T) {
	tmp := t.TempDir()
	content := "---\ntitle: Tagged Plan\ntags: [auth, v2]\nlabels: security\n---\n# Tagged Plan\n\nBody.\n"
	path := filepath.Join(tmp, "test.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/test.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(art.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %v", art.Tags)
	}
}

func TestParse_GeneratorFrontmatterSetsProfileWithoutToolTag(t *testing.T) {
	tmp := t.TempDir()
	content := "---\ngenerator: Claude Desktop\n---\n# Doc Title\n\nBody.\n"
	path := filepath.Join(tmp, "x.md")
	os.WriteFile(path, []byte(content), 0o644)

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/x.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stringSliceContains(art.Tags, "claude-desktop") {
		t.Fatalf("did not expect generator slug as tag, got %#v", art.Tags)
	}
	if art.FormatProfile != format.ProfileClaude {
		t.Fatalf("format_profile: want %q, got %q", format.ProfileClaude, art.FormatProfile)
	}
	if g, _ := art.Extracted["generator"].(string); g != "Claude Desktop" {
		t.Fatalf("extracted generator: want Claude Desktop, got %q", g)
	}
}

func testSamplesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "samples"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPathGeneratorForExtract(t *testing.T) {
	tests := []struct {
		relPath string
		wantGen string
	}{
		{"_bmad-output/planning-artifacts/prd.md", "bmad-method"},
		{"specs/001-x/spec.md", "speckit"},
		{"specs/001-x/plan.md", "speckit"},
		{".cursor/plans/foo.plan.md", "cursor-plan"},
		{".codex/plans/PLAN.md", "codex"},
		{"plans/nested/spec.md", ""},
	}
	for _, tt := range tests {
		gen := pathGeneratorForExtract(tt.relPath)
		if gen != tt.wantGen {
			t.Errorf("%q: generator got %q want %q", tt.relPath, gen, tt.wantGen)
		}
	}
}

func TestDiscover_SampleFixture_BMAD(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "bmad")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 2 {
		t.Fatalf("bmad fixture: want >= 2 markdown candidates, got %d", len(candidates))
	}
	var prdCand adapters.Candidate
	for _, c := range candidates {
		if strings.HasSuffix(strings.ToLower(c.RelPath), "planning-artifacts/prd.md") {
			prdCand = c
			break
		}
	}
	if prdCand.PrimaryPath == "" {
		t.Fatal("prd.md not discovered")
	}
	art, _, _, err := a.Parse(context.Background(), prdCand)
	if err != nil {
		t.Fatal(err)
	}
	if art.FormatProfile != format.ProfileBmad {
		t.Fatalf("expected format_profile bmad, got %q", art.FormatProfile)
	}
	if g, _ := art.Extracted["generator"].(string); g != "bmad-method" {
		t.Fatalf("extracted generator: want bmad-method, got %q", g)
	}
}

func TestDiscover_SampleFixture_Specify(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "specify")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) < 8 {
		t.Fatalf("specify fixture: want >= 8 markdown candidates, got %d", len(candidates))
	}
	var specCand adapters.Candidate
	wantRel := filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs", "spec.md"))
	for _, c := range candidates {
		if filepath.ToSlash(c.RelPath) == wantRel {
			specCand = c
			break
		}
	}
	if specCand.PrimaryPath == "" {
		t.Fatal("spec.md not discovered under specs/001-discover-related-specs/")
	}
	art, _, _, err := a.Parse(context.Background(), specCand)
	if err != nil {
		t.Fatal(err)
	}
	wantLayout := filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs"))
	if art.FormatProfile != format.ProfileSpeckit {
		t.Fatalf("expected format_profile speckit, got %q", art.FormatProfile)
	}
	if art.LayoutGroup != wantLayout {
		t.Fatalf("layout_group: want %q, got %q", wantLayout, art.LayoutGroup)
	}
	if g, _ := art.Extracted["generator"].(string); g != "speckit" {
		t.Fatalf("extracted generator: want speckit, got %q", g)
	}
}

func TestDiscover_SampleFixture_SpecifyChildrenShareLayout(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "specify")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantLayout := filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs"))
	for _, rel := range []string{
		filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs", "plan.md")),
		filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs", "tasks.md")),
		filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs", "research.md")),
	} {
		candidate := findCandidate(candidates, rel)
		if candidate.PrimaryPath == "" {
			t.Fatalf("missing Spec Kit child %s", rel)
		}
		art, _, _, err := a.Parse(context.Background(), candidate)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		if art.FormatProfile != format.ProfileSpeckit {
			t.Fatalf("%s format_profile: want %q got %q", rel, format.ProfileSpeckit, art.FormatProfile)
		}
		if art.LayoutGroup != wantLayout {
			t.Fatalf("%s layout_group: want %q got %q", rel, wantLayout, art.LayoutGroup)
		}
	}
}

func TestDiscover_SampleFixture_SpecifyTasksTodos(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "specify")
	wantRel := filepath.ToSlash(filepath.Join("specs", "001-discover-related-specs", "tasks.md"))
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tasksCand adapters.Candidate
	for _, c := range candidates {
		if filepath.ToSlash(c.RelPath) == wantRel {
			tasksCand = c
			break
		}
	}
	if tasksCand.PrimaryPath == "" {
		t.Fatal("tasks.md not discovered")
	}
	_, _, pr, err := a.Parse(context.Background(), tasksCand)
	if err != nil {
		t.Fatal(err)
	}
	todos := pr.Todos
	if len(todos) < 8 {
		t.Fatalf("specify tasks fixture: want >= 8 checklist todos, got %d", len(todos))
	}
}

func TestDiscover_SampleFixture_CursorPlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "cursor")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("cursor fixture: want 1 candidate, got %d (%v)", len(candidates), candidates)
	}
	if want := ".cursor/plans/probabilistic_related_specs_481c4b3f.plan.md"; filepath.ToSlash(candidates[0].RelPath) != want {
		t.Fatalf("rel path: want %s, got %s", want, candidates[0].RelPath)
	}
	art, _, _, err := a.Parse(context.Background(), candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if art.FormatProfile != format.ProfileCursorPlan {
		t.Fatalf("expected format_profile cursor_plan, got %q", art.FormatProfile)
	}
}

func TestDiscover_SampleFixture_CodexPlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "codex")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || filepath.ToSlash(candidates[0].RelPath) != "plans/PLAN.md" {
		t.Fatalf("codex fixture: want plans/PLAN.md, got %#v", candidates)
	}
}

func TestDiscover_SampleFixture_ClaudePlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "claude")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || filepath.ToSlash(candidates[0].RelPath) != "plans/dreamy-orbiting-quokka.md" {
		t.Fatalf("claude fixture: want plans/dreamy-orbiting-quokka.md, got %#v", candidates)
	}
}

func TestDiscover_SampleFixture_Freetext(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "freetext")
	cfgRules := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{
				Type:  "markdown",
				Paths: []string{".", "v2/plans", "decisions"},
				Rules: []config.SourceRule{
					{Match: "ROADMAP.md", Kind: config.KindPlan},
					{Match: "*/README.md", Kind: config.KindPlan},
					{Match: "README.md", Kind: config.KindPlan},
					{Match: "[0-9][0-9]_*.md", Kind: config.KindPlan},
					{Match: "*/[0-9][0-9]-*.md", Kind: config.KindPlan},
					{Match: "decisions/*.md", Kind: config.KindDecision},
				},
			},
		},
	}
	if err := config.ValidateRepoConfig(cfgRules); err != nil {
		t.Fatal(err)
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, cfgRules)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 20 {
		t.Fatalf("freetext fixture: want 20 markdown candidates, got %d", len(candidates))
	}

	findCand := func(rel string) adapters.Candidate {
		for _, c := range candidates {
			if filepath.ToSlash(c.RelPath) == rel {
				return c
			}
		}
		t.Fatalf("missing candidate %s", rel)
		return adapters.Candidate{}
	}

	for _, tc := range []struct {
		rel  string
		kind string
		sub  string
	}{
		{"ROADMAP.md", config.KindPlan, ""},
		{"v2/plans/README.md", config.KindPlan, ""},
		{"v2/plans/01-ui-scraping-high-fidelity-collection/README.md", config.KindPlan, ""},
		{"v2/plans/02_PROMPT_GROUPING.md", config.KindPlan, ""},
		{"v2/plans/01-ui-scraping-high-fidelity-collection/03-browserbase-chatgpt-spike.md", config.KindPlan, ""},
		{"decisions/001-scraping-approach.md", config.KindDecision, ""},
	} {
		art, _, _, err := a.Parse(context.Background(), findCand(tc.rel))
		if err != nil {
			t.Fatalf("parse %s: %v", tc.rel, err)
		}
		if art.Kind != tc.kind || art.Subtype != tc.sub {
			t.Errorf("%s: want kind=%s subtype=%s got kind=%s subtype=%s", tc.rel, tc.kind, tc.sub, art.Kind, art.Subtype)
		}
	}

	cfgPathsOnly := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{".", "v2/plans", "decisions"}},
		},
	}
	candidatesNoRules, err := a.Discover(context.Background(), root, cfgPathsOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidatesNoRules) != 20 {
		t.Fatalf("paths-only discover: want 20 candidates, got %d", len(candidatesNoRules))
	}
	findNR := func(rel string) adapters.Candidate {
		for _, c := range candidatesNoRules {
			if filepath.ToSlash(c.RelPath) == rel {
				return c
			}
		}
		t.Fatalf("missing candidate %s", rel)
		return adapters.Candidate{}
	}
	art, _, _, err := a.Parse(context.Background(), findNR("marketing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if art.Kind != config.KindMarkdownArtifact {
		t.Errorf("marketing.md without rules: want kind markdown_artifact, got %q", art.Kind)
	}
}

func stringSliceContains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func candidateRelPaths(candidates []adapters.Candidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = filepath.ToSlash(c.RelPath)
	}
	return out
}

func writeMarkdown(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findCandidate(candidates []adapters.Candidate, rel string) adapters.Candidate {
	rel = filepath.ToSlash(rel)
	for _, candidate := range candidates {
		if filepath.ToSlash(candidate.RelPath) == rel {
			return candidate
		}
	}
	return adapters.Candidate{}
}

func hasReasonPrefix(reasons []string, prefix string) bool {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, prefix) {
			return true
		}
	}
	return false
}

func TestDiscover_IgnoredSubtreeExcluded(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("vendor-plans/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pdir := filepath.Join(tmp, "plans")
	os.MkdirAll(pdir, 0o755)
	os.WriteFile(filepath.Join(pdir, "a.md"), []byte("# A\n"), 0o644)
	vdir := filepath.Join(tmp, "vendor-plans")
	os.MkdirAll(vdir, 0o755)
	os.WriteFile(filepath.Join(vdir, "b.md"), []byte("# B\n"), 0o644)

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := ignore.WithContext(context.Background(), m)
	a := &Adapter{}
	cands, err := a.Discover(ctx, tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if strings.HasPrefix(c.RelPath, "vendor-plans/") {
			t.Fatalf("got ignored path %q", c.RelPath)
		}
	}
}

func TestInferDirectoryTag(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"plans/auth/middleware.plan.md", "auth"},
		{"plans/billing.md", ""},
		{"specs/api.md", ""},
		{"docs/auth/login.md", "auth"},
		{".cursor/plans/foo.md", ""},
		{"plans/v2/migration.md", "v2"},
		{"random.md", ""},
		{"_bmad-output/planning-artifacts/prd.md", ""},
		{"specs/001-feature/foo/spec.md", "foo"},
	}
	for _, tt := range tests {
		got := InferDirectoryTag(tt.path)
		if got != tt.want {
			t.Errorf("InferDirectoryTag(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
