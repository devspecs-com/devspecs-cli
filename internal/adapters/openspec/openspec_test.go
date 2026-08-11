package openspec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOpenSpecRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	changeDir := filepath.Join(tmp, "openspec", "changes", "add-sso")
	require.NoError(t, os.MkdirAll(changeDir, 0o755))

	proposal := "# Add SSO Login\n\n## Acceptance Criteria\n\n- [ ] Users can login with Google\n- [ ] Users can login with GitHub\n\n## Design\n\nUse OAuth2 flow.\n"
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte(proposal), 0o644))

	tasks := "# Tasks\n\n- [ ] Implement OAuth2 flow\n- [ ] Add Google provider\n- [x] Design database schema\n"
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte(tasks), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\nDetails here.\n"), 0o644))

	return tmp
}

func TestOpenSpec_ProposalDetected(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	require.NoError(t, err)

	require.Len(t, candidates, 5,
		"expected 5 candidates, got %d", len(candidates))
	assert.Equal(t, "openspec", candidates[0].AdapterName,
		"expected adapter 'openspec', got %q", candidates[0].AdapterName)
	assert.Equal(t, scopeCollection, candidates[0].ArtifactScope)
	assert.Equal(t, scopeBundle, candidates[1].ArtifactScope)

}

func TestOpenSpec_ParseExtractsTitleAndCriteria(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	proposalPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "proposal.md")
	relPath := "openspec/changes/add-sso/proposal.md"

	a := &Adapter{}
	art, sources, pr, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: proposalPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})
	require.NoError(t, err)

	assert.Equal(t, "Add SSO Login", art.Title,
		"title: want 'Add SSO Login', got %q", art.Title)
	assert.Equal(t, "spec", art.Kind)
	assert.Equal(t, config.SubtypeOpenspecChild, art.Subtype)
	assert.Equal(t, "proposed", art.Status,
		"status: want 'proposed', got %q", art.Status)
	require.Len(t, pr.Criteria, 2,
		"expected 2 criteria checklists, got %d", len(pr.Criteria))
	assert.Equal(t, todoparse.KindAcceptance, pr.Criteria[0].CriteriaKind)
	assert.Equal(t, todoparse.KindAcceptance, pr.Criteria[1].CriteriaKind)
	assert.Empty(t, pr.Todos,
		"proposal should not duplicate tasks.md todos, got %d", len(pr.Todos))
	require.Len(t, sources, 1,
		"expected 1 source, got %d", len(sources))
	assert.Equal(t, format.ProfileOpenspec, art.FormatProfile)
	assert.Equal(t, format.ProfileOpenspec, sources[0].FormatProfile)

	wantLayout := filepath.ToSlash(filepath.Join("openspec", "changes", "add-sso"))
	assert.Equal(t, wantLayout, art.LayoutGroup)
	assert.Equal(t, wantLayout, sources[0].LayoutGroup)
	assert.Equal(t, scopeFile, art.Extracted["artifact_scope"])
	assert.Equal(t, roleProposal, art.Extracted["openspec_role"])

}

func TestOpenSpec_ParseChangeBundleAggregatesChildren(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	changeDir := filepath.Join(tmp, "openspec", "changes", "add-sso")

	a := &Adapter{}
	art, sources, pr, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath:   changeDir,
		RelPath:       "openspec/changes/add-sso",
		AdapterName:   "openspec",
		ArtifactScope: scopeBundle,
		Role:          roleChangeBundle,
	})
	require.NoError(t, err)

	require.Equal(t, config.SubtypeOpenspecChangeBundle, art.Subtype,
		"subtype = %q", art.Subtype)
	require.Equal(t, "Add SSO Login", art.Title,
		"bundle title = %q", art.Title)
	assert.Contains(t, art.Body, "## Proposal")
	assert.Contains(t, art.Body, "## Tasks")
	require.Len(t, sources, 4,
		"sources = %d, want bundle + 3 children", len(sources))
	require.Len(t, pr.Todos, 3)
	require.Len(t, pr.Criteria, 2)
	assert.Equal(t, scopeBundle, art.Extracted["artifact_scope"])
	assert.Equal(t, roleChangeBundle, art.Extracted["openspec_role"])

}

