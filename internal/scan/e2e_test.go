package scan

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

func TestE2E_SpecExample(t *testing.T) {
	repoRoot := filepath.Join(testdataDir(), "e2e", "spec-section-16")

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}}
	s := New(db, ids, adpts)

	result, err := s.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Should find 3 artifacts total: 1 openspec, 1 adr, 1 plan
	totalNew := result.New
	if totalNew != 3 {
		t.Errorf("expected 3 new artifacts, got %d (found: %v)", totalNew, result.Found)
	}

	// Verify kinds
	arts, err := db.ListArtifacts(store.FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := make(map[string]int)
	for _, a := range arts {
		kinds[a.Kind]++
	}
	if kinds["spec"] != 1 {
		t.Errorf("expected 1 spec, got %d", kinds["spec"])
	}
	if kinds["decision"] != 1 {
		t.Errorf("expected 1 decision, got %d", kinds["decision"])
	}
	if kinds["plan"] != 1 {
		t.Errorf("expected 1 plan, got %d", kinds["plan"])
	}

	// Verify ADR status
	for _, a := range arts {
		if a.Kind == "decision" && a.Subtype == "adr" && a.Status != "accepted" {
			t.Errorf("ADR status: want 'accepted', got %q", a.Status)
		}
		if a.Kind == "spec" && a.Subtype == "openspec_change" && a.Status != "proposed" {
			t.Errorf("OpenSpec status: want 'proposed', got %q", a.Status)
		}
	}

	// Verify todos extracted (4 from tasks.md + 3 from plan)
	var todoCount int
	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
	if todoCount != 7 {
		t.Errorf("expected 7 total todos, got %d", todoCount)
	}
}
