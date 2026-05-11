package markdown

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		{"v0.prd.md", "prd"},
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

func TestDefaultPaths_IncludesDocs(t *testing.T) {
	paths := defaultPaths()
	required := []string{"docs", "_bmad-output", ".specify/memory"}
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
}

func TestRootGlobs_AllPatterns(t *testing.T) {
	globs := rootGlobs()
	expected := []string{"*.spec.md", "*.plan.md", "*.prd.md", "*.design.md", "*.contract.md", "*.requirements.md"}
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
	os.WriteFile(filepath.Join(tmp, "api.design.md"), []byte("# Design"), 0o644)
	os.WriteFile(filepath.Join(tmp, "auth.contract.md"), []byte("# Contract"), 0o644)
	os.WriteFile(filepath.Join(tmp, "reqs.requirements.md"), []byte("# Reqs"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 4 {
		t.Fatalf("expected 4 root glob candidates, got %d", len(candidates))
	}
}

func TestDiscover_DocsDir(t *testing.T) {
	tmp := t.TempDir()
	docsDir := filepath.Join(tmp, "docs")
	os.MkdirAll(docsDir, 0o755)
	os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte("# Guide"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate from docs/, got %d", len(candidates))
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

func TestParse_GeneratorFrontmatterAddsSlugTagAndExtracted(t *testing.T) {
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
	if !stringSliceContains(art.Tags, "claude-desktop") {
		t.Fatalf("expected slug tag claude-desktop, got %#v", art.Tags)
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

func TestPathGeneratorHints(t *testing.T) {
	tests := []struct {
		relPath      string
		wantTags     []string
		wantGen      string
	}{
		{"_bmad-output/planning-artifacts/prd.md", []string{"bmad"}, "bmad-method"},
		{"specs/001-x/spec.md", []string{"speckit"}, "speckit"},
		{".cursor/plans/foo.plan.md", []string{"cursor"}, "cursor-plan"},
		{"plans/nested/spec.md", nil, ""},
	}
	for _, tt := range tests {
		tags, gen := pathGeneratorHints(tt.relPath)
		if len(tags) != len(tt.wantTags) {
			t.Fatalf("%q: tags %#v want %#v", tt.relPath, tags, tt.wantTags)
		}
		for i := range tt.wantTags {
			if tags[i] != tt.wantTags[i] {
				t.Errorf("%q: tag[%d] got %q want %q", tt.relPath, i, tags[i], tt.wantTags[i])
			}
		}
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
	if !stringSliceContains(art.Tags, "bmad") {
		t.Fatalf("expected tag bmad, got %#v", art.Tags)
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
	if !stringSliceContains(art.Tags, "speckit") {
		t.Fatalf("expected tag speckit, got %#v", art.Tags)
	}
	if g, _ := art.Extracted["generator"].(string); g != "speckit" {
		t.Fatalf("extracted generator: want speckit, got %q", g)
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
	_, _, todos, err := a.Parse(context.Background(), tasksCand)
	if err != nil {
		t.Fatal(err)
	}
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
	if !stringSliceContains(art.Tags, "cursor") {
		t.Fatalf("expected tag cursor, got %#v", art.Tags)
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

func stringSliceContains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
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
