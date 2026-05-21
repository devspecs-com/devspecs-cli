package openspec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
)

func setupOpenSpecRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	changeDir := filepath.Join(tmp, "openspec", "changes", "add-sso")
	os.MkdirAll(changeDir, 0o755)

	proposal := "# Add SSO Login\n\n## Acceptance Criteria\n\n- [ ] Users can login with Google\n- [ ] Users can login with GitHub\n\n## Design\n\nUse OAuth2 flow.\n"
	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte(proposal), 0o644)

	tasks := "# Tasks\n\n- [ ] Implement OAuth2 flow\n- [ ] Add Google provider\n- [x] Design database schema\n"
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte(tasks), 0o644)

	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\nDetails here.\n"), 0o644)

	return tmp
}

func TestOpenSpec_ProposalDetected(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 5 {
		t.Fatalf("expected 5 candidates, got %d", len(candidates))
	}
	if candidates[0].AdapterName != "openspec" {
		t.Errorf("expected adapter 'openspec', got %q", candidates[0].AdapterName)
	}
	if candidates[0].ArtifactScope != scopeCollection || candidates[1].ArtifactScope != scopeBundle {
		t.Fatalf("expected collection then bundle candidates, got %#v", candidates[:2])
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Title != "Add SSO Login" {
		t.Errorf("title: want 'Add SSO Login', got %q", art.Title)
	}
	if art.Kind != "spec" || art.Subtype != config.SubtypeOpenspecChild {
		t.Errorf("kind/subtype: want spec/%s, got %q/%q", config.SubtypeOpenspecChild, art.Kind, art.Subtype)
	}
	if art.Status != "proposed" {
		t.Errorf("status: want 'proposed', got %q", art.Status)
	}
	if len(pr.Criteria) != 2 {
		t.Errorf("expected 2 criteria checklists, got %d", len(pr.Criteria))
	}
	for _, c := range pr.Criteria {
		if c.CriteriaKind != todoparse.KindAcceptance {
			t.Errorf("criteria kind: want %q, got %q", todoparse.KindAcceptance, c.CriteriaKind)
		}
	}
	if len(pr.Todos) != 0 {
		t.Errorf("proposal should not duplicate tasks.md todos, got %d", len(pr.Todos))
	}
	if len(sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(sources))
	}
	if art.FormatProfile != format.ProfileOpenspec || sources[0].FormatProfile != format.ProfileOpenspec {
		t.Errorf("format_profile: want openspec, art=%q src=%q", art.FormatProfile, sources[0].FormatProfile)
	}
	wantLayout := filepath.ToSlash(filepath.Join("openspec", "changes", "add-sso"))
	if art.LayoutGroup != wantLayout || sources[0].LayoutGroup != wantLayout {
		t.Errorf("layout_group: want %q, art=%q src=%q", wantLayout, art.LayoutGroup, sources[0].LayoutGroup)
	}
	if art.Extracted["artifact_scope"] != scopeFile || art.Extracted["openspec_role"] != roleProposal {
		t.Fatalf("missing OpenSpec extracted scope/role: %#v", art.Extracted)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Subtype != config.SubtypeOpenspecChangeBundle {
		t.Fatalf("subtype = %q", art.Subtype)
	}
	if art.Title != "Add SSO Login" {
		t.Fatalf("bundle title = %q", art.Title)
	}
	if !strings.Contains(art.Body, "## Proposal") || !strings.Contains(art.Body, "## Tasks") {
		t.Fatalf("bundle body missing child sections:\n%s", art.Body)
	}
	if len(sources) != 4 {
		t.Fatalf("sources = %d, want bundle + 3 children", len(sources))
	}
	if len(pr.Todos) != 3 || len(pr.Criteria) != 2 {
		t.Fatalf("parse result todos=%d criteria=%d, want 3/2", len(pr.Todos), len(pr.Criteria))
	}
	if art.Extracted["artifact_scope"] != scopeBundle || art.Extracted["openspec_role"] != roleChangeBundle {
		t.Fatalf("missing bundle extracted metadata: %#v", art.Extracted)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.Todos) != 3 {
		t.Fatalf("expected 3 todos from tasks.md, got %d", len(pr.Todos))
	}
	if len(pr.Criteria) != 0 {
		t.Fatalf("expected 0 criteria from tasks.md, got %d", len(pr.Criteria))
	}
	todos := pr.Todos
	if todos[0].Text != "Implement OAuth2 flow" || todos[0].Done {
		t.Errorf("todo 0 wrong: %+v", todos[0])
	}
	if todos[2].Text != "Design database schema" || !todos[2].Done {
		t.Errorf("todo 2 wrong: %+v", todos[2])
	}
}

func TestOpenSpec_IdentityStableAcrossSiblingChanges(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	proposalPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "proposal.md")
	relPath := "openspec/changes/add-sso/proposal.md"

	a := &Adapter{}
	art1, _, _, _ := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: proposalPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})

	// Modify design.md (sibling)
	designPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "design.md")
	os.WriteFile(designPath, []byte("# Updated Design\nNew details.\n"), 0o644)

	art2, _, _, _ := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: proposalPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})

	if art1.SourceIdentity != art2.SourceIdentity {
		t.Errorf("identity changed when sibling modified: %q vs %q", art1.SourceIdentity, art2.SourceIdentity)
	}
}

