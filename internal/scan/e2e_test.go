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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)

	defer db.Close()

	cfg, err := config.LoadRepoConfig(repoRoot)
	require.NoError(t, err)

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&openspec.Adapter{}, &adr.Adapter{}, &markdown.Adapter{}}
	s := New(db, ids, adpts)

	result, err := s.Run(context.Background(), repoRoot, cfg)
	require.NoError(t, err)

	// Should find 6 artifacts total: OpenSpec collection, bundle, 2 child files, 1 ADR, 1 plan.
	totalNew := result.New
	assert.Equal(t, 6, totalNew,
		"expected 6 new artifacts, got %d (found: %v)", totalNew, result.Found)

	// Verify kinds
	arts, err := db.ListArtifacts(store.FilterParams{})
	require.NoError(t, err)

	kinds := make(map[string]int)
	for _, a := range arts {
		kinds[a.Kind]++
	}
	assert.Equal(t, 4, kinds["spec"],
		"expected 4 specs, got %d", kinds["spec"])
	assert.Equal(t, 1, kinds["decision"],
		"expected 1 decision, got %d", kinds["decision"])
	assert.Equal(t, 1, kinds["plan"],
		"expected 1 plan, got %d", kinds["plan"])

	// Verify ADR status
	for _, a := range arts {
		if a.Kind == "decision" && a.Subtype == "adr" {
			assert.Equal(t, "accepted", a.Status)
		}
		if a.Kind == "spec" && (a.Subtype == "openspec_child" || a.Subtype == "openspec_change_bundle") {
			assert.Equal(t, "proposed", a.Status)
		}

	}

	// Verify todos extracted (4 from bundle, 4 from tasks artifact, 3 from plan).
	var todoCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount))
	assert.Equal(t, 11, todoCount,
		"expected 11 total todos, got %d", todoCount)

}
