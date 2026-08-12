package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	tmp := t.TempDir()
	db, err := Open(filepath.Join(tmp, "test.db"))
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func seedArtifact(t *testing.T, db *DB) (repoID, artifactID, revID string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	repoID = "repo_001"
	artifactID = "ds_ARTIFACT001"
	revID = "rev_001"

	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, '/tmp/repo', ?, ?)", repoID, now, now)
	require.NoError(t, db.InsertArtifactDirect(artifactID, repoID, "plan", "", "Test Plan", "draft", revID, now, now))
	require.NoError(t, db.InsertRevisionDirect(revID, artifactID, "sha256:abc", "# Test Plan\n\nBody.", "", now))
	require.NoError(t, db.InsertSourceDirect("src_001", artifactID, repoID, "markdown", "plans/test.md", "plans/test.md|markdown", "", "", now))
	return
}

func TestListArtifacts_Empty(t *testing.T) {
	db := openTestDB(t)
	arts, err := db.ListArtifacts(FilterParams{})
	require.NoError(t, err)
	assert.Empty(t, arts,
		"expected 0 artifacts, got %d", len(arts))

}

func TestListArtifacts_All(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{})
	require.NoError(t, err)
	require.Len(t, arts, 1,
		"expected 1 artifact, got %d", len(arts))
	assert.Equal(t, "Test Plan", arts[0].Title,
		"title: want 'Test Plan', got %q", arts[0].Title)

}

func TestListArtifacts_WithMatchingKind_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{Kind: "plan"})
	require.NoError(t, err)

	require.Len(t, arts, 1)
	assert.Equal(t, "ds_ARTIFACT001", arts[0].ID)
}

func TestListArtifacts_WithUnknownKind_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{Kind: "adr"})
	require.NoError(t, err)

	assert.Empty(t, arts)
}

func TestListArtifacts_WithMatchingStatus_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{Status: "draft"})
	require.NoError(t, err)

	require.Len(t, arts, 1)
	assert.Equal(t, "ds_ARTIFACT001", arts[0].ID)
}

func TestListArtifacts_WithUnknownStatus_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{Status: "approved"})
	require.NoError(t, err)

	assert.Empty(t, arts)
}

func TestListArtifacts_WithMatchingSourceType_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{SourceType: "markdown"})
	require.NoError(t, err)

	require.Len(t, arts, 1)
	assert.Equal(t, "ds_ARTIFACT001", arts[0].ID)
}

func TestListArtifacts_WithUnknownSourceType_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.ListArtifacts(FilterParams{SourceType: "openspec"})
	require.NoError(t, err)

	assert.Empty(t, arts)
}

func TestGetArtifact_ExactID(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	art, err := db.GetArtifact(artID)
	require.NoError(t, err)
	assert.Equal(t, artID, art.ID,
		"want %q, got %q", artID, art.ID)

}

func TestGetArtifact_PrefixMatch(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	art, err := db.GetArtifact("ds_ARTIFACT")
	require.NoError(t, err)
	assert.Equal(t, artID, art.ID,
		"prefix match failed: want %q, got %q", artID, art.ID)

}

func TestGetArtifact_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetArtifact("ds_NONEXISTENT")
	assert.Error(t, err,
		"expected error for non-existent artifact")

}

func TestGetArtifact_AmbiguousPrefix(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_001"
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, '/tmp', ?, ?)", repoID, now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_ABC001", repoID, "plan", "", "A", "draft", "rev_a", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_ABC002", repoID, "plan", "", "B", "draft", "rev_b", now, now))

	_, err := db.GetArtifact("ds_ABC")
	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous")

}

func TestGetRevision(t *testing.T) {
	db := openTestDB(t)
	_, _, revID := seedArtifact(t, db)

	rev, err := db.GetRevision(revID)
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc", rev.ContentHash,
		"hash: want 'sha256:abc', got %q", rev.ContentHash)
	assert.Equal(t, "# Test Plan\n\nBody.", rev.Body,
		"body mismatch: got %q", rev.Body)

}

func TestGetRevision_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetRevision("rev_nonexist")
	assert.Error(t, err,
		"expected error for non-existent revision")

}

