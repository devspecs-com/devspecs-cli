package store

import (
	"reflect"
	"testing"
	"time"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
)

func TestListArtifactsByIDsRespectsFiltersAndInputOrder(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	rootA := t.TempDir()
	rootB := t.TempDir()
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_a', ?, ?, ?), ('repo_b', ?, ?, ?)", rootA, now, now, rootB, now, now); err != nil {
		t.Fatal(err)
	}
	mustNoErr(t, db.InsertArtifactDirect("ds_A", "repo_a", "plan", "", "Alpha", "draft", "rev_a", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_B", "repo_b", "plan", "", "Beta", "draft", "rev_b", now, now))
	mustNoErr(t, db.InsertArtifactDirect("ds_C", "repo_a", "spec", "", "Gamma", "draft", "rev_c", now, now))

	count, err := db.CountArtifacts(FilterParams{RepoRoot: rootA})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two repo_a artifacts, got %d", count)
	}

	rows, err := db.ListArtifactsByIDs([]string{"ds_C", "ds_A", "ds_B", "ds_A"}, FilterParams{RepoRoot: rootA})
	if err != nil {
		t.Fatal(err)
	}
	got := artifactIDs(rows)
	want := []string{"ds_C", "ds_A"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected artifact order: got %v want %v", got, want)
	}
}

func TestBatchHydrationMethodsGroupRows(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_batch', '/tmp/repo-batch', ?, ?)", now, now); err != nil {
		t.Fatal(err)
	}
	mustNoErr(t, db.InsertArtifactDirect("ds_BATCH", "repo_batch", "plan", "", "Batch Plan", "draft", "rev_batch", now, now))
	mustNoErr(t, db.InsertRevisionDirect("rev_batch", "ds_BATCH", "sha256:batch", "# Batch Plan\n\n## Notes\n\nHydrate me.", `{"mode":"test"}`, now))
	mustNoErr(t, db.InsertSourceDirect("src_batch_a", "ds_BATCH", "repo_batch", "markdown", "docs/batch.md", "docs/batch.md|markdown", "", "", now))
	mustNoErr(t, db.InsertSourceDirect("src_batch_b", "ds_BATCH", "repo_batch", "test_case", "tests/batch_test.go", "tests/batch_test.go|TestBatch|7", "", "", now))
	mustNoErr(t, db.InsertLink("link_batch", "ds_BATCH", "implements", "ds_TARGET", now))
	if _, err := db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('todo_batch', 'ds_BATCH', 'rev_batch', 0, 'Check batching', 0, 'docs/batch.md', 4, ?)", now); err != nil {
		t.Fatal(err)
	}
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Batch Plan\n\n## Notes\n\nHydrate me."), "ds_BATCH", "rev_batch", "docs/batch.md")
	mustNoErr(t, db.ReplaceArtifactSections("ds_BATCH", "rev_batch", sections, now))

	sources, err := db.GetSourcesForArtifacts([]string{"ds_BATCH"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sources["ds_BATCH"]); got != 2 {
		t.Fatalf("expected two sources, got %d", got)
	}
	links, err := db.GetLinksForArtifacts([]string{"ds_BATCH"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(links["ds_BATCH"]); got != 1 {
		t.Fatalf("expected one link, got %d", got)
	}
	todos, err := db.GetTodosForArtifacts([]string{"ds_BATCH"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(todos["ds_BATCH"]); got != 1 {
		t.Fatalf("expected one todo, got %d", got)
	}
	sectionsByID, err := db.GetSectionsForArtifacts([]string{"ds_BATCH"})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sectionsByID["ds_BATCH"]); got == 0 {
		t.Fatalf("expected sections, got %d", got)
	}
	revisions, err := db.GetRevisionsByIDs([]string{"rev_batch"})
	if err != nil {
		t.Fatal(err)
	}
	if revisions["rev_batch"].ExtractedJSON != `{"mode":"test"}` {
		t.Fatalf("unexpected revision payload: %#v", revisions["rev_batch"])
	}
}

func TestFindArtifactIDPreselectionLanesRespectFilters(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	root := t.TempDir()
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_find_ids', ?, ?, ?)", root, now, now); err != nil {
		t.Fatal(err)
	}
	mustNoErr(t, db.InsertArtifactDirect("ds_FLUX", "repo_find_ids", "requirement", "", "FluxNova AIGF Requirement", "active", "rev_flux", now, now))
	mustNoErr(t, db.InsertRevisionDirect("rev_flux", "ds_FLUX", "sha256:flux", "# Requirement\n\nFluxNova must support AIGF controls.", "", now))
	mustNoErr(t, db.InsertSourceDirect("src_flux", "ds_FLUX", "repo_find_ids", "markdown", "docs/fluxnova.md", "docs/fluxnova.md|markdown", "", "", now))
	mustNoErr(t, db.IndexArtifactFTS("ds_FLUX", "FluxNova AIGF Requirement", "FluxNova must support AIGF controls.", "docs/fluxnova.md"))
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Requirement\n\n## CALM Guard\n\nFluxNova control evidence."), "ds_FLUX", "rev_flux", "docs/fluxnova.md")
	mustNoErr(t, db.ReplaceArtifactSections("ds_FLUX", "rev_flux", sections, now))

	fp := FilterParams{RepoRoot: root}
	ftsIDs, err := db.FindArtifactIDsFTS(`"fluxnova"`, fp, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ftsIDs, []string{"ds_FLUX"}) {
		t.Fatalf("unexpected artifact FTS IDs: %v", ftsIDs)
	}
	sectionIDs, err := db.FindArtifactIDsBySectionFTS(`"guard"`, fp, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sectionIDs, []string{"ds_FLUX"}) {
		t.Fatalf("unexpected section FTS IDs: %v", sectionIDs)
	}
	likeIDs, err := db.FindArtifactIDsByTitleOrPathTerms([]string{"fluxnova"}, fp, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(likeIDs, []string{"ds_FLUX"}) {
		t.Fatalf("unexpected LIKE IDs: %v", likeIDs)
	}
}

func artifactIDs(rows []ArtifactRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
