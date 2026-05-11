package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupTestRepo(t *testing.T) (string, *store.DB) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "auth.md"), []byte("# Auth Plan\n\n- [ ] Add login\n- [x] Design schema\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return filepath.Join(tmp, "repo"), db
}

func TestScan_DetectsMarkdownPlans(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	result, err := s.Run(context.Background(), repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found["markdown"] != 1 {
		t.Errorf("expected 1 markdown found, got %d", result.Found["markdown"])
	}
	if result.New != 1 {
		t.Errorf("expected 1 new, got %d", result.New)
	}
}

func TestScan_StableIDs(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	// Get artifact ID
	var id1 string
	db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&id1)

	// Scan again
	s.Run(context.Background(), repoRoot, nil)
	var id2 string
	db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&id2)

	if id1 != id2 {
		t.Errorf("ID not stable across rescans: %q vs %q", id1, id2)
	}
}

func TestScan_NoDuplicateOnUnchanged(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)
	result, _ := s.Run(context.Background(), repoRoot, nil)

	if result.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", result.Unchanged)
	}
	if result.New != 0 {
		t.Errorf("expected 0 new, got %d", result.New)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 artifact, got %d", count)
	}
}

func TestScan_NewRevisionOnContentChange(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	// Modify the file
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	os.WriteFile(planPath, []byte("# Auth Plan v2\n\n- [ ] New task\n"), 0o644)

	result, _ := s.Run(context.Background(), repoRoot, nil)
	if result.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", result.Updated)
	}

	var revCount int
	db.QueryRow("SELECT COUNT(*) FROM artifact_revisions").Scan(&revCount)
	if revCount != 2 {
		t.Errorf("expected 2 revisions, got %d", revCount)
	}
}

func TestScan_RefreshesTodosOnRevision(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	var todoCount int
	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
	if todoCount != 2 {
		t.Errorf("expected 2 todos after first scan, got %d", todoCount)
	}

	// Change content, different todos
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	os.WriteFile(planPath, []byte("# Auth Plan\n\n- [ ] Only one todo now\n"), 0o644)

	s.Run(context.Background(), repoRoot, nil)

	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
	if todoCount != 1 {
		t.Errorf("expected 1 todo after revision, got %d", todoCount)
	}
}

func TestScan_FrontmatterOverridesHeuristics(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "test.md"), []byte("---\ntitle: Override Title\nkind: spec\nstatus: approved\n---\n# Ignored\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "markdown", Paths: []string{"plans"}}}}
	s.Run(context.Background(), filepath.Join(tmp, "repo"), cfg)

	var title, kind, status string
	db.QueryRow("SELECT title, kind, status FROM artifacts LIMIT 1").Scan(&title, &kind, &status)
	if title != "Override Title" {
		t.Errorf("expected 'Override Title', got %q", title)
	}
	if kind != "spec" {
		t.Errorf("expected 'spec', got %q", kind)
	}
	if status != "approved" {
		t.Errorf("expected 'approved', got %q", status)
	}
}

func TestScan_PersistsExtractedJSONWithFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	content := "---\ntitle: FM Title\n---\n# H1\n\nBody\n"
	os.WriteFile(filepath.Join(plansDir, "fm.md"), []byte(content), 0o644)
	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil); err != nil {
		t.Fatal(err)
	}
	var ex string
	err = db.QueryRow(`SELECT COALESCE(rv.extracted_json, '') FROM artifact_revisions rv JOIN artifacts a ON a.current_revision_id = rv.id LIMIT 1`).Scan(&ex)
	if err != nil {
		t.Fatal(err)
	}
	if ex == "" {
		t.Fatal("expected non-empty extracted_json")
	}
	if !strings.Contains(ex, "frontmatter") {
		t.Fatalf("expected frontmatter in extracted json: %s", ex)
	}
}