func TestGetSourcesForArtifact(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	sources, err := db.GetSourcesForArtifact(artID)
	require.NoError(t, err)
	require.Len(t, sources, 1,
		"expected 1 source, got %d", len(sources))
	assert.Equal(t, "plans/test.md", sources[0].Path,
		"path: want 'plans/test.md', got %q", sources[0].Path)

}

func TestGetLinksForArtifact_Empty(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)

	links, err := db.GetLinksForArtifact(artID)
	require.NoError(t, err)
	assert.Empty(t, links,
		"expected 0 links, got %d", len(links))

}

func TestInsertLink_And_GetLinks(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	err := db.InsertLink("link_001", artID, "implements", "https://github.com/acme/pr/1", now)
	require.NoError(t, err)

	links, err := db.GetLinksForArtifact(artID)
	require.NoError(t, err)
	require.Len(t, links, 1,
		"expected 1 link, got %d", len(links))
	assert.Equal(t, "implements", links[0].LinkType,
		"type: want 'implements', got %q", links[0].LinkType)
	assert.Equal(t, "https://github.com/acme/pr/1", links[0].Target,
		"target: want 'https://github.com/acme/pr/1', got %q", links[0].Target)

}

func TestGetTodosForArtifact(t *testing.T) {
	db := openTestDB(t)
	_, artID, revID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	mustExecStoreTestSQL(t, db, "INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 0, 'First', 0, 'test.md', 3, ?)", "todo_1", artID, revID, now)
	mustExecStoreTestSQL(t, db, "INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 1, 'Second', 1, 'test.md', 4, ?)", "todo_2", artID, revID, now)

	todos, err := db.GetTodosForArtifact(artID)
	require.NoError(t, err)
	require.Len(t, todos, 2,
		"expected 2 todos, got %d", len(todos))
	assert.Equal(t, "First", todos[0].Text)
	assert.False(t, todos[0].Done)
	assert.Equal(t, "Second", todos[1].Text)
	assert.True(t, todos[1].Done)

}

func TestListAllTodos_WithoutStatusFilter_ReturnsAllTodos(t *testing.T) {
	db := openTestDB(t)
	seedTodoFilters(t, db)

	todos, err := db.ListAllTodos(FilterParams{}, false, false)

	require.NoError(t, err)
	require.Len(t, todos, 2)
	assert.Equal(t, "Open", todos[0].Text)
	assert.Equal(t, "Done", todos[1].Text)
}

func TestListAllTodos_WithOpenFilter_ReturnsOpenTodo(t *testing.T) {
	db := openTestDB(t)
	seedTodoFilters(t, db)

	todos, err := db.ListAllTodos(FilterParams{}, true, false)

	require.NoError(t, err)
	require.Len(t, todos, 1)
	assert.Equal(t, "Open", todos[0].Text)
}

func TestListAllTodos_WithDoneFilter_ReturnsDoneTodo(t *testing.T) {
	db := openTestDB(t)
	seedTodoFilters(t, db)

	todos, err := db.ListAllTodos(FilterParams{}, false, true)

	require.NoError(t, err)
	require.Len(t, todos, 1)
	assert.Equal(t, "Done", todos[0].Text)
}

func seedTodoFilters(t *testing.T, db *DB) {
	t.Helper()
	_, artID, revID := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 0, 'Open', 0, 't.md', 1, ?)", "td_1", artID, revID, now)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, 1, 'Done', 1, 't.md', 2, ?)", "td_2", artID, revID, now)
	require.NoError(t, err)
}

func TestFindArtifacts_WithMatchingQuery_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.FindArtifacts("Test", FilterParams{})
	require.NoError(t, err)

	require.Len(t, arts, 1)
	assert.Equal(t, "ds_ARTIFACT001", arts[0].ID)
}

func TestFindArtifacts_WithUnknownQuery_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.FindArtifacts("nonexistent", FilterParams{})
	require.NoError(t, err)

	assert.Empty(t, arts)
}

func TestFindArtifacts_WithMatchingKind_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.FindArtifacts("Test", FilterParams{Kind: "plan"})
	require.NoError(t, err)

	require.Len(t, arts, 1)
	assert.Equal(t, "ds_ARTIFACT001", arts[0].ID)
}

