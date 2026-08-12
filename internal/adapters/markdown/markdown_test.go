package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscover_DefaultPaths(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "refactor.md"), []byte("# Refactor\n"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	require.Len(t, candidates, 1,
		"expected 1 candidate, got %d", len(candidates))
	assert.Equal(t, "plans/refactor.md", candidates[0].RelPath,
		"expected rel path 'plans/refactor.md', got %q", candidates[0].RelPath)

}

func TestDiscover_RootStandardIntentDocs(t *testing.T) {
	tmp := t.TempDir()
	for _, rel := range []string{"ROADMAP.md", "PLAN.md", "DESIGN.md", "ARCHITECTURE.md", "README.md"} {
		writeMarkdown(t, tmp, rel, "# "+strings.TrimSuffix(rel, ".md")+"\n\n- [ ] follow-up\n")
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	for _, rel := range []string{"ROADMAP.md", "PLAN.md", "DESIGN.md", "ARCHITECTURE.md"} {
		require.NotEqual(t, "", findCandidate(candidates, rel).RelPath,
			"missing root intent doc %s in %#v", rel, candidateRelPaths(candidates))

	}
	require.Equal(t, "", findCandidate(candidates, "README.md").RelPath,
		"root README should not be included by standard intent globs: %#v", candidateRelPaths(candidates))

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
	require.NoError(t, err)

	require.Len(t, candidates, 1,
		"expected 1 candidate, got %d", len(candidates))

}

func TestDiscover_DefaultNestedDocsIntentDirs(t *testing.T) {
	tmp := t.TempDir()
	nestedPlan := filepath.Join(tmp, "apps", "desktop", "docs", "plans")
	nestedPRD := filepath.Join(tmp, "services", "api", "docs", "prd")
	nestedRFC := filepath.Join(tmp, "packages", "api", "docs", "rfcs")
	nestedArchitecture := filepath.Join(tmp, "platform", "docs", "architecture")
	nestedDesignDocs := filepath.Join(tmp, "runtime", "docs", "design-docs")

	require.NoError(t, os.MkdirAll(nestedPlan, 0o755))

	require.NoError(t, os.MkdirAll(nestedPRD, 0o755))

	require.NoError(t, os.MkdirAll(nestedRFC, 0o755))

	require.NoError(t, os.MkdirAll(nestedArchitecture, 0o755))

	require.NoError(t, os.MkdirAll(nestedDesignDocs, 0o755))

	os.WriteFile(filepath.Join(nestedPlan, "pnpm-migration.md"), []byte("# PNPM Migration\n"), 0o644)
	os.WriteFile(filepath.Join(nestedPRD, "billing.md"), []byte("# Billing PRD\n"), 0o644)
	os.WriteFile(filepath.Join(nestedRFC, "token-boundary.md"), []byte("# Token Boundary RFC\n"), 0o644)
	os.WriteFile(filepath.Join(nestedArchitecture, "system-boundaries.md"), []byte("# System Boundaries Architecture\n"), 0o644)
	os.WriteFile(filepath.Join(nestedDesignDocs, "worker-runtime.md"), []byte("# Worker Runtime Design\n"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"apps/desktop/docs/plans/pnpm-migration.md",
		"services/api/docs/prd/billing.md",
		"packages/api/docs/rfcs/token-boundary.md",
		"platform/docs/architecture/system-boundaries.md",
		"runtime/docs/design-docs/worker-runtime.md",
	} {
		require.True(t, stringSliceContains(got, want),
			"missing nested default intent doc %q in %v", want, got)

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
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	require.True(t, stringSliceContains(got, "my-plans/plan.md"),
		"missing configured markdown candidate in %v", got)
	require.False(t, stringSliceContains(got, "apps/desktop/docs/plans/hidden.md"),
		"custom config should not add nested defaults, got %v", got)

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
	require.NoError(t, err)

	baselinePaths := candidateRelPaths(baseline)
	require.False(t, stringSliceContains(baselinePaths, "docs/exec-plans/active/cache-warmup.md"),
		"baseline unexpectedly discovered exec-plans path: %v", baselinePaths)

	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"docs/exec-plans/active/cache-warmup.md",
		"docs/designDocs/auth-boundary.md",
		"docs/project_planning/migration.md",
		".github/AGENTS.md",
	} {
		require.True(t, stringSliceContains(got, want),
			"missing experimental intent candidate %q in %v", want, got)

	}
	for _, noisy := range []string{
		"README.md",
		"CHANGELOG.md",
		".github/pull_request_template.md",
		"examples/agent/browser_agent/build_in_prompt/browser_agent_task_decomposition_prompt.md",
	} {
		require.False(t, stringSliceContains(got, noisy),
			"experimental discovery should not admit noisy maintenance doc %q in %v", noisy, got)

	}

	candidate := findCandidate(candidates, "docs/exec-plans/active/cache-warmup.md")
	require.GreaterOrEqual(t, candidate.DiscoveryScore, intentCandidateMinScore,
		"discovery score = %.2f, want >= %.2f", candidate.DiscoveryScore, intentCandidateMinScore)
	require.True(t, hasReasonPrefix(candidate.DiscoveryReasons, "intent_path_token:plan"),
		"expected plan path-token reason, got %#v", candidate.DiscoveryReasons)
	require.True(t, hasReasonPrefix(candidate.DiscoveryReasons, "intent_heading:implementation_plan"),
		"expected implementation-plan heading reason, got %#v", candidate.DiscoveryReasons)

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
	writeMarkdown(t, tmp, "docs/proposals/search-index.md", "# Search Index Proposal\n\n## Summary\n\n## Motivation\n\n## Proposal\n")
	writeMarkdown(t, tmp, "docs/roadmaps/2026-platform.md", "# Platform Roadmap\n\n## Milestones\n\n## Timeline\n")
	writeMarkdown(t, tmp, "beps/docs/proposals/BEP-001-exceptions/legacy-ignore/context/go.md", "# Go Error Handling Survey\n")
	writeMarkdown(t, tmp, "library/methodologies/bmad-method/skills/architecture-design/SKILL.md", "# Architecture Design Skill\n")
	writeMarkdown(t, tmp, "docs/release-notes/v1.md", "# Release Notes\n\n## Highlights\n")
	writeMarkdown(t, tmp, ".github/pull_request_template.md", "# Pull Request\n")
	writeMarkdown(t, tmp, "README.md", "# Project\n\n## Architecture\n")

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"beps/0013-ai-skills/README.md",
		"enhancements/sig-node/2008-checkpointing/README.md",
		"docs/proposals/search-index.md",
		"docs/roadmaps/2026-platform.md",
	} {
		require.True(t, stringSliceContains(got, want),
			"missing proposal/roadmap candidate %q in %v", want, got)

	}
	for _, noisy := range []string{
		"docs/release-notes/v1.md",
		"beps/docs/proposals/BEP-001-exceptions/legacy-ignore/context/go.md",
		"library/methodologies/bmad-method/skills/architecture-design/SKILL.md",
		".github/pull_request_template.md",
		"README.md",
	} {
		require.False(t, stringSliceContains(got, noisy),
			"broad discovery admitted noisy doc %q in %v", noisy, got)

	}

	for _, rel := range []string{
		"beps/0013-ai-skills/README.md",
		"enhancements/sig-node/2008-checkpointing/README.md",
	} {
		candidate := findCandidate(candidates, rel)
		require.GreaterOrEqual(t, candidate.DiscoveryScore, intentCandidateMinScore,
			"%s discovery score = %.2f, want >= %.2f", rel, candidate.DiscoveryScore, intentCandidateMinScore)

	}
	score, reasons := scoreIntentMarkdownCandidate(
		filepath.Join(tmp, filepath.FromSlash("beps/0013-ai-skills/README.md")),
		"beps/0013-ai-skills/README.md",
	)
	require.GreaterOrEqual(t, score, intentCandidateMinScore,
		"proposal-family score = %.2f, want >= %.2f", score, intentCandidateMinScore)
	require.True(t, hasReasonPrefix(reasons, "intent_heading:proposal"),
		"expected proposal heading reason, got %#v", reasons)

}

func TestDiscover_SupportDocDiscoveryFindsBoundedDocs(t *testing.T) {
	tmp := t.TempDir()
	for rel, body := range map[string]string{
		"docs/security/how-to-authenticate.md":            "# How to Authenticate\n\n## Authentication\n\nUse local auth for offline deployments.\n",
		"docs/security/access-control.md":                 "# Access Control\n\n## RBAC\n\nRole permissions and authorization.\n",
		"docs/backend-system/core-services/metrics.md":    "# Metrics Service\n\n## Metrics\n\nOpenTelemetry naming conventions.\n",
		"docs/docs/ansible.md":                            "# Ansible\n\n## Playbooks\n\nAWX backed playbook execution.\n",
		"docs/src/instance_manager.md":                    "# Instance Manager\n\n## Failover\n\nOperator pod management internals.\n",
		"docs/tutorials/getting-started.md":               "# Getting Started\n\n## Tutorial\n\nA generic tutorial.\n",
		"docs/examples/sample-template.md":                "# Sample Template\n\n## Example\n\nA generated-looking sample.\n",
		"CHANGELOG.md":                                    "# Changelog\n",
		"README.md":                                       "# Project\n\n## Architecture\n",
		"examples/agent/browser_agent/prompt/README.md":   "# Browser Agent Prompt\n",
		"library/methodologies/skills/security/SKILL.md":  "# Security Skill\n",
		"docs/release-notes/security-release-notes.md":    "# Security Release Notes\n",
		"docs/generated/observability-fixture-example.md": "# Observability Fixture Example\n",
		"docs/reference/generated-metrics-template.md":    "# Metrics Template\n",
		"docs/reference/security-news.md":                 "# Security News\n",
		"docs/reference/authorization-sample.md":          "# Authorization Sample\n",
		"docs/reference/authentication-fixture.md":        "# Authentication Fixture\n",
		"docs/reference/operator-tutorial.md":             "# Operator Tutorial\n",
		"docs/reference/telemetry-example.md":             "# Telemetry Example\n",
		"docs/reference/access-control-template.md":       "# Access Control Template\n",
		"docs/reference/observability-prompt.md":          "# Observability Prompt\n",
		"docs/reference/security-changelog.md":            "# Security Changelog\n",
		"docs/reference/metrics-release.md":               "# Metrics Release\n",
		"docs/reference/auth-license.md":                  "# Auth License\n",
		"docs/reference/playbook-sample.md":               "# Playbook Sample\n",
		"docs/reference/ansible-example.md":               "# Ansible Example\n",
		"docs/reference/statefulset-tutorial.md":          "# StatefulSet Tutorial\n",
		"docs/reference/failover-template.md":             "# Failover Template\n",
		"docs/reference/logging-fixture.md":               "# Logging Fixture\n",
		"docs/reference/rbac-prompt.md":                   "# RBAC Prompt\n",
	} {
		writeMarkdown(t, tmp, rel, body)
	}

	a := &Adapter{}
	withoutSupport, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	require.NoError(t, err)

	require.False(t, stringSliceContains(candidateRelPaths(withoutSupport), "docs/security/access-control.md"),
		"support doc discovery should require explicit support-doc experiment")

	candidates, err := a.Discover(context.Background(), tmp, config.WithSupportDocDiscovery(config.WithIntentCandidateDiscovery(nil, true), true))
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"docs/security/how-to-authenticate.md",
		"docs/security/access-control.md",
		"docs/backend-system/core-services/metrics.md",
		"docs/docs/ansible.md",
		"docs/src/instance_manager.md",
	} {
		require.True(t, stringSliceContains(got, want),
			"missing support doc %q in %v", want, got)

		candidate := findCandidate(candidates, want)
		require.True(t, hasReasonPrefix(candidate.DiscoveryReasons, "support_"),
			"%s missing support discovery reason: %#v", want, candidate.DiscoveryReasons)

	}
	for _, noisy := range []string{
		"docs/tutorials/getting-started.md",
		"docs/examples/sample-template.md",
		"CHANGELOG.md",
		"README.md",
		"examples/agent/browser_agent/prompt/README.md",
		"library/methodologies/skills/security/SKILL.md",
		"docs/release-notes/security-release-notes.md",
	} {
		require.False(t, stringSliceContains(got, noisy),
			"support-doc discovery admitted noisy doc %q in %v", noisy, got)

	}
}

