package openspec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
)

func setupOpenSpecRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	changeDir := filepath.Join(tmp, "openspec", "changes", "add-sso")
	os.MkdirAll(changeDir, 0o755)

	proposal := "# Add SSO Login\n\n## Acceptance Criteria\n\n- Users can login with Google\n- Users can login with GitHub\n\n## Design\n\nUse OAuth2 flow.\n"
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
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].AdapterName != "openspec" {
		t.Errorf("expected adapter 'openspec', got %q", candidates[0].AdapterName)
	}
}

func TestOpenSpec_ParseExtractsTitleAndCriteria(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	proposalPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "proposal.md")
	relPath := "openspec/changes/add-sso/proposal.md"

	a := &Adapter{}
	art, sources, _, err := a.Parse(context.Background(), adapters.Candidate{
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
	if art.Kind != "openspec_change" {
		t.Errorf("kind: want 'openspec_change', got %q", art.Kind)
	}
	if art.Status != "proposed" {
		t.Errorf("status: want 'proposed', got %q", art.Status)
	}
	criteria, ok := art.Extracted["acceptance_criteria"].([]string)
	if !ok || len(criteria) != 2 {
		t.Errorf("expected 2 acceptance criteria, got %v", art.Extracted["acceptance_criteria"])
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
}

func TestOpenSpec_TasksFeedTodoTable(t *testing.T) {
	tmp := setupOpenSpecRepo(t)
	proposalPath := filepath.Join(tmp, "openspec", "changes", "add-sso", "proposal.md")
	relPath := "openspec/changes/add-sso/proposal.md"

	a := &Adapter{}
	_, _, todos, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: proposalPath,
		RelPath:     relPath,
		AdapterName: "openspec",
	})
	if err != nil {
		t.Fatal(err)
	}
	// tasks.md has 3 items
	if len(todos) != 3 {
		t.Fatalf("expected 3 todos from tasks.md, got %d", len(todos))
	}
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
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}