func TestFindArtifacts_WithUnknownKind_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	arts, err := db.FindArtifacts("Test", FilterParams{Kind: "adr"})
	require.NoError(t, err)

	assert.Empty(t, arts)
}

func TestUpdateArtifactStatus(t *testing.T) {
	db := openTestDB(t)
	_, artID, _ := seedArtifact(t, db)
	now := time.Now().UTC().Format(time.RFC3339)

	err := db.UpdateArtifactStatus(artID, "approved", now)
	require.NoError(t, err)

	art, err := db.GetArtifact(artID)
	require.NoError(t, err)
	assert.Equal(t, "approved", art.Status,
		"status: want 'approved', got %q", art.Status)

}

func TestFindSourceByIdentity_WithExistingIdentity_ReturnsArtifactID(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	artID, err := db.FindSourceByIdentity("plans/test.md|markdown")
	require.NoError(t, err)

	assert.Equal(t, "ds_ARTIFACT001", artID)
}

func TestFindSourceByIdentity_WithUnknownIdentity_ReturnsEmptyID(t *testing.T) {
	db := openTestDB(t)
	seedArtifact(t, db)

	artID, err := db.FindSourceByIdentity("nonexistent|x")
	require.NoError(t, err)

	assert.Empty(t, artID)
}

func TestEnsureRepo_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.EnsureRepo("/nonexistent", "now")
	assert.Error(t, err,
		"expected error for non-existent repo")

}

func TestInsertArtifactDirect_And_Retrieve(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)

	err := db.InsertArtifactDirect("ds_X", "r1", "spec", "", "My Spec", "proposed", "rev_x", now, now)
	require.NoError(t, err)

	art, err := db.GetArtifact("ds_X")
	require.NoError(t, err)
	assert.Equal(t, "spec", art.Kind)
	assert.Equal(t, "My Spec", art.Title)
	assert.Equal(t, "proposed", art.Status)

}

func TestInsertRevisionDirect(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_A", "r1", "plan", "", "P", "draft", "rev_1", now, now))

	err := db.InsertRevisionDirect("rev_1", "ds_A", "sha256:xyz", "body content", "", now)
	require.NoError(t, err)

	rev, err := db.GetRevision("rev_1")
	require.NoError(t, err)
	assert.Equal(t, "body content", rev.Body,
		"body: got %q", rev.Body)

}

func TestInsertRevisionDirect_ExtractedJSON(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_A", "r1", "plan", "", "P", "draft", "rev_1", now, now))
	payload := `{"generator":"x"}`
	err := db.InsertRevisionDirect("rev_1", "ds_A", "sha256:xyz", "body", payload, now)
	require.NoError(t, err)

	rev, err := db.GetRevision("rev_1")
	require.NoError(t, err)
	assert.Equal(t, payload, rev.ExtractedJSON,
		"extracted_json: got %q want %q", rev.ExtractedJSON, payload)

}

func TestInsertSourceDirect(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/x', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_A", "r1", "plan", "", "P", "draft", "rev_1", now, now))

	err := db.InsertSourceDirect("src_x", "ds_A", "r1", "markdown", "plans/x.md", "plans/x.md|markdown", "", "", now)
	require.NoError(t, err)

	sources, err := db.GetSourcesForArtifact("ds_A")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, "plans/x.md", sources[0].Path)
	assert.Equal(t, "generic", sources[0].FormatProfile)

}

func TestListArtifacts_FilterByRepoRoot(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)

	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/repo/a', ?, ?)", now, now)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r2', '/repo/b', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_A1", "r1", "plan", "", "Plan A", "draft", "rev_a1", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_B1", "r2", "spec", "", "Spec B", "proposed", "rev_b1", now, now))

	arts, err := db.ListArtifacts(FilterParams{RepoRoot: "/repo/a"})
	require.NoError(t, err)
	require.Len(t, arts, 1)
	assert.Equal(t, "ds_A1", arts[0].ID)

}

func TestListAllTodos_WithMatchingRepoRoot_ReturnsTodo(t *testing.T) {
	db := openTestDB(t)
	seedRepoTodo(t, db)

	todos, err := db.ListAllTodos(FilterParams{RepoRoot: "/repo/x"}, false, false)
	require.NoError(t, err)

	require.Len(t, todos, 1)
	assert.Equal(t, "td_x", todos[0].ID)
}