func TestOpenSpec_ConfigCustomPath(t *testing.T) {
	tmp := t.TempDir()
	changeDir := filepath.Join(tmp, "custom", "changes", "test")
	os.MkdirAll(changeDir, 0o755)
	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Test\n"), 0o644)

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "openspec", Path: "custom"}}}
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
}

func TestOpenSpec_Parse_inferStatusVariants(t *testing.T) {
	tmp := t.TempDir()
	changeDir := filepath.Join(tmp, "openspec", "changes", "status-test")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prop := filepath.Join(changeDir, "proposal.md")
	a := &Adapter{}
	cases := []struct {
		content string
		want    string
	}{
		{"# Title\n\nstatus: accepted\n", "approved"},
		{"# Title\n\nstatus: approved\n", "approved"},
		{"# Title\n\nstatus: rejected\n", "rejected"},
		{"# Title\n\nstatus: implementing\n", "implementing"},
		{"# Title\n\nstatus: implemented\n", "implemented"},
		{"# Title\n\nPlain body.\n", "proposed"},
	}
	for _, tc := range cases {
		if err := os.WriteFile(prop, []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
			PrimaryPath: prop,
			RelPath:     "openspec/changes/status-test/proposal.md",
			AdapterName: "openspec",
		})
		if err != nil {
			t.Fatal(err)
		}
		if art.Status != tc.want {
			t.Fatalf("status want %q got %q for %q", tc.want, art.Status, tc.content)
		}
	}
}

func TestOpenSpec_Parse_titleHumanizeFallback(t *testing.T) {
	tmp := t.TempDir()
	changeDir := filepath.Join(tmp, "openspec", "changes", "my-change-id")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("No heading at top.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Adapter{}
	art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: filepath.Join(changeDir, "proposal.md"),
		RelPath:     "openspec/changes/my-change-id/proposal.md",
		AdapterName: "openspec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if art.Title == "" {
		t.Fatal("expected humanized title from change id")
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Subtype != config.SubtypeOpenspecCapabilitySpec {
		t.Fatalf("subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecCapabilitySpec)
	}
	if art.Extracted["openspec_role"] != roleCapabilitySpec {
		t.Fatalf("openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleCapabilitySpec)
	}
	if art.Extracted["openspec_role_mismatch"] != nil {
		t.Fatalf("unexpected role mismatch metadata: %#v", art.Extracted)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Subtype != config.SubtypeOpenspecChild {
		t.Fatalf("subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecChild)
	}
	if art.Extracted["openspec_role"] != roleSpecDelta {
		t.Fatalf("openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleSpecDelta)
	}
	if art.Extracted["openspec_path_role"] != roleCapabilitySpec {
		t.Fatalf("openspec_path_role = %#v, want %q", art.Extracted["openspec_path_role"], roleCapabilitySpec)
	}
	if art.Extracted["openspec_role_mismatch"] != openSpecRoleMismatchCapabilityDelta {
		t.Fatalf("openspec_role_mismatch = %#v, want %q", art.Extracted["openspec_role_mismatch"], openSpecRoleMismatchCapabilityDelta)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Extracted["openspec_role"] != roleCapabilitySpec {
		t.Fatalf("openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleCapabilitySpec)
	}
	if art.Subtype != config.SubtypeOpenspecCapabilitySpec {
		t.Fatalf("subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecCapabilitySpec)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Subtype != config.SubtypeOpenspecChild {
		t.Fatalf("subtype = %q, want %q", art.Subtype, config.SubtypeOpenspecChild)
	}
	if art.Extracted["openspec_role"] != roleSpecDelta {
		t.Fatalf("openspec_role = %#v, want %q", art.Extracted["openspec_role"], roleSpecDelta)
	}
}

func writeOpenSpecTestFile(t *testing.T, repoRoot, relPath, content string) string {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