func TestDiscover_SupportDocDiscoveryIsCapped(t *testing.T) {
	tmp := t.TempDir()
	for i := 0; i < supportDocMaxFiles+15; i++ {
		writeMarkdown(t, tmp, fmt.Sprintf("docs/security/auth-%03d.md", i), "# Authentication\n\n## Access Control\n")
	}

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, config.WithSupportDocDiscovery(config.WithIntentCandidateDiscovery(nil, true), true))
	require.NoError(t, err)

	var supportCount int
	for _, candidate := range candidates {
		if strings.HasPrefix(filepath.ToSlash(candidate.RelPath), "docs/security/auth-") {
			supportCount++
		}
	}
	require.Equal(t, supportDocMaxFiles, supportCount,
		"support candidates = %d, want cap %d", supportCount, supportDocMaxFiles)
	require.NotEqual(t, "", findCandidate(candidates, "docs/security/auth-000.md").RelPath,
		"expected deterministic low path to survive support-doc cap")
	require.Equal(t, "", findCandidate(candidates, fmt.Sprintf("docs/security/auth-%03d.md", supportDocMaxFiles+14)).RelPath,
		"expected path beyond support-doc cap to be excluded")

}

func TestDiscover_ExperimentalIntentDiscoverySkipsNestedOpenSpecRoots(t *testing.T) {
	tmp := t.TempDir()
	writeMarkdown(t, tmp, "services/collector/openspec/changes/add-flow/proposal.md", "# Add Flow\n\n## Proposal\n")
	writeMarkdown(t, tmp, "services/collector/docs/plans/add-flow.md", "# Add Flow Plan\n\n## Implementation Plan\n")

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, config.WithIntentCandidateDiscovery(nil, true))
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	require.False(t, stringSliceContains(got, "services/collector/openspec/changes/add-flow/proposal.md"),
		"nested OpenSpec files should not be generic markdown candidates: %v", got)
	require.True(t, stringSliceContains(got, "services/collector/docs/plans/add-flow.md"),
		"expected nearby non-OpenSpec planning doc to remain discoverable: %v", got)

}

