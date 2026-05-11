package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	tmp := t.TempDir()
	db, err := Open(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seedArtifact(t *testing.T, db *DB) (repoID, artifactID, revID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	repoID = "repo_001"
	artifactID = "ds_ARTIFACT001"
	revID = "rev_001"

	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, '/tmp/repo', ?, ?)", repoID, now, now)
	db.InsertArtifactDirect(artifactID, repoID, "plan", "Test Plan", "draft", revID, now)
	db.InsertRevisionDirect(revID, artifactID, "sha256:abc", "# Test Plan\n\nBody.", "", now)
	db.InsertSourceDirect("src_001", artifactID, repoID, "markdown", "plans/test.md", "plans/test.md|markdown", "", "", now)
	return
}

func TestListArtifacts_Empty(t *testing.T) {
	db := openTestDB(t)
	arts, err := db.ListArtifacts(FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(arts))
	}
}

func TestListArtifacts_All(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].Title != "Test Plan" {
		t.Errorf("title: want 'Test Plan', got %q", arts[0].Title)
	}
}

func TestListArtifacts_FilterByKind(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, _ := db.ListArtifacts(FilterParams{Kind: "plan"})
	if len(arts) != 1 {
		t.Errorf("expected 1 plan, got %d", len(arts))
	}
	arts, _ = db.ListArtifacts(FilterParams{Kind: "adr"})
	if len(arts) != 0 {
		t.Errorf("expected 0 adrs, got %d", len(arts))
	}
}

func TestListArtifacts_FilterByStatus(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, _ := db.ListArtifacts(FilterParams{Status: "draft"})
	if len(arts) != 1 {
		t.Errorf("expected 1 draft, got %d", len(arts))
	}
	arts, _ = db.ListArtifacts(FilterParams{Status: "approved"})
	if len(arts) != 0 {
		t.Errorf("expected 0 approved, got %d", len(arts))
	}
}

func TestListArtifacts_FilterBySourceType(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, _ := db.ListArtifacts(FilterParams{SourceType: "markdown"})
	if len(arts) != 1 {
		t.Errorf("expected 1 markdown, got %d", len(arts))
	}
	arts, _ = db.ListArtifacts(FilterParams{SourceType: "openspec"})
	if len(arts) != 0 {
		t.Errorf("expected 0 openspec, got %d", len(arts))
	}
}

func TestGetArtifact_ExactID(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	art, err := db.GetArtifact(artID)
	if err != nil {
		t.Fatal(err)
	}
	if art.ID != artID {
		t.Errorf("want %q, got %q", artID, art.ID)
	}
}

func TestGetArtifact_PrefixMatch(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	art, err := db.GetArtifact("ds_ARTIFACT")
	if err != nil {
		t.Fatal(err)
	}
	if art.ID != artID {
		t.Errorf("prefix match failed: want %q, got %q", artID, art.ID)
	}
}

func TestGetArtifact_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetArtifact("ds_NONEXISTENT")
	if err == nil {
		t.Error("expected error for non-existent artifact")
	}
}

func TestGetArtifact_AmbiguousPrefix(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_001"
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, '/tmp', ?, ?)", repoID, now, now)
	db.InsertArtifactDirect("ds_ABC001", repoID, "plan", "A", "draft", "rev_a", now)
	db.InsertArtifactDirect("ds_ABC002", repoID, "plan", "B", "draft", "rev_b", now)

	_, err := db.GetArtifact("ds_ABC")
	if err == nil {
		t.Error("expected ambiguity error")
	}
	if err != nil && !contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' in error, got %q", err.Error())
	}
}

func TestGetRevision(t *testing.T) {
	db := openTestDB(t)
	_, _, revID := seedArtifact(t, db)

	rev, err := db.GetRevision(revID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.ContentHash != "sha256:abc" {
		t.Errorf("hash: want 'sha256:abc', got %q", rev.ContentHash)
	}
	if rev.Body != "# Test Plan\n\nBody." {
		t.Errorf("body mismatch: got %q", rev.Body)
	}
}

func TestGetRevision_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetRevision("rev_nonexist")
	if err == nil {
		t.Error("expected error for non-existent revision")
	}
}

