package scan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
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
	// Same map the markdown adapter produces for this file (full parity through scan path).
	md := &markdown.Adapter{}
	repoRoot := filepath.Join(tmp, "repo")
	abs := filepath.Join(repoRoot, "plans", "fm.md")
	wantArt, _, _, err := md.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: abs,
		RelPath:     "plans/fm.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ex), &got); err != nil {
		t.Fatalf("stored json: %v", err)
	}
	wantCanon, err := extractedJSONRoundTrip(wantArt.Extracted)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, wantCanon) {
		t.Fatalf("extracted_json != parsed Extracted (JSON semantics)\ngot:  %#v\nwant: %#v", got, wantCanon)
	}
}

func extractedJSONRoundTrip(m map[string]any) (map[string]any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func testdataSamplesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "samples"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestScan_CursorPlanSample_NoPathToolTagInDB verifies plan § success: after scan,
// artifact_tags must not gain path-derived tool slugs, and sources.format_profile is set.
func TestScan_CursorPlanSample_NoPathToolTagInDB(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)

	srcRoot := filepath.Join(testdataSamplesRoot(t), "cursor")
	planSrc := filepath.Join(srcRoot, ".cursor", "plans", "probabilistic_related_specs_481c4b3f.plan.md")
	data, err := os.ReadFile(planSrc)
	if err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(tmp, "repo")
	dstDir := filepath.Join(repoRoot, ".cursor", "plans")
	os.MkdirAll(dstDir, 0o755)
	dstPath := filepath.Join(dstDir, "probabilistic_related_specs_481c4b3f.plan.md")
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), repoRoot, nil); err != nil {
		t.Fatal(err)
	}

	var artifactID string
	err = db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&artifactID)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query("SELECT tag FROM artifact_tags WHERE artifact_id = ?", artifactID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatal(err)
		}
		if tag == "cursor" {
			t.Fatalf("path-derived tool slug must not appear in artifact_tags after scan, got tag %q", tag)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var profile string
	err = db.QueryRow("SELECT format_profile FROM sources WHERE artifact_id = ?", artifactID).Scan(&profile)
	if err != nil {
		t.Fatal(err)
	}
	if profile != format.ProfileCursorPlan {
		t.Fatalf("sources.format_profile: want %q, got %q", format.ProfileCursorPlan, profile)
	}
}

func TestScan_SourcesBreakdown_MultipleMarkdownFormats(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoRoot := filepath.Join(tmp, "repo")
	plansDir := filepath.Join(repoRoot, "plans")
	cursorDir := filepath.Join(repoRoot, ".cursor", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.MkdirAll(cursorDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "plain.md"), []byte("# Plain\n\nBody.\n"), 0o644)
	os.WriteFile(filepath.Join(cursorDir, "c.md"), []byte("# Cursorish\n\nBody.\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	res, err := s.Run(context.Background(), repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found["markdown"] != 2 {
		t.Fatalf("Found markdown: want 2, got %d", res.Found["markdown"])
	}
	var mdRow *SourceBreakdownRow
	for i := range res.SourcesBreakdown {
		if res.SourcesBreakdown[i].SourceType == "markdown" {
			mdRow = &res.SourcesBreakdown[i]
			break
		}
	}
	if mdRow == nil {
		t.Fatal("no markdown breakdown row")
	}
	if mdRow.Count != 2 {
		t.Fatalf("markdown count: want 2, got %d", mdRow.Count)
	}
	g := mdRow.Formats[format.ProfileGeneric]
	c := mdRow.Formats[format.ProfileCursorPlan]
	if g != 1 || c != 1 {
		t.Fatalf("expected generic=1 and cursor_plan=1, got formats %#v", mdRow.Formats)
	}
}