func TestDiscover_WithConventionalADRPath_LeavesFileToADRAdapter(t *testing.T) {
	repoRoot := t.TempDir()
	adrPath := filepath.Join(repoRoot, "docs", "adr", "0001-use-postgresql.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(adrPath), 0o755))
	require.NoError(t, os.WriteFile(adrPath, []byte("# ADR-0001: Use PostgreSQL\n\n## Status\nAccepted\n"), 0o644))

	candidates, err := (&Adapter{}).Discover(context.Background(), repoRoot, config.DefaultRepoConfig())

	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestDiscover_WithConfiguredADRPath_LeavesFileToADRAdapter(t *testing.T) {
	repoRoot := t.TempDir()
	adrPath := filepath.Join(repoRoot, "docs", "decisions", "0001-use-postgresql.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(adrPath), 0o755))
	require.NoError(t, os.WriteFile(adrPath, []byte("# ADR-0001: Use PostgreSQL\n\n## Status\nAccepted\n"), 0o644))
	cfg := config.DefaultRepoConfig()
	cfg.Sources[1].Paths = []string{"docs/decisions"}

	candidates, err := (&Adapter{}).Discover(context.Background(), repoRoot, cfg)

	require.NoError(t, err)
	assert.Empty(t, candidates)
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
	require.NoError(t, err)

	assert.Equal(t, "Custom Title", art.Title,
		"expected title 'Custom Title', got %q", art.Title)
	assert.Equal(t, "spec", art.Kind,
		"expected kind 'spec', got %q", art.Kind)
	assert.Equal(t, "draft", art.Status,
		"expected status 'draft', got %q", art.Status)
	assert.Len(t, sources, 1,
		"expected 1 source, got %d", len(sources))

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
	require.NoError(t, err)

	assert.Equal(t, "My Plan Title", art.Title,
		"expected 'My Plan Title', got %q", art.Title)
	assert.Equal(t, "plan", art.Kind,
		"expected kind 'plan', got %q", art.Kind)

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
	require.NoError(t, err)

	todos := pr.Todos
	require.Len(t, todos, 2,
		"expected 2 todos, got %d", len(todos))
	assert.Equal(t, "First task", todos[0].Text)
	assert.False(t, todos[0].Done)
	assert.Equal(t, "Done task", todos[1].Text)
	assert.True(t, todos[1].Done)

}

func TestAdapter_Name(t *testing.T) {
	a := &Adapter{}
	assert.Equal(t, "markdown", a.Name(),
		"expected 'markdown', got %q", a.Name())

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
	require.NoError(t, err)

	assert.Equal(t, "My Cool Plan", art.Title,
		"expected 'My Cool Plan', got %q", art.Title)

}

func TestParse_FileNotFound(t *testing.T) {
	a := &Adapter{}
	_, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: "/nonexistent/file.md",
		RelPath:     "file.md",
		AdapterName: "markdown",
	})
	assert.Error(t, err,
		"expected error for missing file")

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
	require.NoError(t, err)

	require.Len(t, candidates, 1,
		"expected 1 candidate, got %d", len(candidates))

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
	require.NoError(t, err)

	assert.Empty(t, candidates,
		"expected 0 candidates for missing path, got %d", len(candidates))

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
	require.NoError(t, err)

	assert.Equal(t, "unknown", art.Status,
		"expected 'unknown' status, got %q", art.Status)

}

func TestStripFrontmatter_UnclosedFrontmatter(t *testing.T) {
	content := "---\ntitle: Test\nno closing marker\n"
	result := stripFrontmatter(content)
	assert.Equal(t, content, result,
		"unclosed frontmatter should return original, got %q", result)

}

func TestFilenameTitle_WithHyphenatedPlan_ReturnsHumanTitle(t *testing.T) {
	actual := filenameTitle("plans/my-cool-plan.md")

	assert.Equal(t, "My Cool Plan", actual)
}

func TestFilenameTitle_WithUnderscoredSpec_ReturnsHumanTitle(t *testing.T) {
	actual := filenameTitle("specs/api_design.md")

	assert.Equal(t, "Api Design", actual)
}

func TestFilenameTitle_WithReadme_PreservesUppercaseName(t *testing.T) {
	actual := filenameTitle("docs/README.md")

	assert.Equal(t, "README", actual)
}

func TestFilenameTitle_WithSingleLetter_ReturnsUppercaseLetter(t *testing.T) {
	actual := filenameTitle("plans/a.md")

	assert.Equal(t, "A", actual)
}

func TestInferKind_WithPlanDirectory_ReturnsPlan(t *testing.T) {
	actual := inferKind("plans/refactor.md")

	assert.Equal(t, config.KindPlan, actual)
}

func TestInferKind_WithSpecDirectory_ReturnsSpec(t *testing.T) {
	actual := inferKind("specs/api.md")

	assert.Equal(t, config.KindSpec, actual)
}

func TestInferKind_WithDocsRFCPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("docs/rfcs/0007-auth-session.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithRootRFCPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("rfcs/session-token-handoff.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithRFCSuffix_ReturnsDesign(t *testing.T) {
	actual := inferKind("token-boundary.rfc.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithBEPPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("beps/0013-ai-skills/README.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithEnhancementPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("enhancements/sig-node/2008-checkpointing/README.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithProposalPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("docs/proposals/search-index.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithDesignDocsPath_ReturnsDesign(t *testing.T) {
	actual := inferKind("design-docs/worker-runtime.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithArchitecturePath_ReturnsDesign(t *testing.T) {
	actual := inferKind("docs/architecture/system-boundaries.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithRequirementsPath_ReturnsRequirements(t *testing.T) {
	actual := inferKind("docs/requirements/auth.md")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferKind_WithProductSpecsPath_ReturnsRequirements(t *testing.T) {
	actual := inferKind("docs/product-specs/course-visit-analytics.md")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferKind_WithCALMRequirementPath_ReturnsRequirements(t *testing.T) {
	actual := inferKind("calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferKind_WithCodexSkill_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind(".codex/skills/query-plan-snapshot-cli/SKILL.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithCursorCommand_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind(".cursor/commands/ds-task.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithWindsurfWorkflow_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind(".windsurf/workflows/ds-apply.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithAgentDefinition_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind("agents/implementation-plan.agent.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithProposalTemplate_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind("contributingGuides/PROPOSAL_TEMPLATE.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithGovernanceFile_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind("GOVERNANCE.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithMaintainersFile_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind("MAINTAINERS.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithRandomNote_ReturnsMarkdownArtifact(t *testing.T) {
	actual := inferKind("notes/random.md")

	assert.Equal(t, config.KindMarkdownArtifact, actual)
}

func TestInferKind_WithPRDSuffix_ReturnsRequirements(t *testing.T) {
	actual := inferKind("v0.prd.md")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferKind_WithDesignSuffix_ReturnsDesign(t *testing.T) {
	actual := inferKind("api.design.md")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferKind_WithContractSuffix_ReturnsContract(t *testing.T) {
	actual := inferKind("api.contract.md")

	assert.Equal(t, config.KindContract, actual)
}

func TestInferKind_WithRequirementsSuffix_ReturnsRequirements(t *testing.T) {
	actual := inferKind("reqs.requirements.md")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferKind_WithPlanSuffix_ReturnsPlan(t *testing.T) {
	actual := inferKind(".cursor/plans/foo.plan.md")

	assert.Equal(t, config.KindPlan, actual)
}

func TestInferKindSubtype_WithCursorCommand_ReturnsAgentInstruction(t *testing.T) {
	kind, subtype := inferKindSubtype(".cursor/commands/ds-task.md")

	assert.Equal(t, config.KindMarkdownArtifact, kind)
	assert.Equal(t, config.SubtypeAgentInstruction, subtype)
}

func TestInferKindSubtype_WithWindsurfWorkflow_ReturnsAgentInstruction(t *testing.T) {
	kind, subtype := inferKindSubtype(".windsurf/workflows/ds-apply.md")

	assert.Equal(t, config.KindMarkdownArtifact, kind)
	assert.Equal(t, config.SubtypeAgentInstruction, subtype)
}

func TestDefaultPaths_NarrowDocs(t *testing.T) {
	paths := defaultPaths()
	required := []string{
		".claude/notes", ".claude/plans", ".codex/plans", ".codex/notes",
		".agents/skills", ".claude/skills", ".codex/skills", ".cursor/commands", ".windsurf/workflows", "agents",
		"docs/specs", "docs/plans", "docs/prd", "docs/product-specs", "docs/requirements", "docs/rfcs", "docs/RFCS", "rfcs", "RFCS",
		"roadmaps", "docs/roadmaps",
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
		assert.True(t, found,
			"defaultPaths() should include %q", req)

	}
	for _, p := range paths {
		assert.NotEqual(t, "docs", p,
			"defaultPaths() should not include bare top-level docs/ (use docs/specs, docs/plans, …)")

		for _, broadProposalRoot := range []string{"proposals", "docs/proposals", "enhancements", "docs/enhancements", "beps", "docs/beps"} {
			assert.NotEqual(t, broadProposalRoot, p,
				"defaultPaths() should not recursively include broad proposal root %q; use scored discovery", p)

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
	require.True(t, sameStrings(cfgPaths, defaultPaths()),
		"config.DefaultRepoConfig markdown paths drifted from adapter defaults\nconfig:  %#v\nadapter: %#v", cfgPaths, defaultPaths())

}

func TestRootGlobs_AllPatterns(t *testing.T) {
	globs := rootGlobs()
	expected := []string{
		"*.spec.md", "*.plan.md", "*.prd.md", "*.rfc.md", "*.roadmap.md", "*.design.md", "*.contract.md", "*.requirements.md", "REQ_*.md", "REQ-*.md", "*_REQ.md", "*-REQ.md",
		"*.spec.mdx", "*.plan.mdx", "*.prd.mdx", "*.rfc.mdx", "*.roadmap.mdx", "*.design.mdx", "*.contract.mdx", "*.requirements.mdx", "REQ_*.mdx", "REQ-*.mdx", "*_REQ.mdx", "*-REQ.mdx",
	}
	require.Len(t, globs, len(expected),
		"expected %d root globs, got %d", len(expected), len(globs))

	for i, g := range globs {
		assert.Equal(t, expected[i], g,
			"rootGlobs[%d] = %q, want %q", i, g, expected[i])

	}
}

func TestDiscover_RootGlobs(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "v0.prd.md"), []byte("# PRD"), 0o644)
	os.WriteFile(filepath.Join(tmp, "platform.roadmap.md"), []byte("# Platform Roadmap"), 0o644)
	os.WriteFile(filepath.Join(tmp, "api.design.md"), []byte("# Design"), 0o644)
	os.WriteFile(filepath.Join(tmp, "auth.contract.md"), []byte("# Contract"), 0o644)
	os.WriteFile(filepath.Join(tmp, "reqs.requirements.md"), []byte("# Reqs"), 0o644)
	os.WriteFile(filepath.Join(tmp, "sdk.plan.mdx"), []byte("# SDK Plan"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	require.Len(t, candidates, 6,
		"expected 6 root glob candidates, got %d", len(candidates))

}

func TestDiscover_DefaultHighSignalMarkdownFiles(t *testing.T) {
	tmp := t.TempDir()
	writeMarkdown(t, tmp, "contributingGuides/PROPOSAL_TEMPLATE.md", "# Proposal Template\n")
	writeMarkdown(t, tmp, "packages/assistant/agents/specification.agent.mdx", "# Specification Agent\n")
	writeMarkdown(t, tmp, "project/GOVERNANCE.md", "# Governance\n")
	writeMarkdown(t, tmp, "project/MAINTAINERS.md", "# Maintainers\n")
	writeMarkdown(t, tmp, "docs/random-note.md", "# Random\n")

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	got := candidateRelPaths(candidates)
	for _, want := range []string{
		"contributingGuides/PROPOSAL_TEMPLATE.md",
		"packages/assistant/agents/specification.agent.mdx",
		"project/GOVERNANCE.md",
		"project/MAINTAINERS.md",
	} {
		require.True(t, stringSliceContains(got, want),
			"missing high-signal markdown %q in %v", want, got)

	}
	require.False(t, stringSliceContains(got, "docs/random-note.md"),
		"unexpected random doc in high-signal discovery: %v", got)

}

func TestDiscover_DocsDir(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide"), 0o644)
	os.WriteFile(filepath.Join(docsDir, "intent.mdx"), []byte("# Intent"), 0o644)

	a := &Adapter{}
	cfg := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{Type: "markdown", Paths: []string{"docs"}},
		},
	}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	require.NoError(t, err)

	require.Len(t, candidates, 2,
		"expected 2 candidates from configured docs/, got %d", len(candidates))

	got := candidateRelPaths(candidates)
	for _, want := range []string{"docs/guide.md", "docs/intent.mdx"} {
		assert.True(t, stringSliceContains(got, want),
			"missing %q from configured docs/: %v", want, got)

	}
}

func TestParseFrontmatterTags_YAMLList(t *testing.T) {
	fm := map[string]string{"tags": "[auth, v2]"}
	tags := parseFrontmatterTags(fm)
	require.Len(t, tags, 2)
	assert.Equal(t, "auth", tags[0])
	assert.Equal(t, "v2", tags[1])

}

func TestParseFrontmatterTags_CommaSeparated(t *testing.T) {
	fm := map[string]string{"tags": "auth, v2"}
	tags := parseFrontmatterTags(fm)
	require.Len(t, tags, 2)
	assert.Equal(t, "auth", tags[0])
	assert.Equal(t, "v2", tags[1])

}

func TestParseFrontmatterTags_Labels(t *testing.T) {
	fm := map[string]string{"labels": "security"}
	tags := parseFrontmatterTags(fm)
	require.Len(t, tags, 1)
	assert.Equal(t, "security", tags[0])

}

func TestParseFrontmatterTags_Empty(t *testing.T) {
	fm := map[string]string{"tags": ""}
	tags := parseFrontmatterTags(fm)
	assert.Empty(t, tags,
		"expected empty, got %v", tags)

}

func TestParseFrontmatterTags_NoKey(t *testing.T) {
	fm := map[string]string{"title": "Test"}
	tags := parseFrontmatterTags(fm)
	assert.Empty(t, tags,
		"expected empty, got %v", tags)

}

func TestParseFrontmatterTags_Combined(t *testing.T) {
	fm := map[string]string{"tags": "[auth, v2]", "labels": "security, backend"}
	tags := parseFrontmatterTags(fm)
	assert.Len(t, tags, 4,
		"expected 4 tags, got %v", tags)

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
	require.NoError(t, err)

	require.Len(t, art.Tags, 3,
		"expected 3 tags, got %v", art.Tags)

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
	require.NoError(t, err)

	assert.False(t, stringSliceContains(art.Tags, "claude-desktop"))
	assert.Equal(t, format.ProfileClaude, art.FormatProfile)
	g, ok := art.Extracted["generator"].(string)
	require.True(t, ok)
	assert.Equal(t, "Claude Desktop", g)
}

func testSamplesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "samples"))
	require.NoError(t, err)

	return root
}

func sampleMarkdownCandidate(t *testing.T, fixture, rel string) adapters.Candidate {
	t.Helper()
	root := filepath.Join(testSamplesRoot(t), fixture)

	return adapters.Candidate{
		PrimaryPath: filepath.Join(root, filepath.FromSlash(rel)),
		RelPath:     rel,
		AdapterName: "markdown",
	}
}

func TestPathGeneratorForExtract_WithBmadArtifact_ReturnsBmad(t *testing.T) {
	actual := pathGeneratorForExtract("_bmad-output/planning-artifacts/prd.md")

	assert.Equal(t, "bmad-method", actual)
}

func TestPathGeneratorForExtract_WithSpecKitSpec_ReturnsSpecKit(t *testing.T) {
	actual := pathGeneratorForExtract("specs/001-x/spec.md")

	assert.Equal(t, "speckit", actual)
}

func TestPathGeneratorForExtract_WithSpecKitPlan_ReturnsSpecKit(t *testing.T) {
	actual := pathGeneratorForExtract("specs/001-x/plan.md")

	assert.Equal(t, "speckit", actual)
}

func TestPathGeneratorForExtract_WithCursorPlan_ReturnsCursorPlan(t *testing.T) {
	actual := pathGeneratorForExtract(".cursor/plans/foo.plan.md")

	assert.Equal(t, "cursor-plan", actual)
}

func TestPathGeneratorForExtract_WithCodexPlan_ReturnsCodex(t *testing.T) {
	actual := pathGeneratorForExtract(".codex/plans/PLAN.md")

	assert.Equal(t, "codex", actual)
}

func TestPathGeneratorForExtract_WithGenericNestedSpec_ReturnsEmpty(t *testing.T) {
	actual := pathGeneratorForExtract("plans/nested/spec.md")

	assert.Empty(t, actual)
}

func TestDiscover_SampleFixture_BMAD_ReturnsPlanningArtifacts(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "bmad")
	a := &Adapter{}

	candidates, err := a.Discover(context.Background(), root, nil)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(candidates), 2,
		"bmad fixture: want >= 2 markdown candidates, got %d", len(candidates))
	assert.NotEmpty(t, findCandidate(candidates, "_bmad-output/planning-artifacts/prd.md").PrimaryPath,
		"prd.md not discovered")
}

func TestParse_SampleFixture_BMADPrd_UsesBMADProfile(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "bmad", "_bmad-output/planning-artifacts/prd.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileBmad, art.FormatProfile,
		"expected format_profile bmad, got %q", art.FormatProfile)
	g, ok := art.Extracted["generator"].(string)
	require.True(t, ok)
	assert.Equal(t, "bmad-method", g)
}

func TestDiscover_SampleFixture_Specify_ReturnsFeatureArtifacts(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "specify")
	a := &Adapter{}

	candidates, err := a.Discover(context.Background(), root, nil)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(candidates), 8,
		"specify fixture: want >= 8 markdown candidates, got %d", len(candidates))
	assert.NotEmpty(t, findCandidate(candidates, "specs/001-synthetic-feature/spec.md").PrimaryPath,
		"spec.md not discovered under specs/001-synthetic-feature/")
}

func TestParse_SampleFixture_SpecifySpec_UsesSpecKitProfileAndLayout(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "specify", "specs/001-synthetic-feature/spec.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileSpeckit, art.FormatProfile,
		"expected format_profile speckit, got %q", art.FormatProfile)
	assert.Equal(t, "specs/001-synthetic-feature", filepath.ToSlash(art.LayoutGroup))
	g, ok := art.Extracted["generator"].(string)
	require.True(t, ok)
	assert.Equal(t, "speckit", g)
}

func TestParse_SampleFixture_SpecifyPlan_UsesFeatureLayout(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "specify", "specs/001-synthetic-feature/plan.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileSpeckit, art.FormatProfile)
	assert.Equal(t, "specs/001-synthetic-feature", filepath.ToSlash(art.LayoutGroup))
}

func TestParse_SampleFixture_SpecifyTasks_UsesFeatureLayout(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "specify", "specs/001-synthetic-feature/tasks.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileSpeckit, art.FormatProfile)
	assert.Equal(t, "specs/001-synthetic-feature", filepath.ToSlash(art.LayoutGroup))
}

func TestParse_SampleFixture_SpecifyResearch_UsesFeatureLayout(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "specify", "specs/001-synthetic-feature/research.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileSpeckit, art.FormatProfile)
	assert.Equal(t, "specs/001-synthetic-feature", filepath.ToSlash(art.LayoutGroup))
}

func TestParse_SampleFixture_SpecifyTasks_ExtractsTodos(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "specify", "specs/001-synthetic-feature/tasks.md")
	a := &Adapter{}

	_, _, result, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(result.Todos), 8,
		"specify tasks fixture: want >= 8 checklist todos, got %d", len(result.Todos))
}

func TestDiscover_SampleFixture_CursorPlan_ReturnsPlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "cursor")
	a := &Adapter{}

	candidates, err := a.Discover(context.Background(), root, nil)

	require.NoError(t, err)
	require.Len(t, candidates, 1,
		"cursor fixture: want 1 candidate, got %d (%v)", len(candidates), candidates)
	assert.Equal(t, ".cursor/plans/sample_cursor_plan.plan.md", filepath.ToSlash(candidates[0].RelPath))
}

func TestParse_SampleFixture_CursorPlan_UsesCursorProfile(t *testing.T) {
	candidate := sampleMarkdownCandidate(t, "cursor", ".cursor/plans/sample_cursor_plan.plan.md")
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, format.ProfileCursorPlan, art.FormatProfile,
		"expected format_profile cursor_plan, got %q", art.FormatProfile)
}

func TestDiscover_SampleFixture_CodexPlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "codex")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	require.NoError(t, err)

	require.Len(t, candidates, 1)
	assert.Equal(t, "plans/PLAN.md", filepath.ToSlash(candidates[0].RelPath))

}

func TestDiscover_SampleFixture_ClaudePlan(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "claude")
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), root, nil)
	require.NoError(t, err)

	require.Len(t, candidates, 1)
	assert.Equal(t, "plans/dreamy-orbiting-quokka.md", filepath.ToSlash(candidates[0].RelPath))

}

