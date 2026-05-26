package retrieval

import "testing"

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
	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected one excluded noise item, got %d", len(pack.ExcludedNoise))
	}
	if pack.ExcludedNoise[0].Path != "AGENTS.md" {
		t.Fatalf("expected AGENTS.md to be excluded, got %q", pack.ExcludedNoise[0].Path)
	}
	if pack.ExcludedNoise[0].RoleReason == "" {
		t.Fatal("excluded noise item should explain why it was excluded")
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested agent instructions to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested project guidelines to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
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

	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected unrequested agent instructions to be excluded, got %#v", pack)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected agent note to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
	if got := includedPackItemCount(pack); got != 1 {
		t.Fatalf("expected one included agent note, got %d in %#v", got, pack.Groups)
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested skill workflow to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected skill with rare title/path overlap to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested skill under agent directory to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested skill reference path to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected unrequested generic skill to be excluded, got %#v", pack)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested protocol/template artifacts to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected unrequested template to be excluded, got %#v", pack)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected targeted stale artifact to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected current design doc to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected requested root instructions with weak stale metadata to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
	assertGroupCount(t, pack, PackRoleSupportingContext, 1)
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

	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected archived instructions to stay excluded for current-rules query, got %#v", pack)
	}
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

	if len(pack.ExcludedNoise) != 0 {
		t.Fatalf("expected historical archive artifact to stay included, got excluded: %#v", pack.ExcludedNoise)
	}
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

	if len(pack.ExcludedNoise) != 1 {
		t.Fatalf("expected weak stale artifact to be excluded, got %#v", pack)
	}
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

	if len(pack.Groups) != 1 || len(pack.Groups[0].Items) != 1 {
		t.Fatalf("expected active artifact to be included, got %#v", pack)
	}
	for _, cue := range pack.Groups[0].Items[0].AuthorityCues {
		if cue == "superseded" {
			t.Fatalf("active artifact should not show stale cue: %#v", pack.Groups[0].Items[0].AuthorityCues)
		}
	}
}

func assertGroupCount(t *testing.T, pack RoleGroupedPack, role string, want int) {
	t.Helper()
	for _, group := range pack.Groups {
		if group.Role == role {
			if got := len(group.Items); got != want {
				t.Fatalf("role %s: want %d items, got %d", role, want, got)
			}
			return
		}
	}
	if want != 0 {
		t.Fatalf("role %s: want %d items, group missing", role, want)
	}
}

func includedPackItemCount(pack RoleGroupedPack) int {
	var count int
	for _, group := range pack.Groups {
		count += len(group.Items)
	}
	return count
}