func TestListAllTodos_WithUnknownRepoRoot_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	seedRepoTodo(t, db)

	todos, err := db.ListAllTodos(FilterParams{RepoRoot: "/repo/other"}, false, false)
	require.NoError(t, err)

	assert.Empty(t, todos)
}

func seedRepoTodo(t *testing.T, db *DB) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/repo/x', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_X1", "r1", "plan", "", "X Plan", "draft", "rev_x1", now, now))
	require.NoError(t, db.InsertRevisionDirect("rev_x1", "ds_X1", "sha256:x", "body", "", now))
	mustExecStoreTestSQL(t, db, "INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('td_x', 'ds_X1', 'rev_x1', 0, 'X Todo', 0, 'x.md', 1, ?)", now)
}

func TestEnsureRepo_Exists(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/existing', ?, ?)", now, now)

	id, err := db.EnsureRepo("/existing", now)
	require.NoError(t, err)
	assert.Equal(t, "r1", id,
		"want 'r1', got %q", id)

}

func TestFindArtifacts_ByBodyContent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_BODY1", "r1", "plan", "", "Simple Title", "draft", "rev_body1", now, now))
	require.NoError(t, db.InsertRevisionDirect("rev_body1", "ds_BODY1", "sha256:b1", "This contains searchable-keyword in body.", "", now))

	arts, err := db.FindArtifacts("searchable-keyword", FilterParams{})
	require.NoError(t, err)
	assert.Len(t, arts, 1,
		"expected 1 match by body, got %d", len(arts))

}

func TestFindArtifacts_BySourcePath(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_PATH1", "r1", "spec", "", "Spec", "draft", "rev_p1", now, now))
	require.NoError(t, db.InsertSourceDirect("src_p1", "ds_PATH1", "r1", "markdown", "docs/unique-path.md", "docs/unique-path.md|markdown", "", "", now))

	arts, err := db.FindArtifacts("unique-path", FilterParams{})
	require.NoError(t, err)
	assert.Len(t, arts, 1,
		"expected 1 match by path, got %d", len(arts))

}

func TestFindArtifactsFTS_WithIndexedArtifact_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedFTSArtifact(t, db)

	results, err := db.findArtifactsFTS("Architecture", FilterParams{})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "ds_FTS1", results[0].ID)
}

func TestFindArtifactsLIKE_WithIndexedArtifact_ReturnsArtifact(t *testing.T) {
	db := openTestDB(t)
	seedFTSArtifact(t, db)

	results, err := db.findArtifactsLIKE("Architecture", FilterParams{})
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "ds_FTS1", results[0].ID)
}

func TestIndexArtifactFTS_WithNewRow_MakesArtifactSearchable(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_I1", "r1", "spec", "", "API Spec", "proposed", "rev_i1", now, now))

	err := db.IndexArtifactFTS("ds_I1", "API Spec", "REST API definition", "specs/api.md")
	require.NoError(t, err)

	results, err := db.findArtifactsFTS("REST", FilterParams{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "ds_I1", results[0].ID)
}

func TestIndexArtifactFTS_WithExistingRow_ReplacesSearchableContent(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_I1", "r1", "spec", "", "API Spec", "proposed", "rev_i1", now, now))
	mustExecStoreTestSQL(t, db, `INSERT INTO artifacts_fts (artifact_id, title, body, source_path) VALUES (?, ?, ?, ?)`, "ds_I1", "Old API Spec", "REST API definition", "specs/api.md")

	err := db.IndexArtifactFTS("ds_I1", "API Spec Updated", "GraphQL definition", "specs/api.md")
	require.NoError(t, err)

	results, err := db.findArtifactsFTS("GraphQL", FilterParams{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "ds_I1", results[0].ID)
}

func seedFTSArtifact(t *testing.T, db *DB) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_FTS1", "r1", "plan", "", "Architecture Plan", "draft", "rev_fts1", now, now))
	require.NoError(t, db.InsertRevisionDirect("rev_fts1", "ds_FTS1", "sha256:f1", "This is the body of our architecture plan.", "", now))
	require.NoError(t, db.InsertSourceDirect("src_fts1", "ds_FTS1", "r1", "markdown", "plans/architecture.md", "plans/architecture.md|markdown", "", "", now))
	mustExecStoreTestSQL(t, db, `INSERT INTO artifacts_fts (artifact_id, title, body, source_path) VALUES (?, ?, ?, ?)`, "ds_FTS1", "Architecture Plan", "This is the body of our architecture plan.", "plans/architecture.md")
}

func TestGetArtifact_ShortID(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_FULL001", "r1", "plan", "", "Short ID Test", "draft", "rev_s1", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'ab12cd34' WHERE id = 'ds_FULL001'")

	art, err := db.GetArtifact("ab12cd34")
	require.NoError(t, err)
	assert.Equal(t, "ds_FULL001", art.ID,
		"short_id lookup failed: got %q", art.ID)
	assert.Equal(t, "ab12cd34", art.ShortID,
		"short_id field: want 'ab12cd34', got %q", art.ShortID)

}