func TestGetSourcesForArtifact(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	sources, err := db.GetSourcesForArtifact(artID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Path != "plans/test.md" {
		t.Errorf("path: want 'plans/test.md', got %q", sources[0].Path)
	}
}

func TestGetLinksForArtifact_Empty(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	links, err := db.GetLinksForArtifact(artID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links, got %d", len(links))
	}
}

func TestInsertLink_And_GetLinks(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	err := db.InsertLink("link_001", artID, "implements", "https://github.com/acme/pr/1", now)
	if err != nil {
		t.Fatal(err)
	}

	links, _ := db.GetLinksForArtifact(artID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].LinkType != "implements" {
		t.Errorf("type: want 'implements', got %q", links[0].LinkType)
	}
	if links[0].Target != "https://github.com/acme/pr/1" {
		t.Errorf("target: want 'https://github.com/acme/pr/1', got %q", links[0].Target)
	}
}

func TestGetTodosForArtifact(t *testing.T) {
	db := openTestDB(t)
	_, artID, revID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 0, 'First', 0, 'test.md', 3, ?)", "todo_1", artID, revID, now)
	db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 1, 'Second', 1, 'test.md', 4, ?)", "todo_2", artID, revID, now)

	todos, err := db.GetTodosForArtifact(artID)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(todos))
	}
	if todos[0].Text != "First" || todos[0].Done {
		t.Errorf("todo 0: %+v", todos[0])
	}
	if todos[1].Text != "Second" || !todos[1].Done {
		t.Errorf("todo 1: %+v", todos[1])
	}
}

func TestListAllTodos_Filters(t *testing.T) {
	db := openTestDB(t)
	_, artID, revID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 0, 'Open', 0, 't.md', 1, ?)", "td_1", artID, revID, now)
	db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 1, 'Done', 1, 't.md', 2, ?)", "td_2", artID, revID, now)

	all, _ := db.ListAllTodos(FilterParams{}, false, false)
	if len(all) != 2 {
		t.Errorf("all: expected 2, got %d", len(all))
	}

	open, _ := db.ListAllTodos(FilterParams{}, true, false)
	if len(open) != 1 || open[0].Text != "Open" {
		t.Errorf("open: expected 1 'Open', got %+v", open)
	}

	done, _ := db.ListAllTodos(FilterParams{}, false, true)
	if len(done) != 1 || done[0].Text != "Done" {
		t.Errorf("done: expected 1 'Done', got %+v", done)
	}
}

func TestFindArtifacts(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.FindArtifacts("Test", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Errorf("expected 1 match for 'Test', got %d", len(arts))
	}

	arts, _ = db.FindArtifacts("nonexistent", FilterParams{})
	if len(arts) != 0 {
		t.Errorf("expected 0 matches, got %d", len(arts))
	}
}

func TestFindArtifacts_FilterByKind(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, _ := db.FindArtifacts("Test", FilterParams{Kind: "plan"})
	if len(arts) != 1 {
		t.Errorf("expected 1, got %d", len(arts))
	}
	arts, _ = db.FindArtifacts("Test", FilterParams{Kind: "adr"})
	if len(arts) != 0 {
		t.Errorf("expected 0, got %d", len(arts))
	}
}

func TestUpdateArtifactStatus(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	err := db.UpdateArtifactStatus(artID, "approved", now)
	if err != nil {
		t.Fatal(err)
	}

	art, _ := db.GetArtifact(artID)
	if art.Status != "approved" {
		t.Errorf("status: want 'approved', got %q", art.Status)
	}
}

