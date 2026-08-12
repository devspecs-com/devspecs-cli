package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRoleGroupedPackClassifiesCoreRolesAndNoise(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "adr-1",
			Path:  "docs/adr/001-auth-session.md",
			Kind:  "decision",
			Title: "Auth session decision",
			Body:  "refresh token session model",
			Metadata: map[string]string{
				"short_id": "ADR1",
			},
		},
		{
			ID:    "src-1",
			Path:  "internal/auth/refresh_token.go",
			Kind:  "source_context",
			Title: "RefreshTokenService",
			Body:  "func RotateRefreshToken() {}",
		},
		{
			ID:      "test-1",
			Path:    "internal/auth/refresh_token_test.go",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "TestRotatesRefreshToken",
			Body:    "rejects expired refresh token",
		},
		{
			ID:      "config-1",
			Path:    "config/auth.yaml",
			Kind:    "source_context",
			Subtype: "configuration",
			Title:   "auth.yaml",
			Body:    "refresh_token_ttl: 3600",
		},
		{
			ID:    "agent-1",
			Path:  "AGENTS.md",
			Kind:  "protocol",
			Title: "Agent instructions",
			Body:  "rules for working in this repo",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
	}
	reasons := map[string][]string{
		"docs/adr/001-auth-session.md":        {"matched query term: refresh"},
		"internal/auth/refresh_token.go":      {"matched identifier: refresh_token"},
		"internal/auth/refresh_token_test.go": {"matched test behavior: rotates refresh token"},
		"config/auth.yaml":                    {"matched config key: refresh_token_ttl"},
		"AGENTS.md":                           {"matched query term: agent"},
	}

	pack := BuildRoleGroupedPack(candidates, reasons, "resume auth token refresh work")

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
	assertGroupCount(t, pack, PackRoleImplementation, 1)
	assertGroupCount(t, pack, PackRoleBehaviorTests, 1)
	assertGroupCount(t, pack, PackRoleConfigSchema, 1)
	assert.Equal(t, 4, pack.Summary.IncludedCount)
	assert.Equal(t, 4, pack.Summary.RoleDiversity)
	assert.True(t, pack.Summary.HasBackgroundDecisions)
	assert.True(t, pack.Summary.HasImplementation)
	assert.True(t, pack.Summary.HasBehaviorTests)
	assert.True(t, pack.Summary.HasConfigSchema)
	require.Len(t, pack.ExcludedNoise, 1,
		"expected one excluded noise item, got %d", len(pack.ExcludedNoise))
	require.Equal(t, 1, pack.Summary.ExcludedNoiseCount,
		"summary excluded count = %d", pack.Summary.ExcludedNoiseCount)
	require.NotEmpty(t, pack.Summary.Notes,
		"expected summary notes: %#v", pack.Summary)
	require.Equal(t, "AGENTS.md", pack.ExcludedNoise[0].Path,
		"expected AGENTS.md to be excluded, got %q", pack.ExcludedNoise[0].Path)
	require.NotEqual(t, "", pack.ExcludedNoise[0].RoleReason,
		"excluded noise item should explain why it was excluded")

}

func TestBuildRoleGroupedPackIncludesAgentInstructionsWhenRequested(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "agent-1",
			Path:  "AGENTS.md",
			Kind:  "protocol",
			Title: "Agent instructions",
			Body:  "rules for working in this repo",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "show repo agent instructions and rules")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested agent instructions to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackAddsLocalLanguageReceipts(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "resolve",
			Path:  "lib/helpers/resolveConfig.js",
			Kind:  "source_context",
			Title: "resolveConfig",
			Body:  "withXSRFToken xsrfCookieName xsrfHeaderName X-XSRF-TOKEN",
		},
		{
			ID:    "defaults",
			Path:  "lib/defaults/index.js",
			Kind:  "source_context",
			Title: "defaults",
			Body:  "xsrfCookieName xsrfHeaderName",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "xsrf csrf cookie header")
	receipts := LocalLanguageReceipts(pack)
	require.NotEmpty(t, receipts,
		"expected local language receipt: %#v", pack.Metadata)
	require.Equal(t, "XSRF/CSRF maps to withXSRFToken, xsrfCookieName, and xsrfHeaderName", receipts[0],
		"unexpected receipt: %#v", receipts)

}