func TestGetArtifact_FullIDStillWorks(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_FULL002", "r1", "spec", "", "Full ID Test", "draft", "rev_s2", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'ef56gh78' WHERE id = 'ds_FULL002'")

	art, err := db.GetArtifact("ds_FULL002")
	require.NoError(t, err)
	assert.Equal(t, "ds_FULL002", art.ID,
		"full ID lookup failed: got %q", art.ID)

}

func TestGetTagsForArtifact_WithThreeTags_ReturnsAllTags(t *testing.T) {
	db := openTestDB(t)
	seedArtifactTags(t, db)

	tags, err := db.GetTagsForArtifact("ds_TAG1")
	require.NoError(t, err)

	require.Len(t, tags, 3)
	assert.Equal(t, "auth", tags[0].Tag)
	assert.Equal(t, "inferred-dir", tags[1].Tag)
	assert.Equal(t, "v2", tags[2].Tag)
}

func TestDeleteTag_WithMatchingTag_RemovesOnlyThatTag(t *testing.T) {
	db := openTestDB(t)
	seedArtifactTags(t, db)

	err := db.DeleteTag("ds_TAG1", "v2")
	require.NoError(t, err)

	tags, err := db.GetTagsForArtifact("ds_TAG1")
	require.NoError(t, err)
	require.Len(t, tags, 2)
	assert.Equal(t, "auth", tags[0].Tag)
	assert.Equal(t, "inferred-dir", tags[1].Tag)
}

func TestDeleteAutoTags_WithManualAndAutomaticTags_PreservesManualTag(t *testing.T) {
	db := openTestDB(t)
	seedArtifactTags(t, db)

	err := db.DeleteAutoTags("ds_TAG1")
	require.NoError(t, err)

	tags, err := db.GetTagsForArtifact("ds_TAG1")
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "v2", tags[0].Tag)
}

