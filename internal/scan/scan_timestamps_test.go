package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestScan_UnchangedBodyLeavesUpdatedAtStable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := exec.Command("git", "init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	mdPath := filepath.Join(tmp, "plans", "stamp.md")
	body := "---\ntitle: Stamp\nstatus: draft\n---\n# Stamp\n\nx\n"
	if err := os.WriteFile(mdPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	exec.Command("git", "-C", tmp, "add", "plans/stamp.md").Run()
	exec.Command("git", "-C", tmp, "commit", "-m", "add plan").Run()

	cfg := config.DefaultRepoConfig()
	dbPath := filepath.Join(tmp, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	sc := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := sc.Run(context.Background(), tmp, cfg); err != nil {
		t.Fatal(err)
	}

	var artifactID string
	if err := db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	var u1, lo1 string
	if err := db.QueryRow("SELECT updated_at, last_observed_at FROM artifacts WHERE id = ?", artifactID).Scan(&u1, &lo1); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	if _, err := sc.Run(context.Background(), tmp, cfg); err != nil {
		t.Fatal(err)
	}
	var u2, lo2 string
	if err := db.QueryRow("SELECT updated_at, last_observed_at FROM artifacts WHERE id = ?", artifactID).Scan(&u2, &lo2); err != nil {
		t.Fatal(err)
	}
	if u1 != u2 {
		t.Fatalf("updated_at changed on unchanged body: %q -> %q", u1, u2)
	}
	t1, _ := time.Parse(time.RFC3339, lo1)
	t2, _ := time.Parse(time.RFC3339, lo2)
	if t2.Before(t1) {
		t.Fatalf("last_observed_at went backwards: %q -> %q", lo1, lo2)
	}
}