func TestBuildRoleGroupedPackClassifiesRawTestSourceByPath(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "src",
			Path:  "src/language-yaml/parser-yaml.js",
			Kind:  "source_context",
			Title: "parser-yaml.js",
			Body:  "YAML parser implementation",
		},
		{
			ID:    "raw-test",
			Path:  "tests/format/yaml/yaml-test-suite/format.test.js",
			Kind:  "source_context",
			Title: "format.test.js",
			Body:  "YAML formatting behavior",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "yaml parser formatting")

	assertGroupCount(t, pack, PackRoleImplementation, 1)
	assertGroupCount(t, pack, PackRoleBehaviorTests, 1)
	require.True(t, pack.Summary.HasBehaviorTests,
		"expected behavior-test coverage in summary: %#v", pack.Summary)

	for _, group := range pack.Groups {
		if group.Role != PackRoleBehaviorTests {
			continue
		}
		require.Equal(t, "test file captures expected behavior", group.Items[0].RoleReason,
			"role reason = %q", group.Items[0].RoleReason)

	}
}

func TestBuildRoleGroupedPackIncludesProjectGuidelinesWhenRequested(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "agent-1",
			Path:  "docs/project/AGENTS.md",
			Kind:  "protocol",
			Title: "Project Guidelines",
			Body:  "coding standards and constraints for contributors",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "before editing payment code load the project guidelines and constraints")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested project guidelines to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackIncludesTopLevelClaudeGuidanceBeforeGenericSkills(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "agent-1",
			Path:    "CLAUDE.md",
			Kind:    "markdown_artifact",
			Subtype: "agent_instruction",
			Title:   "Claude Code Development Guidance",
			Body:    "Guidance for working in the Deno repository",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
		{
			ID:      "skill-1",
			Path:    ".claude/skills/review-pr/SKILL.md",
			Kind:    "markdown_artifact",
			Subtype: "skill",
			Title:   "Deno PR Review Skill",
			Body:    "Skill workflow for reviewing Deno pull requests",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "skill",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "Claude Code development guidance for working in the Deno repository")

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
	{
		got := pack.Groups[0].Items[0].Path
		require.Equal(t, "CLAUDE.md", got,
			"expected top-level CLAUDE.md to be included, got %q", got)
	}
	require.Len(t, pack.ExcludedNoise, 1)
	assert.Equal(t, ".claude/skills/review-pr/SKILL.md", pack.ExcludedNoise[0].Path)

}

func TestBuildRoleGroupedPackExcludesUnrequestedAgentInstructionsForAgentFeature(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "agent-1",
			Path:  "AGENTS.md",
			Kind:  "protocol",
			Title: "Agent instructions",
			Body:  "rules for working in this repo",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "implement the support agent routing feature")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected unrequested agent instructions to be excluded, got %#v", pack)

}

func TestBuildRoleGroupedPackDoesNotExcludeAgentNotesAsInstructions(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "note-1",
			Path:  ".claude/notes/webhook-idempotency-followup.md",
			Kind:  "markdown_artifact",
			Title: "Claude Notes: Webhook Idempotency Follow-up",
			Body:  "stripe_event_id replay protection follow up",
			Metadata: map[string]string{
				"classifier_model": "agent_note",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "give agent context to implement webhook replay protection")
	require.Empty(t, pack.ExcludedNoise,
		"expected agent note to stay included, got excluded: %#v", pack.ExcludedNoise)
	{

		got := includedPackItemCount(pack)
		require.Equal(t, 1, got,
			"expected one included agent note, got %d in %#v", got, pack.Groups)
	}

}