func TestFindSourceByIdentity(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	artID, err := db.FindSourceByIdentity("plans/test.md|markdown")
	if err != nil {
		t.Fatal(err)
	}
	if artID != "ds_ARTIFACT001" {
		t.Errorf("want 'ds_ARTIFACT001', got %q", artID)
	}

	artID, err = db.FindSourceByIdentity("nonexistent|x")
	if err != nil {
		t.Fatal(err)
	}
	if artID != "" {
		t.Errorf("expected empty for missing identity, got %q", artID)
	}
}

func TestEnsureRepo_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.EnsureRepo("/nonexistent", "now")
	if err == nil {
		t.Error("expected error for non-existent repo")
	}
}

func TestInsertArtifactDirect_And_Retrieve(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)

	err := db.InsertArtifactDirect("ds_X", "r1", "spec", "My Spec", "proposed", "rev_x", now)
	if err != nil {
		t.Fatal(err)
	}
	art, _ := db.GetArtifact("ds_X")
	if art.Kind != "spec" || art.Title != "My Spec" || art.Status != "proposed" {
		t.Errorf("artifact mismatch: %+v", art)
	}
}

func TestInsertRevisionDirect(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_A", "r1", "plan", "P", "draft", "rev_1", now)

	err := db.InsertRevisionDirect("rev_1", "ds_A", "sha256:xyz", "body content", "", now)
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := db.GetRevision("rev_1")
	if rev.Body != "body content" {
		t.Errorf("body: got %q", rev.Body)
	}
}

func TestInsertRevisionDirect_ExtractedJSON(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_A", "r1", "plan", "P", "draft", "rev_1", now)
	payload := `{"generator":"x"}`
	err := db.InsertRevisionDirect("rev_1", "ds_A", "sha256:xyz", "body", payload, now)
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := db.GetRevision("rev_1")
	if rev.ExtractedJSON != payload {
		t.Errorf("extracted_json: got %q want %q", rev.ExtractedJSON, payload)
	}
}

func TestInsertSourceDirect(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_A", "r1", "plan", "P", "draft", "rev_1", now)

	err := db.InsertSourceDirect("src_x", "ds_A", "r1", "markdown", "plans/x.md", "plans/x.md|markdown", "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := db.GetSourcesForArtifact("ds_A")
	if len(sources) != 1 || sources[0].Path != "plans/x.md" || sources[0].FormatProfile != "generic" {
		t.Errorf("source mismatch: %+v", sources)
	}
}

func TestListArtifacts_FilterByRepoRoot(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/repo/a', ?, ?)", now, now)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r2', '/repo/b', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_A1", "r1", "plan", "Plan A", "draft", "rev_a1", now)
	db.InsertArtifactDirect("ds_B1", "r2", "spec", "Spec B", "proposed", "rev_b1", now)

	arts, _ := db.ListArtifacts(FilterParams{RepoRoot: "/repo/a"})
	if len(arts) != 1 || arts[0].ID != "ds_A1" {
		t.Errorf("filter by repo root: got %+v", arts)
	}
}

func TestListAllTodos_FilterByRepoRoot(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/repo/x', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_X1", "r1", "plan", "X Plan", "draft", "rev_x1", now)
	db.InsertRevisionDirect("rev_x1", "ds_X1", "sha256:x", "body", "", now)
	db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('td_x', 'ds_X1', 'rev_x1', 0, 'X Todo', 0, 'x.md', 1, ?)", now)

	todos, _ := db.ListAllTodos(FilterParams{RepoRoot: "/repo/x"}, false, false)
	if len(todos) != 1 {
		t.Errorf("expected 1 todo for /repo/x, got %d", len(todos))
	}
	todos, _ = db.ListAllTodos(FilterParams{RepoRoot: "/repo/other"}, false, false)
	if len(todos) != 0 {
		t.Errorf("expected 0 todos for /repo/other, got %d", len(todos))
	}
}

func TestEnsureRepo_Exists(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/existing', ?, ?)", now, now)

	id, err := db.EnsureRepo("/existing", now)
	if err != nil {
		t.Fatal(err)
	}
	if id != "r1" {
		t.Errorf("want 'r1', got %q", id)
	}
}

func TestFindArtifacts_ByBodyContent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_BODY1", "r1", "plan", "Simple Title", "draft", "rev_body1", now)
	db.InsertRevisionDirect("rev_body1", "ds_BODY1", "sha256:b1", "This contains searchable-keyword in body.", "", now)

	arts, _ := db.FindArtifacts("searchable-keyword", FilterParams{})
	if len(arts) != 1 {
		t.Errorf("expected 1 match by body, got %d", len(arts))
	}
}

