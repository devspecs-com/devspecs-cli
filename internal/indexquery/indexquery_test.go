package indexquery

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestLoadCandidatesAttachesPersistedSections(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_sec"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("ds_sec", repoID, "plan", "", "Billing Plan", "active", "rev_sec", now, now); err != nil {
		t.Fatal(err)
	}
	body := "# Billing Plan\n\n## Replay Boundary\n\nstripe_event_id idempotency matters."
	if err := db.InsertRevisionDirect("rev_sec", "ds_sec", "sha256:test", body, "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("src_sec", "ds_sec", repoID, "markdown", "docs/plans/billing.md", "docs/plans/billing.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown(body), "ds_sec", "rev_sec", "docs/plans/billing.md")
	if err := db.ReplaceArtifactSections("ds_sec", "rev_sec", sections, now); err != nil {
		t.Fatal(err)
	}

	candidates, err := LoadCandidates(db, store.FilterParams{RepoRoot: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if got := len(candidates[0].Sections); got == 0 {
		t.Fatalf("expected persisted sections on candidate: %#v", candidates[0])
	}
	if candidates[0].Sections[0].SourcePath != "docs/plans/billing.md" {
		t.Fatalf("unexpected section source path: %#v", candidates[0].Sections[0])
	}
}

func TestLoadCandidatesByArtifactIDsMatchesFullLoader(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_batch"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("ds_a", repoID, "plan", "", "Alpha Plan", "active", "rev_a", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect("rev_a", "ds_a", "sha256:a", "# Alpha\n\n## Replay\n\nstripe_event_id idempotency.", `{"mode":"test"}`, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("src_a", "ds_a", repoID, "markdown", "docs/alpha.md", "docs/alpha.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertLink("link_a", "ds_a", "implements", "ds_target", now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES ('todo_a', 'ds_a', 'rev_a', 0, 'Preserve todo', 0, 'docs/alpha.md', 3, ?)", now); err != nil {
		t.Fatal(err)
	}
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Alpha\n\n## Replay\n\nstripe_event_id idempotency."), "ds_a", "rev_a", "docs/alpha.md")
	if err := db.ReplaceArtifactSections("ds_a", "rev_a", sections, now); err != nil {
		t.Fatal(err)
	}

	full, err := LoadCandidates(db, store.FilterParams{RepoRoot: tmp})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := LoadCandidatesByArtifactIDs(db, store.FilterParams{RepoRoot: tmp}, []string{"ds_a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 1 || len(selected) != 1 {
		t.Fatalf("expected one full and selected candidate, got %d and %d", len(full), len(selected))
	}
	if !reflect.DeepEqual(selected[0], full[0]) {
		t.Fatalf("selected candidate differs from full loader:\nselected=%#v\nfull=%#v", selected[0], full[0])
	}
}

func TestPreselectArtifactIDsForQueryUsesFTSSectionAndPathLanes(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_preselect"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("ds_body", repoID, "plan", "", "Runtime Plan", "active", "rev_body", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect("rev_body", "ds_body", "sha256:body", "FluxNova requirements live here.", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("src_body", "ds_body", repoID, "markdown", "docs/runtime.md", "docs/runtime.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexArtifactFTS("ds_body", "Runtime Plan", "FluxNova requirements live here.", "docs/runtime.md"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("ds_section", repoID, "plan", "", "Guard Plan", "active", "rev_section", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect("rev_section", "ds_section", "sha256:section", "# Guard\n\n## CALM Guard\n\nEvidence details.", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("src_section", "ds_section", repoID, "markdown", "docs/guard.md", "docs/guard.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Guard\n\n## CALM Guard\n\nEvidence details."), "ds_section", "rev_section", "docs/guard.md")
	if err := db.ReplaceArtifactSections("ds_section", "rev_section", sections, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertArtifactDirect("ds_path", repoID, "plan", "", "Path Plan", "active", "rev_path", now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect("rev_path", "ds_path", "sha256:path", "Path-only body.", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("src_path", "ds_path", repoID, "markdown", "docs/fluxnova-path.md", "docs/fluxnova-path.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}

	ids, report, err := PreselectArtifactIDsForQuery(db, store.FilterParams{RepoRoot: tmp}, "FluxNova CALM Guard", PreselectOptions{
		PreselectLimit:              10,
		MaxRepoSizeForFullHydration: 0,
		FallbackFullHydrationBelow:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.FallbackReason != "" {
		t.Fatalf("unexpected fallback: %#v", report)
	}
	for _, want := range []string{"ds_body", "ds_section", "ds_path"} {
		if !containsID(ids, want) {
			t.Fatalf("expected %s in preselected IDs, got %v", want, ids)
		}
	}
	if report.LaneCounts["artifact_fts"] == 0 || report.LaneCounts["section_fts"] == 0 || report.LaneCounts["title_path_like"] == 0 {
		t.Fatalf("expected all lanes to contribute: %#v", report)
	}
}

func TestParseRuntimeModeDefaultsToPreselectActive(t *testing.T) {
	mode, err := ParseRuntimeMode("")
	if err != nil {
		t.Fatal(err)
	}
	if mode != RuntimeModePreselectActive {
		t.Fatalf("default runtime mode = %q, want %q", mode, RuntimeModePreselectActive)
	}

	full, err := ParseRuntimeMode("full")
	if err != nil {
		t.Fatal(err)
	}
	if full != RuntimeModeFull {
		t.Fatalf("explicit full runtime mode = %q, want %q", full, RuntimeModeFull)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