func TestBuildRoleGroupedPackIncludesRequestedSkillWorkflow(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "skill-1",
			Path:  ".codex/skills/data-repair/SKILL.md",
			Kind:  "protocol",
			Title: "Data Repair Workflow",
			Body:  "workflow for reconciling account records",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "skill",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "use the data repair workflow to reconcile account records")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested skill workflow to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackIncludesSkillByRarePathToken(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "skill-1",
			Path:  ".claude/skills/invoice-reconciler/SKILL.md",
			Kind:  "protocol",
			Title: "Invoice Reconciler",
			Body:  "specialized workflow for invoice matching",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "skill",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "run invoice reconciler for vendor matching")
	require.Empty(t, pack.ExcludedNoise,
		"expected skill with rare title/path overlap to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackIncludesSkillUnderAgentDirectoryWhenRequested(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "skill-1",
			Path:    "external/agents/biology-agent/skills/metabolic-simulator/SKILL.md",
			Kind:    "markdown_artifact",
			Subtype: "skill",
			Title:   "Run Metabolic Simulation",
			Body:    "workflow for flux balance analysis",
			Metadata: map[string]string{
				"classifier_model": "protocol",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "run flux balance analysis for a biology agent experiment")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested skill under agent directory to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackIncludesSkillReferencePathWhenWorkflowRequested(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "skill-ref-1",
			Path:  "skills/browser/references/agent.md",
			Kind:  "markdown_artifact",
			Title: "Agent Configuration",
			Body:  "reference for browser automation workflow behavior",
			Metadata: map[string]string{
				"classifier_model": "protocol",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "automate browser interactions with the browser cli skill workflow")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested skill reference path to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackExcludesUnrequestedGenericSkill(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "skill-1",
			Path:  ".codex/skills/documentation/SKILL.md",
			Kind:  "protocol",
			Title: "Documentation Skill",
			Body:  "generic documentation writing workflow",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "skill",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "implement refresh token rotation")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected unrequested generic skill to be excluded, got %#v", pack)

}

func TestBuildRoleGroupedPackIncludesRequestedProtocolAndTemplateArtifacts(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "contrib-1",
			Path:  "CONTRIBUTING.md",
			Kind:  "protocol",
			Title: "Contribution Guidelines",
			Body:  "repository contribution constraints",
			Metadata: map[string]string{
				"classifier_model": "protocol",
			},
		},
		{
			ID:    "pr-template-1",
			Path:  ".github/PULL_REQUEST_TEMPLATE/default.md",
			Kind:  "template",
			Title: "Pull Request Template",
			Body:  "checklist for pull requests",
			Metadata: map[string]string{
				"classifier_model": "template",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "prepare the pull request workflow and contribution guidelines")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested protocol/template artifacts to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 2)
}

func TestBuildRoleGroupedPackExcludesUnrequestedTemplateForImplementationQuery(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "pr-template-1",
			Path:  ".github/PULL_REQUEST_TEMPLATE/default.md",
			Kind:  "template",
			Title: "Pull Request Template",
			Body:  "checklist for pull requests",
			Metadata: map[string]string{
				"classifier_model": "template",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "implement refresh token rotation")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected unrequested template to be excluded, got %#v", pack)

}

func TestBuildRoleGroupedPackDeduplicatesSamePath(t *testing.T) {
	candidates := []Candidate{
		{ID: "a", Path: "docs/adr/0002-webhook-idempotency-boundary.md", Kind: "decision", Subtype: "adr", Title: "ADR 0002"},
		{ID: "b", Path: "docs/adr/0002-webhook-idempotency-boundary.md", Kind: "markdown_artifact", Title: "ADR 0002"},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "webhook idempotency")

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
}

func TestBuildRoleGroupedPackKeepsStaleArtifactWhenQueryTargetsIt(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "adr-stale",
			Path:    "docs/adr/0003-superseded-local-entitlements.md",
			Kind:    "decision",
			Subtype: "adr",
			Title:   "ADR 0003: Superseded Local Entitlements Cache",
			Status:  "superseded",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "continue local entitlement caching plan")
	require.Empty(t, pack.ExcludedNoise,
		"expected targeted stale artifact to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
}