func TestFindArtifacts_BySourcePath(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_PATH1", "r1", "spec", "Spec", "draft", "rev_p1", now)
	db.InsertSourceDirect("src_p1", "ds_PATH1", "r1", "markdown", "docs/unique-path.md", "docs/unique-path.md|markdown", "", "", now)

	arts, _ := db.FindArtifacts("unique-path", FilterParams{})
	if len(arts) != 1 {
		t.Errorf("expected 1 match by path, got %d", len(arts))
	}
}

func TestFTS5_FallbackParity(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_FTS1", "r1", "plan", "Architecture Plan", "draft", "rev_fts1", now)
	db.InsertRevisionDirect("rev_fts1", "ds_FTS1", "sha256:f1", "This is the body of our architecture plan.", "", now)
	db.InsertSourceDirect("src_fts1", "ds_FTS1", "r1", "markdown", "plans/architecture.md", "plans/architecture.md|markdown", "", "", now)

	// Index in FTS
	db.IndexArtifactFTS("ds_FTS1", "Architecture Plan", "This is the body of our architecture plan.", "plans/architecture.md")

	// FTS search
	ftsResults, err := db.findArtifactsFTS("Architecture", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}

	// LIKE search
	likeResults, err := db.findArtifactsLIKE("Architecture", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}

	if len(ftsResults) != len(likeResults) {
		t.Errorf("parity failure: FTS returned %d, LIKE returned %d", len(ftsResults), len(likeResults))
	}
	if len(ftsResults) == 0 {
		t.Error("expected at least 1 result from both")
	}
}