func TestOpenSpec_TasksChildFeedsTodoTable(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	tasksPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "tasks.md")
	relPath := "openspec/changes/add-sso/tasks.md"

	a := &Adapter{}
	_, _, pr, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: tasksPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})
	require.NoError(t, err)

	require.Len(t, pr.Todos, 3,
		"expected 3 todos from tasks.md, got %d", len(pr.Todos))
	require.Empty(t, pr.Criteria,
		"expected 0 criteria from tasks.md, got %d", len(pr.Criteria))

	todos := pr.Todos
	assert.Equal(t, "Implement OAuth2 flow", todos[0].Text)
	assert.False(t, todos[0].Done)
	assert.Equal(t, "Design database schema", todos[2].Text)
	assert.True(t, todos[2].Done)

}

func TestOpenSpec_WithChangedSibling_ReturnsPathBasedIdentity(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	proposalPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "proposal.md")
	relPath := "openspec/changes/add-sso/proposal.md"
	designPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "design.md")
	require.NoError(t, os.WriteFile(designPath, []byte("# Updated Design\nNew details.\n"), 0o644))
	a := &Adapter{}

	artifact, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: proposalPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})

	require.NoError(t, err)
	assert.Equal(t, "openspec/changes/add-sso/proposal.md|openspec", artifact.SourceIdentity)
}

func TestOpenSpec_ConfigCustomPath(t *testing.T) {
	tmp := t.TempDir()
	changeDir := filepath.Join(tmp, "custom", "changes", "test")
	require.NoError(t, os.MkdirAll(changeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Test\n"), 0o644))

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "openspec", Path: "custom"}}}
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	require.NoError(t, err)

	require.Len(t, candidates, 3,
		"expected 3 candidates, got %d", len(candidates))

}

func TestOpenSpec_ParseAcceptedStatus_ReturnsApproved(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nstatus: accepted\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "approved", artifact.Status)
}

func TestOpenSpec_ParseApprovedStatus_ReturnsApproved(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nstatus: approved\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "approved", artifact.Status)
}

func TestOpenSpec_ParseRejectedStatus_ReturnsRejected(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nstatus: rejected\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "rejected", artifact.Status)
}

func TestOpenSpec_ParseImplementingStatus_ReturnsImplementing(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nstatus: implementing\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "implementing", artifact.Status)
}

func TestOpenSpec_ParseImplementedStatus_ReturnsImplemented(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nstatus: implemented\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "implemented", artifact.Status)
}

func TestOpenSpec_ParseWithoutStatus_ReturnsProposed(t *testing.T) {
	candidate := writeOpenSpecStatusFixture(t, "# Title\n\nPlain body.\n")

	artifact, _, _, err := (&Adapter{}).Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, "proposed", artifact.Status)
}

func TestOpenSpec_Parse_titleHumanizeFallback(t *testing.T) {
	tmp := t.TempDir()
	changeDir := filepath.Join(tmp, "openspec", "changes", "my-change-id")

	require.NoError(t, os.MkdirAll(changeDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("No heading at top.\n"), 0o644))

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: filepath.Join(changeDir, "proposal.md"),
		RelPath:     "openspec/changes/my-change-id/proposal.md",
		AdapterName: "openspec",
	})
	require.NoError(t, err)

	require.NotEmpty(t, art.Title,
		"expected humanized title from change id")

}