func seedArtifactTags(t *testing.T, db *DB) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_TAG1", "r1", "plan", "", "Tagged", "draft", "rev_t1", now, now))
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES (?, ?, ?, ?)`, "ds_TAG1", "auth", "frontmatter", now)
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES (?, ?, ?, ?)`, "ds_TAG1", "v2", "manual", now)
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES (?, ?, ?, ?)`, "ds_TAG1", "inferred-dir", "inferred", now)
}

func TestInsertTag_DuplicateIsNoOp(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_DUP1", "r1", "plan", "", "Dup", "draft", "rev_d1", now, now))
	mustExecStoreTestSQL(t, db, `INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES (?, ?, ?, ?)`, "ds_DUP1", "auth", "manual", now)

	err := db.InsertTag("ds_DUP1", "auth", "manual", now)
	require.NoError(t, err)

	tags, err := db.GetTagsForArtifact("ds_DUP1")
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "auth", tags[0].Tag)
}

func TestListArtifacts_FilterByTag(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_T1", "r1", "plan", "", "Auth Plan", "draft", "rev_1", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_T2", "r1", "spec", "", "Other Spec", "draft", "rev_2", now, now))
	require.NoError(t, db.InsertTag("ds_T1", "auth", "manual", now))

	arts, err := db.ListArtifacts(FilterParams{Tag: "auth"})

	require.NoError(t, err)
	require.Len(t, arts, 1)
	assert.Equal(t, "ds_T1", arts[0].ID)
}

func TestListArtifacts_WithUnknownTag_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_T1", "r1", "plan", "", "Auth Plan", "draft", "rev_1", now, now))
	require.NoError(t, db.InsertTag("ds_T1", "auth", "manual", now))

	arts, err := db.ListArtifacts(FilterParams{Tag: "nonexistent"})

	require.NoError(t, err)
	assert.Empty(t, arts)
}

func TestListArtifacts_FilterByBranch(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, git_current_branch, created_at, updated_at) VALUES ('r1', '/tmp', 'main', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_B1", "r1", "plan", "", "Main Plan", "draft", "rev_1", now, now))

	arts, err := db.ListArtifacts(FilterParams{Branch: "main"})

	require.NoError(t, err)
	require.Len(t, arts, 1)
	assert.Equal(t, "ds_B1", arts[0].ID)
}

func TestListArtifacts_WithUnknownBranch_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, git_current_branch, created_at, updated_at) VALUES ('r1', '/tmp', 'main', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_B1", "r1", "plan", "", "Main Plan", "draft", "rev_1", now, now))

	arts, err := db.ListArtifacts(FilterParams{Branch: "feature"})

	require.NoError(t, err)
	assert.Empty(t, arts)
}

func TestListArtifacts_FilterByUser(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, scanned_by, created_at, updated_at) VALUES ('r1', '/tmp', 'brenn', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_U1", "r1", "plan", "", "User Plan", "draft", "rev_1", now, now))

	arts, err := db.ListArtifacts(FilterParams{User: "brenn"})

	require.NoError(t, err)
	require.Len(t, arts, 1)
	assert.Equal(t, "ds_U1", arts[0].ID)
}

func TestListArtifacts_WithUnknownUser_ReturnsEmpty(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, scanned_by, created_at, updated_at) VALUES ('r1', '/tmp', 'brenn', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_U1", "r1", "plan", "", "User Plan", "draft", "rev_1", now, now))

	arts, err := db.ListArtifacts(FilterParams{User: "other"})

	require.NoError(t, err)
	assert.Empty(t, arts)
}

func TestResumeArtifacts(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_R1", "r1", "plan", "", "Resume Plan", "draft", "rev_1", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'abc12345' WHERE id = 'ds_R1'")
	mustExecStoreTestSQL(t, db, "INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('src_r1', 'ds_R1', 'r1', 'markdown', 'plans/x.md', 'plans/x.md|markdown', 'generic', NULL, ?, ?)", now, now)
	require.NoError(t, db.InsertTag("ds_R1", "beta", "manual", now))
	require.NoError(t, db.InsertTag("ds_R1", "auth", "manual", now))

	rows, err := db.ResumeArtifacts("/tmp/repo", FilterParams{})
	require.NoError(t, err)
	require.Len(t, rows, 1,
		"expected 1 resume row, got %d", len(rows))
	assert.Equal(t, "abc12345", rows[0].ShortID,
		"short_id: want 'abc12345', got %q", rows[0].ShortID)
	assert.Equal(t, "plans/x.md", rows[0].SourcePath,
		"source path: want 'plans/x.md', got %q", rows[0].SourcePath)
	assert.Equal(t, "auth, beta", rows[0].TagsJoined,
		"tags: want 'auth, beta', got %q", rows[0].TagsJoined)
	assert.NotEmpty(t, rows[0].AuthoredAt)
	assert.NotEmpty(t, rows[0].UpdatedAt)
	{

		_, err := time.Parse(time.RFC3339, rows[0].AuthoredAt)
		assert.NoError(t, err,
			"authored_at RFC3339: %v", err)
	}
	{

		_, err := time.Parse(time.RFC3339, rows[0].UpdatedAt)
		assert.NoError(t, err,
			"updated_at RFC3339: %v", err)
	}

}

func TestResumeArtifacts_DeduplicatesMultipleSources(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo2', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_MDUP", "r1", "plan", "", "Dup Sources", "draft", "rev_md", now, now))
	mustExecStoreTestSQL(t, db, "INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('s1', 'ds_MDUP', 'r1', 'markdown', 'z-last.md', 'z|md', 'generic', NULL, ?, ?)", now, now)
	mustExecStoreTestSQL(t, db, "INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('s2', 'ds_MDUP', 'r1', 'markdown', 'a-first.md', 'a|md', 'generic', NULL, ?, ?)", now, now)

	rows, err := db.ResumeArtifacts("/tmp/repo2", FilterParams{})
	require.NoError(t, err)
	require.Len(t, rows, 1,
		"expected 1 row (deduped), got %d", len(rows))
	assert.Equal(t, "a-first.md", rows[0].SourcePath,
		"MIN(path): want 'a-first.md', got %q", rows[0].SourcePath)

}

func TestResumeArtifacts_SortsByUpdatedAtDescending(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	older := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/sort', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_OLD", "r1", "plan", "", "Older", "draft", "rev_o", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_NEW", "r1", "plan", "", "Newer", "draft", "rev_n", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET updated_at = ? WHERE id = 'ds_OLD'", older)
	mustExecStoreTestSQL(t, db, "INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('so', 'ds_OLD', 'r1', 'markdown', 'a.md', 'a|md', 'generic', NULL, ?, ?)", now, now)
	mustExecStoreTestSQL(t, db, "INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ('sn', 'ds_NEW', 'r1', 'markdown', 'b.md', 'b|md', 'generic', NULL, ?, ?)", now, now)

	rows, err := db.ResumeArtifacts("/tmp/sort", FilterParams{})
	require.NoError(t, err)
	require.Len(t, rows, 2,
		"want 2 rows, got %d", len(rows))
	assert.Equal(t, "ds_NEW", rows[0].ID)
	assert.Equal(t, "ds_OLD", rows[1].ID)

}

func TestAssignArtifactShortID_CollisionUsesSuffix(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_COLA", "r1", "plan", "", "A", "draft", "rev_a", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_COLB", "r1", "plan", "", "B", "draft", "rev_b", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'deadbeef' WHERE id = 'ds_COLA'")

	err := db.AssignArtifactShortID("ds_COLB", "deadbeef")
	require.NoError(t, err)

	artB, err := db.GetArtifact("ds_COLB")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef1", artB.ShortID,
		"after collision want short_id deadbeef1, got %q", artB.ShortID)

}

func TestAssignArtifactShortID_SecondCollisionUsesSuffix2(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_X1", "r1", "plan", "", "X1", "draft", "rev_1", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_X2", "r1", "plan", "", "X2", "draft", "rev_2", now, now))
	require.NoError(t, db.InsertArtifactDirect("ds_X3", "r1", "plan", "", "X3", "draft", "rev_3", now, now))
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'cafebabe' WHERE id = 'ds_X1'")
	mustExecStoreTestSQL(t, db, "UPDATE artifacts SET short_id = 'cafebabe1' WHERE id = 'ds_X2'")

	err := db.AssignArtifactShortID("ds_X3", "cafebabe")
	require.NoError(t, err)

	art, err := db.GetArtifact("ds_X3")
	require.NoError(t, err)
	assert.Equal(t, "cafebabe2", art.ShortID,
		"want cafebabe2, got %q", art.ShortID)

}

func TestUpdateArtifactShortID(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp', ?, ?)", now, now)
	require.NoError(t, db.InsertArtifactDirect("ds_SID1", "r1", "plan", "", "SID Test", "draft", "rev_1", now, now))

	err := db.UpdateArtifactShortID("ds_SID1", "deadbeef")
	require.NoError(t, err)

	art, err := db.GetArtifact("deadbeef")
	require.NoError(t, err)
	require.NotNil(t, art)
	assert.Equal(t, "ds_SID1", art.ID)

}

func TestUpdateScanMeta_WithUser(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	mustExecStoreTestSQL(t, db, "INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/x', ?, ?)", now, now)

	db.UpdateScanMeta("r1", "abc123", "brenn", now)

	meta := db.GetRepoByRoot("/tmp/x")
	require.NotNil(t, meta,
		"expected repo meta")
	assert.Equal(t, "brenn", meta.ScannedBy,
		"scanned_by: want 'brenn', got %q", meta.ScannedBy)
	assert.Equal(t, "abc123", meta.LastScanCommit,
		"last_scan_commit: want 'abc123', got %q", meta.LastScanCommit)

}