func TestDiscover_SampleFixture_FreetextWithRules_ReturnsAllCandidates(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "freetext")
	cfg := freetextRepoConfig(freetextRules())
	a := &Adapter{}

	candidates, err := a.Discover(context.Background(), root, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 20,
		"freetext fixture: want 20 markdown candidates, got %d", len(candidates))
}

func TestDiscover_SampleFixture_FreetextWithPathsOnly_ReturnsAllCandidates(t *testing.T) {
	root := filepath.Join(testSamplesRoot(t), "freetext")
	cfg := freetextRepoConfig(nil)
	a := &Adapter{}

	candidates, err := a.Discover(context.Background(), root, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 20,
		"paths-only discover: want 20 candidates, got %d", len(candidates))
}

func TestParse_SampleFixture_FreetextRoadmap_UsesPlanRule(t *testing.T) {
	candidate := freetextCandidate(t, "ROADMAP.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindPlan, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextPlanIndex_UsesPlanRule(t *testing.T) {
	candidate := freetextCandidate(t, "v2/plans/README.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindPlan, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextWorkflowIndex_UsesPlanRule(t *testing.T) {
	candidate := freetextCandidate(t, "v2/plans/01-sample-capture-workflow/README.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindPlan, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextNumberedTopic_UsesPlanRule(t *testing.T) {
	candidate := freetextCandidate(t, "v2/plans/02_TOPIC_GROUPING.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindPlan, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextWorkflowStep_UsesPlanRule(t *testing.T) {
	candidate := freetextCandidate(t, "v2/plans/01-sample-capture-workflow/03-service-integration-spike.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindPlan, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextDecision_UsesDecisionRule(t *testing.T) {
	candidate := freetextCandidate(t, "decisions/001-capture-boundary.md", freetextRules())
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindDecision, art.Kind)
	assert.Empty(t, art.Subtype)
}

func TestParse_SampleFixture_FreetextMarketingWithoutRules_UsesMarkdownArtifact(t *testing.T) {
	candidate := freetextCandidate(t, "marketing.md", nil)
	a := &Adapter{}

	art, _, _, err := a.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindMarkdownArtifact, art.Kind)
}

func freetextRepoConfig(rules []config.SourceRule) *config.RepoConfig {
	return &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{{
			Type:  "markdown",
			Paths: []string{".", "v2/plans", "decisions"},
			Rules: rules,
		}},
	}
}

func freetextRules() []config.SourceRule {
	return []config.SourceRule{
		{Match: "ROADMAP.md", Kind: config.KindPlan},
		{Match: "*/README.md", Kind: config.KindPlan},
		{Match: "README.md", Kind: config.KindPlan},
		{Match: "[0-9][0-9]_*.md", Kind: config.KindPlan},
		{Match: "*/[0-9][0-9]-*.md", Kind: config.KindPlan},
		{Match: "decisions/*.md", Kind: config.KindDecision},
	}
}

func freetextCandidate(t *testing.T, rel string, rules []config.SourceRule) adapters.Candidate {
	t.Helper()
	root := filepath.Join(testSamplesRoot(t), "freetext")

	return adapters.Candidate{
		PrimaryPath:   filepath.Join(root, filepath.FromSlash(rel)),
		RelPath:       rel,
		AdapterName:   "markdown",
		MarkdownPaths: []string{".", "v2/plans", "decisions"},
		MarkdownRules: rules,
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

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

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

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("vendor-plans/\n"), 0o644))

	pdir := filepath.Join(tmp, "plans")
	os.MkdirAll(pdir, 0o755)
	os.WriteFile(filepath.Join(pdir, "a.md"), []byte("# A\n"), 0o644)
	vdir := filepath.Join(tmp, "vendor-plans")
	os.MkdirAll(vdir, 0o755)
	os.WriteFile(filepath.Join(vdir, "b.md"), []byte("# B\n"), 0o644)

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	ctx := ignore.WithContext(context.Background(), m)
	a := &Adapter{}
	cands, err := a.Discover(ctx, tmp, nil)
	require.NoError(t, err)

	for _, c := range cands {
		require.False(t, strings.HasPrefix(c.RelPath, "vendor-plans/"),
			"got ignored path %q", c.RelPath)

	}
}

func TestInferDirectoryTag_WithNestedPlan_ReturnsParentDirectory(t *testing.T) {
	actual := InferDirectoryTag("plans/auth/middleware.plan.md")

	assert.Equal(t, "auth", actual)
}

func TestInferDirectoryTag_WithRootPlan_ReturnsEmpty(t *testing.T) {
	actual := InferDirectoryTag("plans/billing.md")

	assert.Empty(t, actual)
}

func TestInferDirectoryTag_WithRootSpec_ReturnsEmpty(t *testing.T) {
	actual := InferDirectoryTag("specs/api.md")

	assert.Empty(t, actual)
}

func TestInferDirectoryTag_WithNestedDocsFile_ReturnsParentDirectory(t *testing.T) {
	actual := InferDirectoryTag("docs/auth/login.md")

	assert.Equal(t, "auth", actual)
}

func TestInferDirectoryTag_WithCursorPlan_ReturnsEmpty(t *testing.T) {
	actual := InferDirectoryTag(".cursor/plans/foo.md")

	assert.Empty(t, actual)
}

func TestInferDirectoryTag_WithVersionedPlan_ReturnsVersion(t *testing.T) {
	actual := InferDirectoryTag("plans/v2/migration.md")

	assert.Equal(t, "v2", actual)
}

func TestInferDirectoryTag_WithRootMarkdown_ReturnsEmpty(t *testing.T) {
	actual := InferDirectoryTag("random.md")

	assert.Empty(t, actual)
}

func TestInferDirectoryTag_WithBmadArtifact_ReturnsEmpty(t *testing.T) {
	actual := InferDirectoryTag("_bmad-output/planning-artifacts/prd.md")

	assert.Empty(t, actual)
}

func TestInferDirectoryTag_WithNestedSpec_ReturnsParentDirectory(t *testing.T) {
	actual := InferDirectoryTag("specs/001-feature/foo/spec.md")

	assert.Equal(t, "foo", actual)
}