func TestBuildRoleGroupedPackKeepsCurrentDesignDocWithDeprecatedBodyText(t *testing.T) {
	candidates := []Candidate{
		{
			ID:     "design-1",
			Path:   "docs/design/session-model.md",
			Kind:   "design",
			Title:  "Session Model",
			Status: "current",
			Body:   "The previous cookie strategy is deprecated; this document describes the current token session model.",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "understand token session model")
	require.Empty(t, pack.ExcludedNoise,
		"expected current design doc to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
}

func TestBuildRoleGroupedPackKeepsRequestedBEPWithWeakStaleMetadata(t *testing.T) {
	candidates := []Candidate{
		{
			ID:     "bep-1",
			Path:   "docs/architecture-decisions/beps/0012-metrics-service.md",
			Kind:   "markdown_artifact",
			Title:  "BEP 0012: Metrics Service",
			Status: "unknown",
			Body:   "Backstage metrics service proposal with OpenTelemetry naming conventions",
			Metadata: map[string]string{
				"classifier_model":     "design",
				"classifier_lifecycle": "deprecated",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "Backstage core MetricsService BEP proposal with OpenTelemetry naming conventions")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested BEP with weak stale metadata to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
}

func TestBuildRoleGroupedPackKeepsRequestedInstructionWithWeakStaleMetadata(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "agent-1",
			Path:    "AGENTS.md",
			Kind:    "markdown_artifact",
			Subtype: "agent_instruction",
			Title:   "Repository Guidelines",
			Status:  "unknown",
			Body:    "current project instructions and constraints",
			Metadata: map[string]string{
				"classifier_model":     "protocol",
				"classifier_subtype":   "agent_instruction",
				"classifier_lifecycle": "stale",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "load the project-specific instructions and constraints before editing")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested root instructions with weak stale metadata to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
}

func TestBuildRoleGroupedPackKeepsStrongSourceDespiteWeakStaleMetadata(t *testing.T) {
	candidates := []Candidate{
		{
			ID:     "game",
			Path:   "client/src/game/Game.ts",
			Kind:   "source_context",
			Title:  "Game",
			Status: "unknown",
			Body:   "The game coordinates RTS camera mode and input controls.",
			Metadata: map[string]string{
				"source_type":           "source_context",
				"classifier_lifecycle":  "stale",
				"classifier_confidence": "0.51",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "game rts camera mode")
	require.Empty(t, pack.ExcludedNoise,
		"expected strong source match to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleImplementation, 1)
}

func TestBuildRoleGroupedPackCollapsesLocalizedMirrors(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "en",
			Path:  "docs/en/docs/tutorial/background-tasks.md",
			Kind:  "markdown_artifact",
			Title: "Background Tasks",
		},
		{
			ID:    "uk",
			Path:  "docs/uk/docs/tutorial/background-tasks.md",
			Kind:  "markdown_artifact",
			Title: "Background Tasks",
		},
		{
			ID:    "zh",
			Path:  "docs/zh-hant/docs/tutorial/background-tasks.md",
			Kind:  "markdown_artifact",
			Title: "Background Tasks",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "background tasks")
	require.Equal(t, 1, pack.Summary.IncludedCount,
		"expected one localized mirror keeper, got summary=%#v groups=%#v", pack.Summary, pack.Groups)
	require.Len(t, pack.Groups, 1)
	require.Len(t, pack.Groups[0].Items, 1)
	assert.Equal(t, "docs/en/docs/tutorial/background-tasks.md", pack.Groups[0].Items[0].Path)

}

func TestBuildRoleGroupedPackExcludesArchivedInstructionWhenCurrentRulesRequested(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "agent-1",
			Path:    "docs/archive/AGENTS.md",
			Kind:    "markdown_artifact",
			Subtype: "agent_instruction",
			Title:   "Archived Repository Guidelines",
			Status:  "unknown",
			Body:    "old project instructions",
			Metadata: map[string]string{
				"classifier_model":   "protocol",
				"classifier_subtype": "agent_instruction",
			},
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "load the project-specific instructions and constraints before editing")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected archived instructions to stay excluded for current-rules query, got %#v", pack)

}

func TestBuildRoleGroupedPackKeepsArchivedArtifactWhenQueryRequestsHistory(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "adr-stale",
			Path:    "docs/archive/adr/0003-token-cache.md",
			Kind:    "decision",
			Subtype: "adr",
			Title:   "ADR 0003: Token Cache",
			Status:  "archived",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "review token cache history and drift")
	require.Empty(t, pack.ExcludedNoise,
		"expected historical archive artifact to stay included, got excluded: %#v", pack.ExcludedNoise)

	assertGroupCount(t, pack, PackRoleBackgroundDecisions, 1)
}

func TestBuildRoleGroupedPackExcludesWeakStaleArtifact(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "adr-stale",
			Path:    "docs/adr/0003-superseded-local-entitlements.md",
			Kind:    "decision",
			Subtype: "adr",
			Title:   "ADR 0003: Superseded Local Entitlements Cache",
			Status:  "superseded",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "resume entitlement sync hardening")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected weak stale artifact to be excluded, got %#v", pack)

}

func TestBuildRoleGroupedPackSuppressesConflictingStaleCueForActiveArtifact(t *testing.T) {
	candidates := []Candidate{
		{
			ID:     "proposal",
			Path:   "openspec/changes/harden-entitlement-sync/proposal.md",
			Kind:   "spec",
			Title:  "Harden Entitlement Sync",
			Status: "implementing",
			Body:   "This proposal supersedes the old local entitlement cache.",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "resume entitlement sync hardening")
	require.Len(t, pack.Groups, 1)
	require.Len(t, pack.Groups[0].Items, 1)

	for _, cue := range pack.Groups[0].Items[0].AuthorityCues {
		require.NotEqual(t, "superseded", cue,
			"active artifact should not show stale cue: %#v", pack.Groups[0].Items[0].AuthorityCues)

	}
}

func TestBuildRoleGroupedPackDampensUnrequestedFixtureSampleArtifacts(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "sample",
			Path:    "testdata/samples/webhook-payload.md",
			Kind:    "markdown_artifact",
			Subtype: "documentation",
			Title:   "Webhook Payload Sample",
			Body:    "webhook replay payload fixture",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "implement webhook replay handling")
	require.Len(t, pack.ExcludedNoise, 1,
		"expected unrequested fixture/sample to be excluded, got %#v", pack)
	require.NotEqual(t, "", pack.ExcludedNoise[0].RoleReason,
		"expected fixture/sample exclusion reason, got %#v", pack.ExcludedNoise[0])

}

func TestBuildRoleGroupedPackKeepsExplicitlyRequestedFixtureSampleArtifacts(t *testing.T) {
	candidates := []Candidate{
		{
			ID:      "sample",
			Path:    "testdata/samples/webhook-payload.md",
			Kind:    "markdown_artifact",
			Subtype: "documentation",
			Title:   "Webhook Payload Sample",
			Body:    "webhook replay payload fixture",
		},
	}

	pack := BuildRoleGroupedPack(candidates, nil, "inspect webhook replay testdata sample")
	require.Empty(t, pack.ExcludedNoise,
		"expected requested fixture/sample to stay available, got %#v", pack.ExcludedNoise)
	require.Equal(t, 1, includedPackItemCount(pack),
		"expected requested fixture/sample to be included, got %#v", pack)

}

func assertGroupCount(t *testing.T, pack RoleGroupedPack, role string, want int) {
	t.Helper()
	for _, group := range pack.Groups {
		if group.Role == role {
			{
				got := len(group.Items)
				require.Equal(t, want, got,
					"role %s: want %d items, got %d", role, want, got)
			}

			return
		}
	}
	require.Equal(t, 0, want,
		"role %s: want %d items, group missing", role, want)

}

func includedPackItemCount(pack RoleGroupedPack) int {
	var count int
	for _, group := range pack.Groups {
		count += len(group.Items)
	}
	return count
}
