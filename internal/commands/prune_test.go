package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestPruneCommand_DryRunThenDeleteJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	staleRoot := filepath.Join(t.TempDir(), "deleted")
	if _, err := db.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('stale', ?, ?, ?)`, staleRoot, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('artifact', 'stale', 'plan', '', 'Plan', 'draft', ?, ?, ?, ?)`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	dryRun := NewPruneCmd()
	dryRun.SetArgs([]string{"--dry-run", "--json"})
	dryRunOut := &bytes.Buffer{}
	dryRun.SetOut(dryRunOut)
	if err := dryRun.Execute(); err != nil {
		t.Fatal(err)
	}
	var preview store.PruneReport
	if err := json.Unmarshal(dryRunOut.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || preview.RepositoriesPruned != 1 || preview.ArtifactsPruned != 1 {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	prune := NewPruneCmd()
	prune.SetArgs([]string{"--json"})
	pruneOut := &bytes.Buffer{}
	prune.SetOut(pruneOut)
	if err := prune.Execute(); err != nil {
		t.Fatal(err)
	}
	var applied store.PruneReport
	if err := json.Unmarshal(pruneOut.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.DryRun || applied.RepositoriesPruned != 1 || applied.ArtifactsPruned != 1 {
		t.Fatalf("unexpected applied report: %#v", applied)
	}
	db, err = store.Open(filepath.Join(home, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("repository remains after prune: count=%d err=%v", count, err)
	}
}

func TestPruneCommand_NoIndexDoesNotCreateOne(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--json"})
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "devspecs.db")); !os.IsNotExist(err) {
		t.Fatalf("prune created an index unexpectedly: %v", err)
	}
}