func TestOpenSpec_CapabilitySpecPathStaysCanonical(t *testing.T) {
	tmp := t.TempDir()
	relPath := "openspec/specs/billing/spec.md"
	specPath := writeOpenSpecTestFile(t, tmp, relPath, "# Billing\n\n## Requirements\n\n### Requirement: Checkout\n\nThe system SHALL process payments.\n")

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath:   specPath,
		RelPath:       relPath,
		AdapterName:   "openspec",
		ArtifactScope: scopeFile,
		Role:          roleCapabilitySpec,
	})
	require.NoError(t, err)

	require.Equal(t, config.SubtypeOpenspecCapabilitySpec, art.Subtype,
		"subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecCapabilitySpec)
	require.Equal(t, roleCapabilitySpec, art.Extracted["openspec_role"],
		"openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleCapabilitySpec)
	require.Nil(t, art.Extracted["openspec_role_mismatch"],
		"unexpected role mismatch metadata: %#v", art.Extracted)

}

func TestOpenSpec_CapabilitySpecWithDeltaHeadingReassigned(t *testing.T) {
	tmp := t.TempDir()
	relPath := "openspec/specs/billing/spec.md"
	specPath := writeOpenSpecTestFile(t, tmp, relPath, "# Billing\n\n## ADDED Requirements\n\n### Requirement: Refunds\n\nThe system SHALL support refunds.\n")

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath:   specPath,
		RelPath:       relPath,
		AdapterName:   "openspec",
		ArtifactScope: scopeFile,
		Role:          roleCapabilitySpec,
	})
	require.NoError(t, err)

	require.Equal(t, config.SubtypeOpenspecChild, art.Subtype,
		"subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecChild)
	require.Equal(t, roleSpecDelta, art.Extracted["openspec_role"],
		"openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleSpecDelta)
	require.Equal(t, roleCapabilitySpec, art.Extracted["openspec_path_role"],
		"openspec_path_role = %#v, want %q", art.Extracted["openspec_path_role"], roleCapabilitySpec)
	require.Equal(t, openSpecRoleMismatchCapabilityDelta, art.Extracted["openspec_role_mismatch"],
		"openspec_role_mismatch = %#v, want %q", art.Extracted["openspec_role_mismatch"], openSpecRoleMismatchCapabilityDelta)

}

func TestOpenSpec_DeltaHeadingInsideFenceDoesNotReassign(t *testing.T) {
	tmp := t.TempDir()
	relPath := "openspec/specs/billing/spec.md"
	specPath := writeOpenSpecTestFile(t, tmp, relPath, "# Billing\n\n```md\n## ADDED Requirements\n```\n\n## Requirements\n\n### Requirement: Checkout\n\nThe system SHALL process payments.\n")

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath:   specPath,
		RelPath:       relPath,
		AdapterName:   "openspec",
		ArtifactScope: scopeFile,
		Role:          roleCapabilitySpec,
	})
	require.NoError(t, err)

	require.Equal(t, roleCapabilitySpec, art.Extracted["openspec_role"],
		"openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleCapabilitySpec)
	require.Equal(t, config.SubtypeOpenspecCapabilitySpec, art.Subtype,
		"subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecCapabilitySpec)

}

func TestOpenSpec_ChangeSpecPathStaysDeltaWithoutDeltaHeading(t *testing.T) {
	tmp := t.TempDir()
	relPath := "openspec/changes/add-billing/specs/billing/spec.md"
	specPath := writeOpenSpecTestFile(t, tmp, relPath, "# Billing\n\n## Requirements\n\n### Requirement: Checkout\n\nThe system SHALL process payments.\n")

	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: specPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})
	require.NoError(t, err)

	require.Equal(t, config.SubtypeOpenspecChild, art.Subtype,
		"subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecChild)
	require.Equal(t, roleSpecDelta, art.Extracted["openspec_role"],
		"openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleSpecDelta)

}

func writeOpenSpecTestFile(t *testing.T, repoRoot, relPath, content string) string {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func writeOpenSpecStatusFixture(t *testing.T, content string) adapters.Candidate {
	t.Helper()
	root := t.TempDir()
	path := writeOpenSpecTestFile(t, root, "openspec/changes/status-test/proposal.md", content)
	return adapters.Candidate{PrimaryPath: path, RelPath: "openspec/changes/status-test/proposal.md", AdapterName: "openspec"}
}
