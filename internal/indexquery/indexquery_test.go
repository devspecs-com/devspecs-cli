package indexquery

import (
	"path/filepath"
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