func TestIndexArtifactFTS(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_I1", "r1", "spec", "API Spec", "proposed", "rev_i1", now)

	err := db.IndexArtifactFTS("ds_I1", "API Spec", "REST API definition", "specs/api.md")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's searchable
	results, err := db.findArtifactsFTS("REST", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 FTS result, got %d", len(results))
	}

	// Update FTS (re-index)
	err = db.IndexArtifactFTS("ds_I1", "API Spec Updated", "GraphQL definition", "specs/api.md")
	if err != nil {
		t.Fatal(err)
	}
	results, _ = db.findArtifactsFTS("GraphQL", FilterParams{})
	if len(results) != 1 {
		t.Errorf("expected 1 result after re-index, got %d", len(results))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestGetArtifact_ShortID(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_FULL001", "r1", "plan", "Short ID Test", "draft", "rev_s1", now)
	db.Exec("UPDATE artifacts SET short_id = 'ab12cd34' WHERE id = 'ds_FULL001'")

	art, err := db.GetArtifact("ab12cd34")
	if err != nil {
		t.Fatal(err)
	}
	if art.ID != "ds_FULL001" {
		t.Errorf("short_id lookup failed: got %q", art.ID)
	}
	if art.ShortID != "ab12cd34" {
		t.Errorf("short_id field: want 'ab12cd34', got %q", art.ShortID)
	}
}

func TestGetArtifact_FullIDStillWorks(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_FULL002", "r1", "spec", "Full ID Test", "draft", "rev_s2", now)
	db.Exec("UPDATE artifacts SET short_id = 'ef56gh78' WHERE id = 'ds_FULL002'")

	art, err := db.GetArtifact("ds_FULL002")
	if err != nil {
		t.Fatal(err)
	}
	if art.ID != "ds_FULL002" {
		t.Errorf("full ID lookup failed: got %q", art.ID)
	}
}

func TestTagCRUD(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_TAG1", "r1", "plan", "Tagged", "draft", "rev_t1", now)

	// Insert tags
	db.InsertTag("ds_TAG1", "auth", "frontmatter", now)
	db.InsertTag("ds_TAG1", "v2", "manual", now)
	db.InsertTag("ds_TAG1", "inferred-dir", "inferred", now)

	tags, err := db.GetTagsForArtifact("ds_TAG1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}

	// Delete manual tag
	db.DeleteTag("ds_TAG1", "v2")
	tags, _ = db.GetTagsForArtifact("ds_TAG1")
	if len(tags) != 2 {
		t.Errorf("expected 2 tags after delete, got %d", len(tags))
	}

	// Delete auto tags (frontmatter + inferred)
	db.DeleteAutoTags("ds_TAG1")
	tags, _ = db.GetTagsForArtifact("ds_TAG1")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags after DeleteAutoTags, got %d", len(tags))
	}
}

func TestInsertTag_DuplicateIsNoOp(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_DUP1", "r1", "plan", "Dup", "draft", "rev_d1", now)

	err := db.InsertTag("ds_DUP1", "auth", "manual", now)
	if err != nil {
		t.Fatal(err)
	}
	// Second insert should be no-op
	err = db.InsertTag("ds_DUP1", "auth", "manual", now)
	if err != nil {
		t.Fatal(err)
	}

	tags, _ := db.GetTagsForArtifact("ds_DUP1")
	if len(tags) != 1 {
		t.Errorf("expected 1 tag (no duplicate), got %d", len(tags))
	}
}

func TestListArtifacts_FilterByTag(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_T1", "r1", "plan", "Auth Plan", "draft", "rev_1", now)
	db.InsertArtifactDirect("ds_T2", "r1", "spec", "Other Spec", "draft", "rev_2", now)
	db.InsertTag("ds_T1", "auth", "manual", now)

	arts, _ := db.ListArtifacts(FilterParams{Tag: "auth"})
	if len(arts) != 1 || arts[0].ID != "ds_T1" {
		t.Errorf("expected 1 artifact with tag auth, got %+v", arts)
	}

	arts, _ = db.ListArtifacts(FilterParams{Tag: "nonexistent"})
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts for nonexistent tag, got %d", len(arts))
	}
}

func TestListArtifacts_FilterByBranch(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, git_current_branch, created_at, updated_at) VALUES ('r1', '/tmp', 'main', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_B1", "r1", "plan", "Main Plan", "draft", "rev_1", now)

	arts, _ := db.ListArtifacts(FilterParams{Branch: "main"})
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact on branch main, got %d", len(arts))
	}

	arts, _ = db.ListArtifacts(FilterParams{Branch: "feature"})
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts on branch feature, got %d", len(arts))
	}
}

func TestListArtifacts_FilterByUser(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, scanned_by, created_at, updated_at) VALUES ('r1', '/tmp', 'brenn', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_U1", "r1", "plan", "User Plan", "draft", "rev_1", now)

	arts, _ := db.ListArtifacts(FilterParams{User: "brenn"})
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact for user brenn, got %d", len(arts))
	}

	arts, _ = db.ListArtifacts(FilterParams{User: "other"})
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts for user other, got %d", len(arts))
	}
}

func TestResumeArtifacts(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_R1", "r1", "plan", "Resume Plan", "draft", "rev_1", now)
	db.Exec("UPDATE artifacts SET short_id = 'abc12345' WHERE id = 'ds_R1'")
	db.Exec("INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('src_r1', 'ds_R1', 'r1', 'markdown', 'plans/x.md', 'plans/x.md|markdown', 'generic', NULL, ?, ?)", now, now)
	db.InsertTag("ds_R1", "beta", "manual", now)
	db.InsertTag("ds_R1", "auth", "manual", now)

	rows, err := db.ResumeArtifacts("/tmp/repo", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 resume row, got %d", len(rows))
	}
	if rows[0].ShortID != "abc12345" {
		t.Errorf("short_id: want 'abc12345', got %q", rows[0].ShortID)
	}
	if rows[0].SourcePath != "plans/x.md" {
		t.Errorf("source path: want 'plans/x.md', got %q", rows[0].SourcePath)
	}
	if rows[0].TagsJoined != "auth, beta" {
		t.Errorf("tags: want 'auth, beta', got %q", rows[0].TagsJoined)
	}
}

