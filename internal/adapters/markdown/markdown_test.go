package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
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
	_, _, todos, err := a.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "plans/plan.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
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
		{"docs/requirements/auth.md", "requirements"},
		{"notes/random.md", "markdown_artifact"},
	}
	for _, tt := range tests {
		got := inferKind(tt.path)
		if got != tt.want {
			t.Errorf("inferKind(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