func TestResumeArtifacts_DeduplicatesMultipleSources(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo2', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_MDUP", "r1", "plan", "Dup Sources", "draft", "rev_md", now)
	db.Exec("INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('s1', 'ds_MDUP', 'r1', 'markdown', 'z-last.md', 'z|md', 'generic', NULL, ?, ?)", now, now)
	db.Exec("INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('s2', 'ds_MDUP', 'r1', 'markdown', 'a-first.md', 'a|md', 'generic', NULL, ?, ?)", now, now)

	rows, err := db.ResumeArtifacts("/tmp/repo2", FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (deduped), got %d", len(rows))
	}
	if rows[0].SourcePath != "a-first.md" {
		t.Errorf("MIN(path): want 'a-first.md', got %q", rows[0].SourcePath)
	}
}

func TestAssignArtifactShortID_CollisionUsesSuffix(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_COLA", "r1", "plan", "A", "draft", "rev_a", now)
	db.InsertArtifactDirect("ds_COLB", "r1", "plan", "B", "draft", "rev_b", now)
	db.UpdateArtifactShortID("ds_COLA", "deadbeef")

	err := db.AssignArtifactShortID("ds_COLB", "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	artB, err := db.GetArtifact("ds_COLB")
	if err != nil {
		t.Fatal(err)
	}
	if artB.ShortID != "deadbeef1" {
		t.Errorf("after collision want short_id deadbeef1, got %q", artB.ShortID)
	}
}

func TestAssignArtifactShortID_SecondCollisionUsesSuffix2(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_X1", "r1", "plan", "X1", "draft", "rev_1", now)
	db.InsertArtifactDirect("ds_X2", "r1", "plan", "X2", "draft", "rev_2", now)
	db.InsertArtifactDirect("ds_X3", "r1", "plan", "X3", "draft", "rev_3", now)
	db.UpdateArtifactShortID("ds_X1", "cafebabe")
	db.UpdateArtifactShortID("ds_X2", "cafebabe1")

	err := db.AssignArtifactShortID("ds_X3", "cafebabe")
	if err != nil {
		t.Fatal(err)
	}
	art, _ := db.GetArtifact("ds_X3")
	if art.ShortID != "cafebabe2" {
		t.Errorf("want cafebabe2, got %q", art.ShortID)
	}
}

func TestUpdateArtifactShortID(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	db.InsertArtifactDirect("ds_SID1", "r1", "plan", "SID Test", "draft", "rev_1", now)

	err := db.UpdateArtifactShortID("ds_SID1", "deadbeef")
	if err != nil {
		t.Fatal(err)
	}

	art, _ := db.GetArtifact("deadbeef")
	if art == nil || art.ID != "ds_SID1" {
		t.Error("UpdateArtifactShortID failed")
	}
}

func TestUpdateScanMeta_WithUser(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/x', ?, ?)", now, now)

	db.UpdateScanMeta("r1", "abc123", "brenn", now)

	meta := db.GetRepoByRoot("/tmp/x")
	if meta == nil {
		t.Fatal("expected repo meta")
	}
	if meta.ScannedBy != "brenn" {
		t.Errorf("scanned_by: want 'brenn', got %q", meta.ScannedBy)
	}
	if meta.LastScanCommit != "abc123" {
		t.Errorf("last_scan_commit: want 'abc123', got %q", meta.LastScanCommit)
	}
}
